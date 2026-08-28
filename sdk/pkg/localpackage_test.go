package pkg

import (
	"context"
	"strings"
	"testing"

	sysexec "github.com/manchtools/cadestro/sdk/sys/exec"
	"github.com/manchtools/cadestro/sdk/sys/exec/exectest"
)

func TestLocalPackageInfo_AptHappyPath(t *testing.T) {
	m, f := aptM(t)

	ok(f, "Package: nginx\nVersion: 1.24.0-1ubuntu1\nArchitecture: amd64\n")
	info, err := m.LocalPackageInfo(context.Background(), "/tmp/nginx.deb")
	if err != nil {
		t.Fatalf("LocalPackageInfo err = %v", err)
	}
	if info.Name != "nginx" || info.Version != "1.24.0-1ubuntu1" || info.Arch != "amd64" {
		t.Errorf("info = %+v, want {nginx 1.24.0-1ubuntu1 amd64}", info)
	}
	c := f.Calls()[0]
	want := "dpkg-deb -f /tmp/nginx.deb Package Version Architecture"
	if argv(c) != want {
		t.Errorf("argv = %q, want %q", argv(c), want)
	}
	if c.Escalate {
		t.Error("reading a local package file is an unprivileged read; must NOT escalate")
	}
}

func TestLocalPackageInfo_RpmHappyPath(t *testing.T) {
	for _, tc := range []struct {
		name string
		mk   func(t *testing.T) (Manager, *exectest.FakeRunner)
	}{
		{"dnf", dnfM},
		{"zypper", zypperM},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m, f := tc.mk(t)

			ok(f, "httpd\n2.4.57-5.fc39\nx86_64\n")
			info, err := m.LocalPackageInfo(context.Background(), "/tmp/httpd.rpm")
			if err != nil {
				t.Fatalf("LocalPackageInfo err = %v", err)
			}
			if info.Name != "httpd" || info.Version != "2.4.57-5.fc39" || info.Arch != "x86_64" {
				t.Errorf("info = %+v, want {httpd 2.4.57-5.fc39 x86_64}", info)
			}
			c := f.Calls()[0]
			if c.Name != "rpm" {
				t.Fatalf("command = %q, want rpm", c.Name)
			}
			joined := argv(c)
			for _, frag := range []string{"-qp", "--qf", "%{NAME}", "/tmp/httpd.rpm"} {
				if !strings.Contains(joined, frag) {
					t.Errorf("argv = %q, missing %q", joined, frag)
				}
			}
			if c.Escalate {
				t.Error("rpm -qp on a local file is an unprivileged read; must NOT escalate")
			}
		})
	}
}

func TestLocalPackageInfo_PacmanHappyPath(t *testing.T) {
	m, f := pacmanM(t)
	ok(f, "neovim 0.9.5-1\n")
	info, err := m.LocalPackageInfo(context.Background(), "/tmp/neovim.pkg.tar.zst")
	if err != nil {
		t.Fatalf("LocalPackageInfo err = %v", err)
	}
	if info.Name != "neovim" || info.Version != "0.9.5-1" {
		t.Errorf("info = %+v, want name=neovim version=0.9.5-1", info)
	}
	c := f.Calls()[0]
	want := "pacman -Qp /tmp/neovim.pkg.tar.zst"
	if argv(c) != want {
		t.Errorf("argv = %q, want %q", argv(c), want)
	}
	if c.Escalate {
		t.Error("pacman -Qp on a local file is an unprivileged read; must NOT escalate")
	}
}

func TestLocalPackageInfo_PacmanRejectsNamelessOutput(t *testing.T) {
	cases := map[string]string{
		"leading-space version": " 1.0-1\n",
		"leading-tab version":   "\t1.0-1\n",
		"whitespace only":       "   \n",
		"empty":                 "\n",
	}
	for name, out := range cases {
		t.Run(name, func(t *testing.T) {
			m, f := pacmanM(t)
			ok(f, out)
			info, err := m.LocalPackageInfo(context.Background(), "/tmp/x.pkg.tar.zst")
			if err == nil {
				t.Fatalf("accepted nameless -Qp output %q as info=%+v; want a no-name rejection", out, info)
			}
			if info != nil {
				t.Errorf("info = %+v, want nil on rejection", info)
			}
		})
	}
}

