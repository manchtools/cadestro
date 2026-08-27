//go:build container

package repo

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/manchtools/cadestro/sdk/pkg"
)

const armoredTestKey2 = `-----BEGIN PGP PUBLIC KEY BLOCK-----

mDMEajeV+BYJKwYBBAHaRw8BAQdAxo9qGCe3XUaNKRWUE98ne0eruTOpxaf85Jlm
Crw1INe0NVBNIFJlcG8gVGVzdCBLZXkgMiA8cmVwby10ZXN0LTJAcG93ZXItbWFu
YWdlLmludmFsaWQ+iJMEExYKADsWIQQdwrxp0Zm6NyeuKiSY6wjO212V4AUCajeV
+AIbIwULCQgHAgIiAgYVCgkICwIEFgIDAQIeBwIXgAAKCRCY6wjO212V4B/oAQDX
IzP53rgn9zz6rhzYNLf7yWtog0MbeGAjFPy5/B2G3gEAuhJEqqjp+uBXM0MvYrfc
547DbFV618I2mEz+yeMz7gI=
=CJdT
-----END PGP PUBLIC KEY BLOCK-----
`

func TestRepoSecurity_AptMalformedKey_Rejected_Container(t *testing.T) {
	m := realRepoMgr(t, pkg.Apt)
	ctx := repoCtx(t)
	const name = "cadestro-sec-apt-badkey"
	repoFile, keyFile := aptRepoFile(name), aptKeyFile(name)
	t.Cleanup(func() { _, _ = m.Remove(context.Background(), name) })

	_, err := m.Apply(ctx, Repository{Name: name, Apt: &AptConfig{
		URL:          "https://example.com/cadestro-sec-debian",
		Distribution: "bookworm",
		Components:   []string{"main"},
		GPGKey:       []byte("this is definitely not an OpenPGP public key"),
	}})
	if err == nil {
		t.Fatal("Apply accepted a malformed GPG key; expected real gpg --dearmor to fail the Apply closed")
	}
	if fileExists(repoFile) {
		t.Errorf("malformed-key Apply still wrote the .sources file %s — must not configure a repo whose key failed", repoFile)
	}
	if fileExists(keyFile) {
		t.Errorf("malformed-key Apply wrote a keyring %s from un-dearmorable input", keyFile)
	}
}

func TestRepoSecurity_AptConflictCleanupConfinedToKeyringJail_Container(t *testing.T) {
	m := realRepoMgr(t, pkg.Apt)
	ctx := repoCtx(t)
	const name = "cadestro-sec-apt-conflict"
	const url = "https://example.com/cadestro-sec-conflict"
	repoFile, keyFile := aptRepoFile(name), aptKeyFile(name)

	if err := os.MkdirAll(aptKeyringDir, 0o755); err != nil {
		t.Fatalf("prepare keyring dir: %v", err)
	}
	decoySource := aptSourcesDir + "/cadestro-sec-decoy.sources"
	inJailKey := aptKeyringDir + "/cadestro-sec-decoy-injail.gpg"
	outOfJailSentinel := "/etc/cadestro-sec-out-of-jail-sentinel.gpg"

	for path, body := range map[string]string{
		inJailKey:         "decoy-in-jail-key\n",
		outOfJailSentinel: "do-not-delete-me\n",
		decoySource: "Types: deb\nURIs: " + url + "\nSuites: bookworm\nComponents: main\n" +
			"Signed-By: " + inJailKey + "\n" +
			"Signed-By: " + outOfJailSentinel + "\n",
	} {
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatalf("plant %s: %v", path, err)
		}
	}
	t.Cleanup(func() {
		_, _ = m.Remove(context.Background(), name)
		_ = os.Remove(decoySource)
		_ = os.Remove(inJailKey)
		_ = os.Remove(outOfJailSentinel)
	})

	o, err := m.Apply(ctx, Repository{Name: name, Apt: &AptConfig{
		URL: url, Distribution: "bookworm", Components: []string{"main"}, GPGKey: []byte(armoredTestKey),
	}})
	if err != nil {
		t.Fatalf("Apply(apt conflict): %v", err)
	}

	if fileExists(decoySource) {
		t.Errorf("conflicting source %s was not removed by cleanup", decoySource)
	}
	if fileExists(inJailKey) {
		t.Errorf("in-jail decoy keyring %s was not cleaned up (conflict cleanup did not run on the in-jail key)", inJailKey)
	}
	if !fileExists(outOfJailSentinel) {
		t.Errorf("SECURITY: out-of-jail Signed-By target %s was deleted — cleanup escaped the keyring jail (arbitrary privileged delete)", outOfJailSentinel)
	}
	if !strings.Contains(o.Result.Stdout, "refusing to remove out-of-jail") {
		t.Errorf("expected the log to record refusing the out-of-jail key; log:\n%s", o.Result.Stdout)
	}
	if !fileExists(repoFile) || !fileExists(keyFile) {
		t.Errorf("the new repo was not configured: source=%v key=%v", fileExists(repoFile), fileExists(keyFile))
	}
}

