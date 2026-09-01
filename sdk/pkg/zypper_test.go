package pkg

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	sysexec "github.com/manchtools/cadestro/sdk/sys/exec"
	"github.com/manchtools/cadestro/sdk/sys/exec/exectest"
)

func zypperM(t *testing.T) (Manager, *exectest.FakeRunner) {
	t.Helper()
	return mustNew(t, Zypper)
}

func TestZypper_Version(t *testing.T) {
	t.Run("parses version", func(t *testing.T) {
		m, f := zypperM(t)
		ok(f, "zypper 1.14.59\n")
		v, err := m.Version(context.Background())
		if err != nil || v != "1.14.59" {
			t.Fatalf("v=%q err=%v", v, err)
		}
		if c := f.Calls()[0]; argv(c) != "zypper --version" || c.Escalate {
			t.Errorf("argv=%q escalate=%v", argv(c), c.Escalate)
		}
	})
	t.Run("short output empty", func(t *testing.T) {
		m, f := zypperM(t)
		ok(f, "zypper\n")
		if v, err := m.Version(context.Background()); err != nil || v != "" {
			t.Fatalf("v=%q err=%v", v, err)
		}
	})
	t.Run("exec error", func(t *testing.T) {
		m, f := zypperM(t)
		f.Push(sysexec.Result{}, errors.New("boom"))
		if _, err := m.Version(context.Background()); err == nil {
			t.Fatal("want error")
		}
	})
}

func TestZypper_Install(t *testing.T) {
	ctx := context.Background()
	t.Run("latest", func(t *testing.T) {
		m, f := zypperM(t)
		ok(f, "")
		if _, err := m.Install(ctx, InstallOptions{}, InstallSpec{Name: "vim"}, InstallSpec{Name: "git"}); err != nil {
			t.Fatal(err)
		}
		c := f.Calls()[0]
		if argv(c) != "zypper --non-interactive install vim git" || !c.Escalate {
			t.Errorf("argv=%q escalate=%v", argv(c), c.Escalate)
		}
	})
	t.Run("pinned version", func(t *testing.T) {
		m, f := zypperM(t)
		ok(f, "")
		if _, err := m.Install(ctx, InstallOptions{}, InstallSpec{Name: "vim", Version: "9.0-1"}); err != nil {
			t.Fatal(err)
		}
		if a := argv(f.Calls()[0]); !strings.Contains(a, "vim=9.0-1") {
			t.Errorf("argv=%q", a)
		}
	})
	t.Run("allow downgrade adds --oldpackage", func(t *testing.T) {
		m, f := zypperM(t)
		ok(f, "")
		if _, err := m.Install(ctx, InstallOptions{AllowDowngrade: true}, InstallSpec{Name: "vim", Version: "1.0"}); err != nil {
			t.Fatal(err)
		}
		if a := argv(f.Calls()[0]); !strings.Contains(a, "--oldpackage") {
			t.Errorf("argv=%q want --oldpackage", a)
		}
	})
	t.Run("empty no-op", func(t *testing.T) {
		m, f := zypperM(t)
		if _, err := m.Install(ctx, InstallOptions{}); err != nil || len(f.Calls()) != 0 {
			t.Fatalf("err=%v calls=%d", err, len(f.Calls()))
		}
	})
	t.Run("bad name", func(t *testing.T) {
		m, f := zypperM(t)
		if _, err := m.Install(ctx, InstallOptions{}, InstallSpec{Name: "v;m"}); err == nil || len(f.Calls()) != 0 {
			t.Fatal("want rejection")
		}
	})
	t.Run("bad version", func(t *testing.T) {
		m, f := zypperM(t)
		if _, err := m.Install(ctx, InstallOptions{}, InstallSpec{Name: "vim", Version: "1;0"}); err == nil || len(f.Calls()) != 0 {
			t.Fatal("want rejection")
		}
	})
	t.Run("multiple package versions", func(t *testing.T) {
		m, f := zypperM(t)
		ok(f, "")
		if _, err := m.Install(ctx, InstallOptions{}, InstallSpec{Name: "vim", Version: "1.0"}, InstallSpec{Name: "git", Version: "2.0"}); err != nil {
			t.Fatal(err)
		}
		if got := argv(f.Calls()[0]); got != "zypper --non-interactive install vim=1.0 git=2.0" {
			t.Fatalf("argv=%q", got)
		}
	})
}