func TestLocalPackageInfo_RejectsBadPath(t *testing.T) {
	for _, mk := range []func(t *testing.T) (Manager, *exectest.FakeRunner){aptM, dnfM, zypperM, pacmanM} {
		m, f := mk(t)
		for _, path := range []string{
			"",
			"relative.deb",
			"/tmp/../etc/x",
			"/tmp/a\nb.deb",
			"-rf",
		} {
			_, err := m.LocalPackageInfo(context.Background(), path)
			if err == nil {
				t.Errorf("%T LocalPackageInfo(%q) = nil err, want path rejection", m, path)
			}
		}
		if n := len(f.Calls()); n != 0 {
			t.Errorf("%T ran %d commands on rejected paths, want 0", m, n)
		}
	}
}

func TestLocalPackageInfo_RejectsCraftedName(t *testing.T) {

	craftedNames := []string{
		"-rf",
		"--eval=%x",
		"evil name",
		"pkg;id",
		"pkg$(whoami)",
		"pkg|tee",
		"pkg`id`",
		"",
	}

	t.Run("apt rejects a crafted Package field", func(t *testing.T) {
		for _, bad := range craftedNames {
			m, f := aptM(t)

			ok(f, bad+"\n1.0\namd64\n")
			info, err := m.LocalPackageInfo(context.Background(), "/tmp/evil.deb")
			if err == nil {
				t.Errorf("crafted Package %q: err = nil, want rejection (info=%+v)", bad, info)
			}
			if info != nil {
				t.Errorf("crafted Package %q: info = %+v, want nil", bad, info)
			}
		}
	})

	t.Run("dnf/zypper reject a crafted %{NAME}", func(t *testing.T) {
		for _, mk := range []func(t *testing.T) (Manager, *exectest.FakeRunner){dnfM, zypperM} {
			for _, bad := range craftedNames {
				m, f := mk(t)
				ok(f, bad+"\n2.4.57-5.fc39\nx86_64\n")
				info, err := m.LocalPackageInfo(context.Background(), "/tmp/evil.rpm")
				if err == nil {
					t.Errorf("%T crafted %%{NAME} %q: err = nil, want rejection (info=%+v)", m, bad, info)
				}
				if info != nil {
					t.Errorf("%T crafted %%{NAME} %q: info = %+v, want nil", m, bad, info)
				}
			}
		}
	})

	t.Run("pacman rejects a crafted name", func(t *testing.T) {

		for _, bad := range craftedNames {
			if bad == "" || strings.ContainsAny(bad, " ") {
				continue
			}
			m, f := pacmanM(t)
			ok(f, bad+" 1.0-1\n")
			info, err := m.LocalPackageInfo(context.Background(), "/tmp/evil.pkg.tar.zst")
			if err == nil {
				t.Errorf("pacman crafted name %q: err = nil, want rejection (info=%+v)", bad, info)
			}
			if info != nil {
				t.Errorf("pacman crafted name %q: info = %+v, want nil", bad, info)
			}
		}
	})

	t.Run("pacman rejects empty -Qp output (no name)", func(t *testing.T) {

		for _, out := range []string{"", "\n", "   \n"} {
			m, f := pacmanM(t)
			ok(f, out)
			info, err := m.LocalPackageInfo(context.Background(), "/tmp/evil.pkg.tar.zst")
			if err == nil {
				t.Errorf("pacman empty -Qp %q: err = nil, want rejection (info=%+v)", out, info)
			}
			if info != nil {
				t.Errorf("pacman empty -Qp %q: info = %+v, want nil", out, info)
			}
		}
	})
}

func TestLocalPackageInfo_AcceptsRpmPlusName(t *testing.T) {
	for _, mk := range []func(t *testing.T) (Manager, *exectest.FakeRunner){dnfM, zypperM} {
		m, f := mk(t)
		ok(f, "libstdc++\n13.2.1-4.fc39\nx86_64\n")
		info, err := m.LocalPackageInfo(context.Background(), "/tmp/libstdc++.rpm")
		if err != nil {
			t.Fatalf("%T legitimate '+'-bearing RPM name rejected: %v", m, err)
		}
		if info.Name != "libstdc++" {
			t.Errorf("%T info.Name = %q, want libstdc++", m, info.Name)
		}
	}
}

func TestLocalPackageInfo_ReadFailurePropagates(t *testing.T) {
	m, f := aptM(t)
	f.Push(sysexec.Result{ExitCode: 2, Stderr: "dpkg-deb: error: not a debian archive\n"}, nil)
	info, err := m.LocalPackageInfo(context.Background(), "/tmp/not-a.deb")
	if err == nil {
		t.Fatal("a non-zero dpkg-deb exit must surface as an error")
	}
	if info != nil {
		t.Errorf("info = %+v, want nil on read failure", info)
	}
}