func TestRepoSecurity_AptKeyRotation_Container(t *testing.T) {
	m := realRepoMgr(t, pkg.Apt)
	ctx := repoCtx(t)
	const name = "cadestro-sec-apt-rotate"
	const url = "https://example.com/cadestro-sec-rotate"
	keyFile := aptKeyFile(name)
	t.Cleanup(func() { _, _ = m.Remove(context.Background(), name) })

	repoWith := func(key string) Repository {
		return Repository{Name: name, Apt: &AptConfig{
			URL: url, Distribution: "bookworm", Components: []string{"main"}, GPGKey: []byte(key),
		}}
	}

	if _, err := m.Apply(ctx, repoWith(armoredTestKey)); err != nil {
		t.Fatalf("Apply(key A): %v", err)
	}
	keyA := readFile(t, keyFile)
	if keyA == "" || strings.HasPrefix(keyA, "-----BEGIN PGP") {
		t.Fatalf("key A was not dearmored into the keyring")
	}

	o, err := m.Apply(ctx, repoWith(armoredTestKey2))
	if err != nil {
		t.Fatalf("Apply(key B rotation): %v", err)
	}
	if !o.Changed {
		t.Error("rotating to a different key should report Changed=true")
	}
	keyB := readFile(t, keyFile)
	if keyB == keyA {
		t.Error("the keyring still holds key A after rotation to key B")
	}
	if keyB == "" || strings.HasPrefix(keyB, "-----BEGIN PGP") {
		t.Error("key B was not dearmored into the keyring")
	}

	o2, err := m.Apply(ctx, repoWith(armoredTestKey2))
	if err != nil {
		t.Fatalf("re-Apply(key B): %v", err)
	}
	if o2.Changed {
		t.Error("re-applying the same rotated key should be idempotent (Changed=false)")
	}
}

func TestRepoSecurity_AptRejectedReapplyPreservesExisting_Container(t *testing.T) {
	m := realRepoMgr(t, pkg.Apt)
	ctx := repoCtx(t)
	const name = "cadestro-sec-apt-preserve"
	const url = "https://example.com/cadestro-sec-preserve"
	repoFile := aptRepoFile(name)
	t.Cleanup(func() { _, _ = m.Remove(context.Background(), name) })

	if _, err := m.Apply(ctx, Repository{Name: name, Apt: &AptConfig{
		URL: url, Distribution: "bookworm", Components: []string{"main"}, GPGKey: []byte(armoredTestKey),
	}}); err != nil {
		t.Fatalf("initial Apply: %v", err)
	}
	original := readFile(t, repoFile)

	_, err := m.Apply(ctx, Repository{Name: name, Apt: &AptConfig{
		URL: url, Distribution: "bookworm\nMalicious: injected", Components: []string{"main"}, GPGKey: []byte(armoredTestKey),
	}})
	if !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("re-Apply with a control-char distribution: got err=%v, want ErrInvalidConfig", err)
	}
	if got := readFile(t, repoFile); got != original {
		t.Errorf("a rejected re-Apply mutated the existing .sources:\n--- before ---\n%s\n--- after ---\n%s", original, got)
	}
}

func TestRepoSecurity_AptTrustedYes_OperatorOverride_Container(t *testing.T) {
	m := realRepoMgr(t, pkg.Apt)
	ctx := repoCtx(t)
	const name = "cadestro-sec-apt-trusted"
	const url = "https://example.com/cadestro-sec-trusted"
	repoFile := aptRepoFile(name)
	t.Cleanup(func() { _, _ = m.Remove(context.Background(), name) })

	if _, err := m.Apply(ctx, Repository{Name: name, Apt: &AptConfig{
		URL: url, Distribution: "bookworm", Components: []string{"main"}, Trusted: true,
	}}); err != nil {
		t.Fatalf("Apply(apt Trusted:yes) is an allowed operator override but failed: %v", err)
	}
	src := readFile(t, repoFile)
	if !strings.Contains(src, "Trusted: yes") {
		t.Errorf(".sources missing the operator-chosen Trusted: yes:\n%s", src)
	}
	if strings.Contains(src, "Signed-By:") {
		t.Errorf("keyless Trusted repo must not emit a Signed-By:\n%s", src)
	}
	if !aptParsesRepo(t, url) {
		t.Error("apt did not parse the Trusted: yes .sources")
	}
}