func TestZypper_Remove(t *testing.T) {
	ctx := context.Background()
	t.Run("purge is unsupported", func(t *testing.T) {
		m, f := zypperM(t)
		if _, err := m.Remove(ctx, RemoveOptions{Purge: true}, "vim"); !errors.Is(err, ErrUnsupported) {
			t.Fatalf("err=%v, want ErrUnsupported", err)
		}
		if len(f.Calls()) != 0 {
			t.Fatalf("unsupported purge ran %d commands", len(f.Calls()))
		}
	})
	t.Run("empty purge no-op", func(t *testing.T) {
		m, f := zypperM(t)
		if _, err := m.Remove(ctx, RemoveOptions{Purge: true}); err != nil || len(f.Calls()) != 0 {
			t.Fatalf("err=%v calls=%d", err, len(f.Calls()))
		}
	})
	t.Run("bad name", func(t *testing.T) {
		m, f := zypperM(t)
		if _, err := m.Remove(ctx, RemoveOptions{}, "--x"); err == nil || len(f.Calls()) != 0 {
			t.Fatal("want rejection")
		}
	})
}

func TestZypper_UpdateUpgrade(t *testing.T) {
	ctx := context.Background()
	t.Run("update refresh", func(t *testing.T) {
		m, f := zypperM(t)
		ok(f, "")
		if _, err := m.Update(ctx); err != nil {
			t.Fatal(err)
		}
		if argv(f.Calls()[0]) != "zypper --non-interactive refresh" {
			t.Errorf("argv=%q", argv(f.Calls()[0]))
		}
	})
	t.Run("UpgradeAll -> update", func(t *testing.T) {
		m, f := zypperM(t)
		ok(f, "")
		if _, err := m.UpgradeAll(ctx); err != nil {
			t.Fatal(err)
		}
		if argv(f.Calls()[0]) != "zypper --non-interactive update" {
			t.Errorf("argv=%q", argv(f.Calls()[0]))
		}
	})
	for _, code := range []int{102, 103} {
		t.Run(fmt.Sprintf("UpgradeAll informational exit %d", code), func(t *testing.T) {
			m, f := zypperM(t)
			f.Push(sysexec.Result{ExitCode: code}, nil)
			res, err := m.UpgradeAll(ctx)
			if err != nil || res.ExitCode != code {
				t.Fatalf("res=%+v err=%v", res, err)
			}
		})
	}
	t.Run("empty Upgrade is a no-op", func(t *testing.T) {
		m, f := zypperM(t)
		if _, err := m.Upgrade(ctx); err != nil {
			t.Fatal(err)
		}
		if len(f.Calls()) != 0 {
			t.Errorf("empty Upgrade ran %d commands, want 0", len(f.Calls()))
		}
	})
	t.Run("upgrade specific -> update", func(t *testing.T) {
		m, f := zypperM(t)
		ok(f, "")
		if _, err := m.Upgrade(ctx, "vim"); err != nil {
			t.Fatal(err)
		}
		if argv(f.Calls()[0]) != "zypper --non-interactive update vim" {
			t.Errorf("argv=%q", argv(f.Calls()[0]))
		}
	})
	t.Run("upgrade bad name", func(t *testing.T) {
		m, f := zypperM(t)
		if _, err := m.Upgrade(ctx, "v;m"); err == nil || len(f.Calls()) != 0 {
			t.Fatal("want rejection")
		}
	})
	t.Run("write exec error surfaced", func(t *testing.T) {
		m, f := zypperM(t)
		f.Push(sysexec.Result{}, sysexec.ErrEscalationDenied)
		if _, err := m.Update(ctx); !errors.Is(err, sysexec.ErrEscalationDenied) {
			t.Fatalf("err=%v want ErrEscalationDenied", err)
		}
	})
}

