//go:build container

package repo

import (
	"context"
	"os"
	osexec "os/exec"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/manchtools/cadestro/sdk/pkg"
	sysexec "github.com/manchtools/cadestro/sdk/sys/exec"
)

const armoredTestKey = `-----BEGIN PGP PUBLIC KEY BLOCK-----

mDMEajZ3rBYJKwYBBAHaRw8BAQdAwYnGPZg6OfGisPl+/RAOseXCejyrS+CjiSKZ
lCXoDjG0MVBNIFJlcG8gVGVzdCBLZXkgPHJlcG8tdGVzdEBwb3dlci1tYW5hZ2Uu
aW52YWxpZD6IkAQTFggAOBYhBN1Qn9dyeqU4SntfyL6U56IHNSDTBQJqNnesAhsj
BQsJCAcCBhUKCQgLAgQWAgMBAh4BAheAAAoJEL6U56IHNSDTGp4BALFRj253kOzs
gxVpo/34NPKJga6Orty0loT/fCuEIwhvAQCpfGCUcX2QgqDXxrlS9IQ6wn6JCPNw
fGAUk8ja+rIzBA==
=8ls7
-----END PGP PUBLIC KEY BLOCK-----
`

func realRepoMgr(t *testing.T, b pkg.Backend) Manager {
	t.Helper()
	if !slices.Contains(pkg.Detect(), b) {
		t.Skipf("%s not installed here; repo backend not exercisable", b)
	}
	r, err := sysexec.NewRunner(sysexec.Direct)
	if err != nil {
		t.Fatalf("NewRunner(Direct): %v", err)
	}
	m, err := New(b, r)
	if err != nil {
		t.Fatalf("New(%s): %v", b, err)
	}
	return m
}