func TestRepoSecurity_DnfGpgcheckZeroDropsKeyImport_Container(t *testing.T) {
	m := realRepoMgr(t, pkg.Dnf)
	ctx := repoCtx(t)
	const name = "cadestro-sec-dnf-nokey"
	repoFile := dnfRepoFile(name)
	t.Cleanup(func() { _, _ = m.Remove(context.Background(), name) })

	if _, err := m.Apply(ctx, Repository{Name: name, Dnf: &DnfConfig{
		BaseURL: "https://example.com/cadestro-sec-el9", Description: "Cadestro Sec", Enabled: true,
		GPGCheck: false, GPGKey: "https://example.com/cadestro-sec-el9/RPM-GPG-KEY",
	}}); err != nil {
		t.Fatalf("Apply(dnf gpgcheck=0): %v", err)
	}
	body := readFile(t, repoFile)
	if !strings.Contains(body, "gpgcheck=0") {
		t.Errorf(".repo missing gpgcheck=0:\n%s", body)
	}
	if strings.Contains(body, "gpgkey=") {
		t.Errorf("SECURITY: gpgkey= was written behind gpgcheck=0 — the key would be trusted while the repo verifies nothing:\n%s", body)
	}
	if !dnfListsRepo(name) {
		t.Errorf("dnf did not list %q after Apply", name)
	}
}

func TestRepoSecurity_PacmanSigLevelNever_Rejected_Container(t *testing.T) {
	m := realRepoMgr(t, pkg.Pacman)
	ctx := repoCtx(t)
	const name = "cadestro-sec-pacman-never"
	t.Cleanup(func() { _, _ = m.Remove(context.Background(), name) })

	before := readFile(t, pacmanConf)
	_, err := m.Apply(ctx, Repository{Name: name, Pacman: &PacmanConfig{
		Server: "https://example.com/cadestro-sec-arch/$repo/os/$arch", SigLevel: "Never",
	}})
	if !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("Apply(SigLevel Never): got err=%v, want ErrInvalidConfig", err)
	}
	if after := readFile(t, pacmanConf); after != before {
		t.Errorf("a rejected SigLevel Never still mutated /etc/pacman.conf")
	}
	if strings.Contains(readFile(t, pacmanConf), "["+name+"]") {
		t.Errorf("pacman.conf gained a [%s] section despite the rejection", name)
	}
}

func TestRepoSecurity_PacmanReservedOptionsName_Rejected_Container(t *testing.T) {
	m := realRepoMgr(t, pkg.Pacman)
	ctx := repoCtx(t)

	before := readFile(t, pacmanConf)
	_, err := m.Apply(ctx, Repository{Name: "options", Pacman: &PacmanConfig{
		Server: "https://example.com/cadestro-sec-arch/$repo/os/$arch",
	}})
	if !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("Apply(name=options): got err=%v, want ErrInvalidConfig", err)
	}
	if after := readFile(t, pacmanConf); after != before {
		t.Error("a rejected reserved-name Apply mutated the global pacman.conf [options] block")
	}
}

func TestRepoSecurity_PacmanTrustAll_OperatorOverride_Container(t *testing.T) {
	m := realRepoMgr(t, pkg.Pacman)
	ctx := repoCtx(t)
	const name = "cadestro-sec-pacman-trustall"
	t.Cleanup(func() { _, _ = m.Remove(context.Background(), name) })

	if _, err := m.Apply(ctx, Repository{Name: name, Pacman: &PacmanConfig{
		Server: "https://example.com/cadestro-sec-arch/$repo/os/$arch", SigLevel: "Optional TrustAll",
	}}); err != nil {
		t.Fatalf("Apply(Optional TrustAll) is an allowed operator override but failed: %v", err)
	}
	if !pacmanParsesRepo(name) {
		t.Errorf("`pacman-conf --repo %s` failed — pacman did not accept the TrustAll section", name)
	}
}

func TestRepoSecurity_ZypperNoGpgcheck_OperatorOverride_Container(t *testing.T) {
	m := realRepoMgr(t, pkg.Zypper)
	ctx := repoCtx(t)
	const name = "cadestro-sec-zypper-nogpg"
	t.Cleanup(func() { _, _ = m.Remove(context.Background(), name) })

	if _, err := m.Apply(ctx, Repository{Name: name, Zypper: &ZypperConfig{
		URL: "https://example.com/cadestro-sec-suite", Description: "Cadestro Sec", Enabled: false,
		Autorefresh: false, GPGCheck: false,
	}}); err != nil {
		t.Fatalf("Apply(zypper --no-gpgcheck) is an allowed operator override but failed: %v", err)
	}
	if !zypperRepoListed(t, name) {
		t.Errorf("`zypper lr %s` does not list the repo after the operator-override Apply", name)
	}
}