func TestZypper_Autoremove(t *testing.T) {
	m, f := zypperM(t)
	if _, err := m.Autoremove(context.Background()); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("err=%v, want ErrUnsupported", err)
	}
	if len(f.Calls()) != 0 {
		t.Errorf("unsupported autoremove ran %d commands", len(f.Calls()))
	}
}
func TestZypper_Search(t *testing.T) {
	ctx := context.Background()
	t.Run("parses table after header", func(t *testing.T) {
		m, f := zypperM(t)
		ok(f, "Loading repository data...\nS | Name | Summary | Type\n---+------+---------+-----\ni | vim | Vi IMproved | package\n  |     |  | \nnopipes\n")
		res, err := m.Search(ctx, "vim")
		if err != nil {
			t.Fatal(err)
		}
		if len(res) != 1 || res[0].Name != "vim" || res[0].Description != "Vi IMproved" {
			t.Fatalf("res=%+v", res)
		}
	})
	t.Run("exit 104 means no matches", func(t *testing.T) {
		m, f := zypperM(t)
		f.Push(sysexec.Result{ExitCode: 104}, nil)
		if res, err := m.Search(ctx, "ghost"); err != nil || res != nil {
			t.Fatalf("res=%v err=%v", res, err)
		}
	})
	t.Run("other non-zero is an error", func(t *testing.T) {
		m, f := zypperM(t)
		f.Push(sysexec.Result{ExitCode: 1, Stderr: "x"}, nil)
		if _, err := m.Search(ctx, "vim"); err == nil {
			t.Fatal("want error")
		}
	})
	t.Run("exec error", func(t *testing.T) {
		m, f := zypperM(t)
		f.Push(sysexec.Result{}, errors.New("boom"))
		if _, err := m.Search(ctx, "vim"); err == nil {
			t.Fatal("want error")
		}
	})
}