func repoCtx(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	t.Cleanup(cancel)
	return ctx
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(b)
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func TestApt_ApplyRemove_Container(t *testing.T) {
	m := realRepoMgr(t, pkg.Apt)
	ctx := repoCtx(t)
	const name = "cadestro-test-apt"
	repoFile, keyFile := aptRepoFile(name), aptKeyFile(name)
	t.Cleanup(func() { _, _ = m.Remove(context.Background(), name) })

	repo := Repository{Name: name, Apt: &AptConfig{
		URL:          "https://example.com/cadestro-test-debian",
		Distribution: "bookworm",
		Components:   []string{"main"},
		GPGKey:       []byte(armoredTestKey),
	}}

	o, err := m.Apply(ctx, repo)
	if err != nil {
		t.Fatalf("Apply(apt): %v", err)
	}
	if !o.Changed {
		t.Error("first Apply should report Changed=true")
	}

	src := readFile(t, repoFile)
	for _, want := range []string{"Types: deb", "URIs: https://example.com/cadestro-test-debian", "Suites: bookworm", "Components: main", "Signed-By: " + keyFile} {
		if !strings.Contains(src, want) {
			t.Errorf("deb822 source missing %q:\n%s", want, src)
		}
	}
	key := readFile(t, keyFile)
	if key == "" {
		t.Error("keyring is empty")
	}
	if strings.HasPrefix(key, "-----BEGIN PGP") {
		t.Error("keyring is still ASCII-armored — real `gpg --dearmor` did not run")
	}

	if !aptParsesRepo(t, "https://example.com/cadestro-test-debian") {
		t.Error("apt did not parse the written .sources (`apt-get --print-uris update` omits the repo URI)")
	}

	o2, err := m.Apply(ctx, repo)
	if err != nil {
		t.Fatalf("re-Apply(apt): %v", err)
	}
	if o2.Changed {
		t.Error("idempotent re-Apply should report Changed=false")
	}

	o3, err := m.Remove(ctx, name)
	if err != nil {
		t.Fatalf("Remove(apt): %v", err)
	}
	if !o3.Changed {
		t.Error("Remove of a present repo should report Changed=true")
	}
	if fileExists(repoFile) || fileExists(keyFile) {
		t.Errorf("Remove left files behind: source=%v key=%v", fileExists(repoFile), fileExists(keyFile))
	}
	o4, err := m.Remove(ctx, name)
	if err != nil {
		t.Fatalf("re-Remove(apt): %v", err)
	}
	if o4.Changed {
		t.Error("Remove of an absent repo should report Changed=false")
	}
}

func TestDnf_ApplyRemove_Container(t *testing.T) {
	m := realRepoMgr(t, pkg.Dnf)
	ctx := repoCtx(t)
	const name = "cadestro-test-dnf"
	repoFile := dnfRepoFile(name)
	t.Cleanup(func() { _, _ = m.Remove(context.Background(), name) })

	repo := Repository{Name: name, Dnf: &DnfConfig{
		BaseURL: "https://example.com/cadestro-test-el9", Description: "Cadestro Test", Enabled: true,
		GPGCheck: true, GPGKey: "https://example.com/cadestro-test-el9/RPM-GPG-KEY",
	}}

	o, err := m.Apply(ctx, repo)
	if err != nil {
		t.Fatalf("Apply(dnf): %v", err)
	}
	if !o.Changed {
		t.Error("first Apply should report Changed=true")
	}
	body := readFile(t, repoFile)
	for _, want := range []string{"[" + name + "]", "name=Cadestro Test", "baseurl=https://example.com/cadestro-test-el9", "enabled=1", "gpgcheck=1", "gpgkey=https://example.com/cadestro-test-el9/RPM-GPG-KEY"} {
		if !strings.Contains(body, want) {
			t.Errorf(".repo missing %q:\n%s", want, body)
		}
	}

	if !dnfListsRepo(name) {
		t.Errorf("dnf did not list %q after Apply (`dnf repolist --all -C` omits it) — config not accepted", name)
	}
	if o2, err := m.Apply(ctx, repo); err != nil || o2.Changed {
		t.Errorf("idempotent re-Apply: changed=%v err=%v", o2.Changed, err)
	}
	if o3, err := m.Remove(ctx, name); err != nil || !o3.Changed || fileExists(repoFile) {
		t.Errorf("Remove: changed=%v err=%v exists=%v", o3.Changed, err, fileExists(repoFile))
	}
	if o4, err := m.Remove(ctx, name); err != nil || o4.Changed {
		t.Errorf("idempotent Remove: changed=%v err=%v", o4.Changed, err)
	}
}

func TestPacman_ApplyRemove_Container(t *testing.T) {
	m := realRepoMgr(t, pkg.Pacman)
	ctx := repoCtx(t)
	const name = "cadestro-test-pacman"
	t.Cleanup(func() { _, _ = m.Remove(context.Background(), name) })

	repo := Repository{Name: name, Pacman: &PacmanConfig{
		Server: "https://example.com/cadestro-test-arch/$repo/os/$arch", SigLevel: "Required DatabaseOptional",
	}}

	o, err := m.Apply(ctx, repo)
	if err != nil {
		t.Fatalf("Apply(pacman): %v", err)
	}
	if !o.Changed {
		t.Error("first Apply should report Changed=true")
	}
	conf := readFile(t, pacmanConf)
	for _, want := range []string{"[" + name + "]", "SigLevel = Required DatabaseOptional", "Server = https://example.com/cadestro-test-arch/$repo/os/$arch"} {
		if !strings.Contains(conf, want) {
			t.Errorf("pacman.conf missing %q", want)
		}
	}

	if !pacmanParsesRepo(name) {
		t.Errorf("`pacman-conf --repo %s` failed after Apply — pacman did not accept the appended section", name)
	}
	if o2, err := m.Apply(ctx, repo); err != nil || o2.Changed {
		t.Errorf("idempotent re-Apply: changed=%v err=%v", o2.Changed, err)
	}
	if o3, err := m.Remove(ctx, name); err != nil || !o3.Changed {
		t.Errorf("Remove: changed=%v err=%v", o3.Changed, err)
	}
	if strings.Contains(readFile(t, pacmanConf), "["+name+"]") {
		t.Error("Remove left the [section] in pacman.conf")
	}
	if o4, err := m.Remove(ctx, name); err != nil || o4.Changed {
		t.Errorf("idempotent Remove: changed=%v err=%v", o4.Changed, err)
	}
}

func TestZypper_ApplyRemove_Container(t *testing.T) {
	m := realRepoMgr(t, pkg.Zypper)
	ctx := repoCtx(t)
	const name = "cadestro-test-zypper"
	t.Cleanup(func() { _, _ = m.Remove(context.Background(), name) })

	repo := Repository{Name: name, Zypper: &ZypperConfig{
		URL: "https://example.com/cadestro-test-suite", Description: "Cadestro Test", Enabled: false, Autorefresh: false, GPGCheck: true,
	}}

	o, err := m.Apply(ctx, repo)
	if err != nil {
		t.Fatalf("Apply(zypper): %v", err)
	}
	if !o.Changed {
		t.Error("Apply should report Changed=true (zypper addrepo has no cheap idempotency probe)")
	}
	if !zypperRepoListed(t, name) {
		t.Errorf("`zypper lr %s` does not list the repo after Apply", name)
	}

	o3, err := m.Remove(ctx, name)
	if err != nil {
		t.Fatalf("Remove(zypper): %v", err)
	}
	if !o3.Changed {
		t.Error("Remove of a present repo should report Changed=true")
	}
	if zypperRepoListed(t, name) {
		t.Errorf("`zypper lr %s` still lists the repo after Remove", name)
	}
	o4, err := m.Remove(ctx, name)
	if err != nil {
		t.Fatalf("re-Remove(zypper): %v", err)
	}
	if o4.Changed {
		t.Error("Remove of an absent repo should report Changed=false (not found)")
	}
}

func zypperRepoListed(t *testing.T, name string) bool {
	t.Helper()
	return osexec.Command("zypper", "--non-interactive", "lr", name).Run() == nil
}

func aptParsesRepo(t *testing.T, uri string) bool {
	t.Helper()
	out, _ := osexec.Command("apt-get", "--print-uris", "update").CombinedOutput()
	return strings.Contains(string(out), uri)
}

func dnfListsRepo(name string) bool {
	out, _ := osexec.Command("dnf", "repolist", "--all", "-C").CombinedOutput()
	return strings.Contains(string(out), name)
}

func pacmanParsesRepo(name string) bool {
	return osexec.Command("pacman-conf", "--repo", name).Run() == nil
}