func TestZypper_List(t *testing.T) {
	t.Run("parses rpm query with pin", func(t *testing.T) {
		m, f := zypperM(t)
		ok(f, "vim\t9.0-1\tx86_64\t3000\tVi IMproved\nshort\tline\n")
		pkgs, err := m.List(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if len(pkgs) != 1 || pkgs[0].Name != "vim" || pkgs[0].Size != 3000 {
			t.Fatalf("pkgs=%+v", pkgs)
		}
	})
	t.Run("exec error", func(t *testing.T) {
		m, f := zypperM(t)
		f.Push(sysexec.Result{}, errors.New("boom"))
		if _, err := m.List(context.Background()); err == nil {
			t.Fatal("want error")
		}
	})
}

func TestZypper_ListUpgradable(t *testing.T) {
	ctx := context.Background()
	t.Run("parses table", func(t *testing.T) {
		m, f := zypperM(t)
		ok(f, "S | Repository | Name | Current Version | Available Version | Arch\n---+----+----+----+----+----\nv | repo-oss | vim | 9.0-1 | 9.0-2 | x86_64\n  | | | | | \na | b\n")
		ups, err := m.ListUpgradable(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if len(ups) != 1 {
			t.Fatalf("ups=%+v", ups)
		}
		u := ups[0]
		if u.Name != "vim" || u.Repository != "repo-oss" || u.CurrentVersion != "9.0-1" || u.NewVersion != "9.0-2" || u.Architecture != "x86_64" {
			t.Errorf("u=%+v", u)
		}
	})
	t.Run("informational exit code (100) is parsed, not failed", func(t *testing.T) {
		m, f := zypperM(t)
		f.Push(sysexec.Result{ExitCode: 100, Stdout: "S | Repository | Name | Current Version | Available Version | Arch\n---+\nv | repo-oss | vim | 9.0-1 | 9.0-2 | x86_64\n"}, nil)
		ups, err := m.ListUpgradable(ctx)
		if err != nil {
			t.Fatalf("informational exit 100 must not fail: %v", err)
		}
		if len(ups) != 1 || ups[0].Name != "vim" {
			t.Fatalf("ups=%+v", ups)
		}
	})
	t.Run("genuine failure exit code surfaces", func(t *testing.T) {
		m, f := zypperM(t)
		f.Push(sysexec.Result{ExitCode: 6, Stderr: "no repositories"}, nil)
		if _, err := m.ListUpgradable(ctx); err == nil {
			t.Fatal("a non-informational non-zero exit must surface as an error")
		}
	})
	t.Run("exec error", func(t *testing.T) {
		m, f := zypperM(t)
		f.Push(sysexec.Result{}, errors.New("boom"))
		if _, err := m.ListUpgradable(ctx); err == nil {
			t.Fatal("want error")
		}
	})
}

func TestZypper_Show(t *testing.T) {
	ctx := context.Background()
	info := "Information for package vim:\nRepository     : repo-oss\nName           : vim\nVersion        : 9.0-2\nArch           : x86_64\nInstalled Size : 3.0 MiB\nStatus         : up-to-date (installed)\nSummary        : Vi IMproved\n"
	t.Run("installed", func(t *testing.T) {
		m, f := zypperM(t)
		ok(f, info)
		f.Push(sysexec.Result{ExitCode: 0}, nil)
		p, err := m.Show(ctx, "vim")
		if err != nil {
			t.Fatal(err)
		}
		if p.Status != PackageStatusInstalled || p.Version != "9.0-2" || p.Architecture != "x86_64" || p.Size != 3*1024*1024 || p.Repository != "repo-oss" {
			t.Fatalf("p=%+v", p)
		}
	})
	t.Run("available not installed", func(t *testing.T) {
		m, f := zypperM(t)
		ok(f, "Name : vim\nVersion : 9.0-2\n")
		f.Push(sysexec.Result{ExitCode: 1}, nil)
		ok(f, "")
		p, err := m.Show(ctx, "vim")
		if err != nil {
			t.Fatal(err)
		}
		if p.Status != PackageStatusAvailable {
			t.Fatalf("p=%+v", p)
		}
	})
	t.Run("exec error", func(t *testing.T) {
		m, f := zypperM(t)
		f.Push(sysexec.Result{}, errors.New("boom"))
		if _, err := m.Show(ctx, "vim"); err == nil {
			t.Fatal("want error")
		}
	})
	t.Run("bad name", func(t *testing.T) {
		m, f := zypperM(t)
		if _, err := m.Show(ctx, "v;m"); err == nil || len(f.Calls()) != 0 {
			t.Fatal("want rejection")
		}
	})
}

func TestZypper_ListVersions(t *testing.T) {
	ctx := context.Background()
	t.Run("parses match-exact table", func(t *testing.T) {
		m, f := zypperM(t)
		ok(f, "S | Name | Type | Version | Arch | Repository\n---+----+----+----+----+----\nv | vim | package | 9.0-2 | x86_64 | repo-oss\nv | vim | package | 9.0-2 | i586 | repo-oss\ni | other | package | 1.0 | x86_64 | repo\nshort | line\n")
		ok(f, "9.0-1\n")
		info, err := m.ListVersions(ctx, "vim")
		if err != nil {
			t.Fatal(err)
		}
		if info.Installed != "9.0-1" || len(info.Versions) != 1 || info.Versions[0].Repository != "repo-oss" {
			t.Fatalf("info=%+v", info)
		}
	})
	t.Run("no match (exit 104) returns info without versions", func(t *testing.T) {
		m, f := zypperM(t)
		f.Push(sysexec.Result{ExitCode: 104}, nil)
		ok(f, "9.0-1\n")
		info, err := m.ListVersions(ctx, "vim")
		if err != nil {
			t.Fatalf("exit 104 must not fail: %v", err)
		}
		if info.Installed != "9.0-1" || len(info.Versions) != 0 {
			t.Fatalf("info=%+v", info)
		}
	})
	t.Run("genuine search failure surfaces", func(t *testing.T) {
		m, f := zypperM(t)
		f.Push(sysexec.Result{ExitCode: 6, Stderr: "no repos"}, nil)
		ok(f, "9.0-1\n")
		if _, err := m.ListVersions(ctx, "vim"); err == nil {
			t.Fatal("a non-0/104 search exit must surface as an error")
		}
	})
	t.Run("installed-version runner failure propagates", func(t *testing.T) {
		m, f := zypperM(t)
		ok(f, "S | Name | Type | Version | Arch | Repository\n---\nv | vim | package | 9.0-2 | x86_64 | repo-oss\n")
		f.Push(sysexec.Result{}, errors.New("rpm"))
		if _, err := m.ListVersions(ctx, "vim"); err == nil {
			t.Fatal("a runner failure in the installed-version lookup must propagate")
		}
	})
	t.Run("exec error", func(t *testing.T) {
		m, f := zypperM(t)
		f.Push(sysexec.Result{}, errors.New("boom"))
		if _, err := m.ListVersions(ctx, "vim"); err == nil {
			t.Fatal("want error")
		}
	})
	t.Run("bad name", func(t *testing.T) {
		m, f := zypperM(t)
		if _, err := m.ListVersions(ctx, "v;m"); err == nil || len(f.Calls()) != 0 {
			t.Fatal("want rejection")
		}
	})
}

func TestZypper_IsInstalledVersionCount(t *testing.T) {
	ctx := context.Background()
	t.Run("installed", func(t *testing.T) {
		m, f := zypperM(t)
		f.Push(sysexec.Result{ExitCode: 0}, nil)
		if got, err := m.IsInstalled(ctx, "vim"); err != nil || !got {
			t.Fatalf("got=%v err=%v", got, err)
		}
	})
	t.Run("not installed", func(t *testing.T) {
		m, f := zypperM(t)
		f.Push(sysexec.Result{ExitCode: 1}, nil)
		if got, err := m.IsInstalled(ctx, "ghost"); err != nil || got {
			t.Fatalf("got=%v err=%v", got, err)
		}
	})
	t.Run("IsInstalled exec error", func(t *testing.T) {
		m, f := zypperM(t)
		f.Push(sysexec.Result{}, errors.New("boom"))
		if _, err := m.IsInstalled(ctx, "vim"); err == nil {
			t.Fatal("want error")
		}
	})
	t.Run("IsInstalled bad name", func(t *testing.T) {
		m, f := zypperM(t)
		if _, err := m.IsInstalled(ctx, "v;m"); err == nil || len(f.Calls()) != 0 {
			t.Fatal("want rejection")
		}
	})
	t.Run("InstalledVersion present", func(t *testing.T) {
		m, f := zypperM(t)
		f.Push(sysexec.Result{ExitCode: 0, Stdout: "9.0-2\n"}, nil)
		if v, err := m.InstalledVersion(ctx, "vim"); err != nil || v != "9.0-2" {
			t.Fatalf("v=%q err=%v", v, err)
		}
	})
	t.Run("InstalledVersion not installed -> ErrNotFound", func(t *testing.T) {
		m, f := zypperM(t)
		f.Push(sysexec.Result{ExitCode: 1}, nil)
		if v, err := m.InstalledVersion(ctx, "ghost"); !errors.Is(err, ErrNotFound) || v != "" {
			t.Fatalf("v=%q err=%v", v, err)
		}
	})
	t.Run("InstalledVersion exec error", func(t *testing.T) {
		m, f := zypperM(t)
		f.Push(sysexec.Result{}, errors.New("boom"))
		if _, err := m.InstalledVersion(ctx, "vim"); err == nil {
			t.Fatal("want error")
		}
	})
	t.Run("InstalledVersion bad name", func(t *testing.T) {
		m, f := zypperM(t)
		if _, err := m.InstalledVersion(ctx, "v;m"); err == nil || len(f.Calls()) != 0 {
			t.Fatal("want rejection")
		}
	})
	t.Run("InstalledCount", func(t *testing.T) {
		m, f := zypperM(t)
		ok(f, ".\n.\n")
		if n, err := m.InstalledCount(ctx); err != nil || n != 2 {
			t.Fatalf("n=%d err=%v", n, err)
		}
	})
	t.Run("InstalledCount exec error", func(t *testing.T) {
		m, f := zypperM(t)
		f.Push(sysexec.Result{}, errors.New("boom"))
		if _, err := m.InstalledCount(ctx); err == nil {
			t.Fatal("want error")
		}
	})
}

func TestZypper_HasUpdates(t *testing.T) {
	ctx := context.Background()
	t.Run("exit 100 means updates", func(t *testing.T) {
		m, f := zypperM(t)
		f.Push(sysexec.Result{ExitCode: 100}, nil)
		if got, err := m.HasUpdates(ctx); err != nil || !got {
			t.Fatalf("got=%v err=%v", got, err)
		}
	})
	t.Run("exit 101 means security updates", func(t *testing.T) {
		m, f := zypperM(t)
		f.Push(sysexec.Result{ExitCode: 101}, nil)
		if got, err := m.HasUpdates(ctx); err != nil || !got {
			t.Fatalf("got=%v err=%v", got, err)
		}
	})
	t.Run("exit 0 with table row means updates", func(t *testing.T) {
		m, f := zypperM(t)
		f.Push(sysexec.Result{ExitCode: 0, Stdout: "S | Repo | Name\nv | repo | vim\n"}, nil)
		if got, err := m.HasUpdates(ctx); err != nil || !got {
			t.Fatalf("got=%v err=%v", got, err)
		}
	})
	t.Run("exit 0 no rows means none", func(t *testing.T) {
		m, f := zypperM(t)
		f.Push(sysexec.Result{ExitCode: 0, Stdout: "No updates found.\n"}, nil)
		if got, err := m.HasUpdates(ctx); err != nil || got {
			t.Fatalf("got=%v err=%v", got, err)
		}
	})
	t.Run("a real-error exit fails closed, not silent 'no updates'", func(t *testing.T) {
		m, f := zypperM(t)
		f.Push(sysexec.Result{ExitCode: 6}, nil)
		got, err := m.HasUpdates(ctx)

		if err == nil {
			t.Fatal("a failed check must error, not report 'no updates' (compliance fail-open)")
		}
		if got {
			t.Errorf("got=%v, want false on a failed check", got)
		}
	})
	t.Run("security flag added", func(t *testing.T) {
		m, f := zypperM(t)
		f.Push(sysexec.Result{ExitCode: 100}, nil)
		if _, err := m.HasSecurityUpdates(ctx); err != nil {
			t.Fatal(err)
		}
		if a := argv(f.Calls()[0]); !strings.Contains(a, "--category security") {
			t.Errorf("argv=%q want security category", a)
		}
	})
	t.Run("security exit 101 means updates", func(t *testing.T) {
		m, f := zypperM(t)
		f.Push(sysexec.Result{ExitCode: 101}, nil)
		if got, err := m.HasSecurityUpdates(ctx); err != nil || !got {
			t.Fatalf("got=%v err=%v", got, err)
		}
	})
	t.Run("exec error", func(t *testing.T) {
		m, f := zypperM(t)
		f.Push(sysexec.Result{}, errors.New("boom"))
		if _, err := m.HasUpdates(ctx); err == nil {
			t.Fatal("want error")
		}
	})
}

func TestZypper_PinUnpin(t *testing.T) {
	ctx := context.Background()
	t.Run("pin addlock", func(t *testing.T) {
		m, f := zypperM(t)
		ok(f, "")
		if _, err := m.Pin(ctx, "vim"); err != nil {
			t.Fatal(err)
		}
		if c := f.Calls()[0]; argv(c) != "zypper --non-interactive addlock vim" || !c.Escalate {
			t.Errorf("argv=%q escalate=%v", argv(c), c.Escalate)
		}
	})
	t.Run("unpin removelock", func(t *testing.T) {
		m, f := zypperM(t)
		ok(f, "")
		if _, err := m.Unpin(ctx, "vim"); err != nil {
			t.Fatal(err)
		}
		if argv(f.Calls()[0]) != "zypper --non-interactive removelock vim" {
			t.Errorf("argv=%q", argv(f.Calls()[0]))
		}
	})
	t.Run("pin empty no-op", func(t *testing.T) {
		m, f := zypperM(t)
		if _, err := m.Pin(ctx); err != nil || len(f.Calls()) != 0 {
			t.Fatalf("err=%v calls=%d", err, len(f.Calls()))
		}
	})
	t.Run("unpin empty no-op", func(t *testing.T) {
		m, f := zypperM(t)
		if _, err := m.Unpin(ctx); err != nil || len(f.Calls()) != 0 {
			t.Fatalf("err=%v calls=%d", err, len(f.Calls()))
		}
	})
	t.Run("pin bad name", func(t *testing.T) {
		m, f := zypperM(t)
		if _, err := m.Pin(ctx, "v;m"); err == nil || len(f.Calls()) != 0 {
			t.Fatal("want rejection")
		}
	})
	t.Run("unpin bad name", func(t *testing.T) {
		m, f := zypperM(t)
		if _, err := m.Unpin(ctx, "v;m"); err == nil || len(f.Calls()) != 0 {
			t.Fatal("want rejection")
		}
	})
}

func TestZypper_ListPinnedAndIsPinned(t *testing.T) {
	ctx := context.Background()
	t.Run("ListPinned with versions", func(t *testing.T) {
		m, f := zypperM(t)
		ok(f, "# | Name | Type | Repository\n---+------+------+-----------\n1 | vim | package |\n2 | git | package |\nnotanumber | foo |\n")
		ok(f, "9.0-1\n")
		ok(f, "2.39-1\n")
		pkgs, err := m.ListPinned(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if len(pkgs) != 2 || pkgs[0].Name != "vim" || pkgs[0].Version != "9.0-1" {
			t.Fatalf("pkgs=%+v", pkgs)
		}
	})
	t.Run("ListPinned exec error", func(t *testing.T) {
		m, f := zypperM(t)
		f.Push(sysexec.Result{}, errors.New("boom"))
		if _, err := m.ListPinned(ctx); err == nil {
			t.Fatal("want error")
		}
	})
	t.Run("IsPinned true", func(t *testing.T) {
		m, f := zypperM(t)
		ok(f, "# | Name |\n--+\n1 | vim |\n")
		if got, err := m.IsPinned(ctx, "vim"); err != nil || !got {
			t.Fatalf("got=%v err=%v", got, err)
		}
	})
	t.Run("IsPinned false", func(t *testing.T) {
		m, f := zypperM(t)
		ok(f, "1 | git |\n")
		if got, err := m.IsPinned(ctx, "vim"); err != nil || got {
			t.Fatalf("got=%v err=%v", got, err)
		}
	})
	t.Run("IsPinned tolerant of locks failure", func(t *testing.T) {
		m, f := zypperM(t)
		f.Push(sysexec.Result{ExitCode: 1}, nil)
		if got, err := m.IsPinned(ctx, "vim"); err != nil || got {
			t.Fatalf("got=%v err=%v", got, err)
		}
	})
	t.Run("IsPinned bad name", func(t *testing.T) {
		m, f := zypperM(t)
		if _, err := m.IsPinned(ctx, "v;m"); err == nil || len(f.Calls()) != 0 {
			t.Fatal("want rejection")
		}
	})
}

func TestZypper_ParseValueAndSize(t *testing.T) {
	if v := parseColonValue("Version : 9.0"); v != "9.0" {
		t.Errorf("parseColonValue=%q", v)
	}
	if v := parseColonValue("no colon"); v != "" {
		t.Errorf("parseColonValue no-colon=%q", v)
	}
	cases := map[string]int64{
		"3.0 MiB": 3 * 1024 * 1024,
		"512 KiB": 512 * 1024,
		"2 GiB":   2 * 1024 * 1024 * 1024,
		"900 B":   900,
		"42":      42,
	}
	for in, want := range cases {
		got, sizeOK := parseZypperSize(in)
		if !sizeOK {
			t.Errorf("parseZypperSize(%q) reported a parse failure on valid input", in)
			continue
		}
		if got != want {
			t.Errorf("parseZypperSize(%q)=%d want %d", in, got, want)
		}
	}

	for _, in := range []string{"", "unknown", "3.0 MiB extra"} {
		if got, sizeOK := parseZypperSize(in); sizeOK {
			t.Errorf("parseZypperSize(%q)=(%d, true), want ok=false", in, got)
		} else if got != 0 {
			t.Errorf("parseZypperSize(%q) failed but returned %d, want 0", in, got)
		}
	}
}

func TestZypper_EnrichmentRunnerFailuresPropagate(t *testing.T) {
	ctx := context.Background()
	t.Run("Show: IsInstalled runner failure", func(t *testing.T) {
		m, f := zypperM(t)
		ok(f, "Name : vim\nVersion : 9.0-2\n")
		f.Push(sysexec.Result{}, errors.New("rpm"))
		if _, err := m.Show(ctx, "vim"); err == nil {
			t.Fatal("an IsInstalled runner failure must propagate")
		}
	})
	t.Run("ListPinned: InstalledVersion runner failure", func(t *testing.T) {
		m, f := zypperM(t)
		ok(f, "# | Name |\n---+\n1 | vim | package |\n")
		f.Push(sysexec.Result{}, errors.New("rpm"))
		if _, err := m.ListPinned(ctx); err == nil {
			t.Fatal("an InstalledVersion runner failure must propagate")
		}
	})
}
