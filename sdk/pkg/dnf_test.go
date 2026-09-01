package pkg

import (
	"context"
	"errors"
	"strings"
	"testing"

	sysexec "github.com/manchtools/cadestro/sdk/sys/exec"
	"github.com/manchtools/cadestro/sdk/sys/exec/exectest"
)

func dnfM(t *testing.T) (Manager, *exectest.FakeRunner) {
	t.Helper()
	return mustNew(t, Dnf)
}

func dnf5M(t *testing.T) (Manager, *exectest.FakeRunner) {
	t.Helper()
	return mustNew(t, Dnf5)
}

func TestDnf_Version(t *testing.T) {
	t.Run("first line", func(t *testing.T) {
		m, f := dnfM(t)
		ok(f, "4.18.2\n Installed: dnf-4.18.2\n")
		v, err := m.Version(context.Background())
		if err != nil || v != "4.18.2" {
			t.Fatalf("v=%q err=%v", v, err)
		}
		if c := f.Calls()[0]; argv(c) != "dnf --version" || c.Escalate {
			t.Errorf("argv=%q escalate=%v", argv(c), c.Escalate)
		}
	})
	t.Run("exec error", func(t *testing.T) {
		m, f := dnfM(t)
		f.Push(sysexec.Result{}, errors.New("boom"))
		if _, err := m.Version(context.Background()); err == nil {
			t.Fatal("want error")
		}
	})
}

func TestDnf_Install(t *testing.T) {
	ctx := context.Background()
	t.Run("multiple latest", func(t *testing.T) {
		m, f := dnfM(t)
		ok(f, "")
		if _, err := m.Install(ctx, InstallOptions{}, InstallSpec{Name: "vim"}, InstallSpec{Name: "git"}); err != nil {
			t.Fatal(err)
		}
		c := f.Calls()[0]
		if argv(c) != "dnf install -y vim git" || !c.Escalate {
			t.Errorf("argv=%q escalate=%v", argv(c), c.Escalate)
		}
	})
	t.Run("pinned version uses name-version", func(t *testing.T) {
		m, f := dnfM(t)
		ok(f, "")
		if _, err := m.Install(ctx, InstallOptions{}, InstallSpec{Name: "vim", Version: "8.2.3"}); err != nil {
			t.Fatal(err)
		}
		if a := argv(f.Calls()[0]); !strings.Contains(a, "vim-8.2.3") {
			t.Errorf("argv=%q want name-version", a)
		}
	})
	t.Run("dnf4 exact downgrade spec", func(t *testing.T) {
		m, f := dnfM(t)
		ok(f, "")
		if _, err := m.Install(ctx, InstallOptions{AllowDowngrade: true}, InstallSpec{Name: "vim", Version: "1.0"}); err != nil {
			t.Fatal(err)
		}
		if got := argv(f.Calls()[0]); got != "dnf install -y vim-1.0" {
			t.Fatalf("argv=%q", got)
		}
	})
	t.Run("install failure without downgrade is returned", func(t *testing.T) {
		m, f := dnfM(t)
		f.Push(sysexec.Result{ExitCode: 1, Stderr: "no package"}, nil)
		_, err := m.Install(ctx, InstallOptions{}, InstallSpec{Name: "ghost"})
		var ce *sysexec.CommandError
		if !errors.As(err, &ce) || ce.ExitCode != 1 {
			t.Fatalf("err=%v want CommandError", err)
		}
		if len(f.Calls()) != 1 {
			t.Error("must not retry without AllowDowngrade")
		}
	})
	t.Run("empty no-op", func(t *testing.T) {
		m, f := dnfM(t)
		if _, err := m.Install(ctx, InstallOptions{}); err != nil || len(f.Calls()) != 0 {
			t.Fatalf("err=%v calls=%d", err, len(f.Calls()))
		}
	})
	t.Run("bad name", func(t *testing.T) {
		m, f := dnfM(t)
		if _, err := m.Install(ctx, InstallOptions{}, InstallSpec{Name: "v;m"}); err == nil || len(f.Calls()) != 0 {
			t.Fatal("want rejection, no exec")
		}
	})
	t.Run("bad version", func(t *testing.T) {
		m, f := dnfM(t)
		if _, err := m.Install(ctx, InstallOptions{}, InstallSpec{Name: "vim", Version: "1;0"}); err == nil || len(f.Calls()) != 0 {
			t.Fatal("want version rejection, no exec")
		}
	})
	t.Run("multiple package versions", func(t *testing.T) {
		m, f := dnfM(t)
		ok(f, "")
		if _, err := m.Install(ctx, InstallOptions{}, InstallSpec{Name: "vim", Version: "1.0"}, InstallSpec{Name: "git", Version: "2.0"}); err != nil {
			t.Fatal(err)
		}
		if got := argv(f.Calls()[0]); got != "dnf install -y vim-1.0 git-2.0" {
			t.Fatalf("argv=%q", got)
		}
	})
}

func TestDnf5_AllowDowngrade(t *testing.T) {
	m, f := dnf5M(t)
	ok(f, "")
	if _, err := m.Install(context.Background(), InstallOptions{AllowDowngrade: true}, InstallSpec{Name: "vim", Version: "1.0"}, InstallSpec{Name: "git", Version: "2.0"}); err != nil {
		t.Fatal(err)
	}
	if got := argv(f.Calls()[0]); got != "dnf5 install -y --allow-downgrade vim-1.0 git-2.0" {
		t.Fatalf("argv=%q", got)
	}
}

func TestDnf_Remove(t *testing.T) {
	ctx := context.Background()
	t.Run("purge is unsupported", func(t *testing.T) {
		m, f := dnfM(t)
		if _, err := m.Remove(ctx, RemoveOptions{Purge: true}, "vim"); !errors.Is(err, ErrUnsupported) {
			t.Fatalf("err=%v, want ErrUnsupported", err)
		}
		if len(f.Calls()) != 0 {
			t.Fatalf("unsupported purge ran %d commands", len(f.Calls()))
		}
	})
	t.Run("empty purge no-op", func(t *testing.T) {
		m, f := dnfM(t)
		if _, err := m.Remove(ctx, RemoveOptions{Purge: true}); err != nil || len(f.Calls()) != 0 {
			t.Fatalf("err=%v calls=%d", err, len(f.Calls()))
		}
	})
	t.Run("bad name", func(t *testing.T) {
		m, f := dnfM(t)
		if _, err := m.Remove(ctx, RemoveOptions{}, "--x"); err == nil || len(f.Calls()) != 0 {
			t.Fatal("want rejection")
		}
	})
}

func TestDnf_Update(t *testing.T) {
	ctx := context.Background()
	t.Run("exit 0 is success", func(t *testing.T) {
		m, f := dnfM(t)
		f.Push(sysexec.Result{ExitCode: 0}, nil)
		if _, err := m.Update(ctx); err != nil {
			t.Fatal(err)
		}
		if c := f.Calls()[0]; argv(c) != "dnf check-update" || !c.Escalate {
			t.Errorf("argv=%q escalate=%v", argv(c), c.Escalate)
		}
	})
	t.Run("exit 100 (updates available) is success", func(t *testing.T) {
		m, f := dnfM(t)
		f.Push(sysexec.Result{ExitCode: 100}, nil)
		if _, err := m.Update(ctx); err != nil {
			t.Fatalf("exit 100 must be success, got %v", err)
		}
	})
	t.Run("other non-zero is an error", func(t *testing.T) {
		m, f := dnfM(t)
		f.Push(sysexec.Result{ExitCode: 5, Stderr: "metadata error"}, nil)
		var ce *sysexec.CommandError
		if _, err := m.Update(ctx); !errors.As(err, &ce) || ce.ExitCode != 5 {
			t.Fatalf("err=%v want CommandError(5)", err)
		}
	})
	t.Run("exec error", func(t *testing.T) {
		m, f := dnfM(t)
		f.Push(sysexec.Result{}, errors.New("boom"))
		if _, err := m.Update(ctx); err == nil {
			t.Fatal("want error")
		}
	})
}

func TestDnf_Upgrade(t *testing.T) {
	ctx := context.Background()
	t.Run("UpgradeAll", func(t *testing.T) {
		m, f := dnfM(t)
		ok(f, "")
		if _, err := m.UpgradeAll(ctx); err != nil {
			t.Fatal(err)
		}
		if argv(f.Calls()[0]) != "dnf upgrade -y" {
			t.Errorf("argv=%q", argv(f.Calls()[0]))
		}
	})
	t.Run("empty Upgrade is a no-op", func(t *testing.T) {
		m, f := dnfM(t)
		if _, err := m.Upgrade(ctx); err != nil {
			t.Fatal(err)
		}
		if len(f.Calls()) != 0 {
			t.Errorf("empty Upgrade ran %d commands, want 0", len(f.Calls()))
		}
	})
	t.Run("specific", func(t *testing.T) {
		m, f := dnfM(t)
		ok(f, "")
		if _, err := m.Upgrade(ctx, "vim"); err != nil {
			t.Fatal(err)
		}
		if argv(f.Calls()[0]) != "dnf upgrade -y vim" {
			t.Errorf("argv=%q", argv(f.Calls()[0]))
		}
	})
	t.Run("bad name", func(t *testing.T) {
		m, f := dnfM(t)
		if _, err := m.Upgrade(ctx, "v;m"); err == nil || len(f.Calls()) != 0 {
			t.Fatal("want rejection")
		}
	})
}

func TestDnf_Autoremove(t *testing.T) {
	m, f := dnfM(t)
	ok(f, "")
	if _, err := m.Autoremove(context.Background()); err != nil {
		t.Fatal(err)
	}
	if c := f.Calls()[0]; argv(c) != "dnf autoremove -y" || !c.Escalate {
		t.Errorf("argv=%q escalate=%v", argv(c), c.Escalate)
	}
}

func TestDnf_Search(t *testing.T) {
	ctx := context.Background()
	t.Run("parses 'name.arch : summary'", func(t *testing.T) {
		m, f := dnfM(t)
		ok(f, "================ Name Matched ================\nvim.x86_64 : Vi IMproved\n\nnope\n")
		res, err := m.Search(ctx, "vim")
		if err != nil {
			t.Fatal(err)
		}
		if len(res) != 1 || res[0].Name != "vim" || res[0].Description != "Vi IMproved" {
			t.Fatalf("res=%+v", res)
		}
	})
	t.Run("exit 1 means no matches", func(t *testing.T) {
		m, f := dnfM(t)
		f.Push(sysexec.Result{ExitCode: 1}, nil)
		res, err := m.Search(ctx, "ghost")
		if err != nil || res != nil {
			t.Fatalf("res=%v err=%v", res, err)
		}
	})
	t.Run("other non-zero is an error", func(t *testing.T) {
		m, f := dnfM(t)
		f.Push(sysexec.Result{ExitCode: 2, Stderr: "broken repo"}, nil)
		if _, err := m.Search(ctx, "vim"); err == nil {
			t.Fatal("want error")
		}
	})
	t.Run("exec error", func(t *testing.T) {
		m, f := dnfM(t)
		f.Push(sysexec.Result{}, errors.New("boom"))
		if _, err := m.Search(ctx, "vim"); err == nil {
			t.Fatal("want error")
		}
	})
}

func TestDnf_List(t *testing.T) {
	t.Run("parses rpm query", func(t *testing.T) {
		m, f := dnfM(t)
		ok(f, "vim\t8.2-1\tx86_64\t3000\tVi IMproved\nshort\tline\n")
		pkgs, err := m.List(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if len(pkgs) != 1 || pkgs[0].Name != "vim" || pkgs[0].Size != 3000 {
			t.Fatalf("pkgs=%+v", pkgs)
		}
	})
	t.Run("exec error", func(t *testing.T) {
		m, f := dnfM(t)
		f.Push(sysexec.Result{}, errors.New("boom"))
		if _, err := m.List(context.Background()); err == nil {
			t.Fatal("want error")
		}
	})
}

func TestDnf_ListUpgradable(t *testing.T) {
	ctx := context.Background()
	t.Run("exit 100 then parses rows", func(t *testing.T) {
		m, f := dnfM(t)
		f.Push(sysexec.Result{ExitCode: 100, Stdout: "vim.x86_64 8.2-2 updates\n\nshort line\n"}, nil)
		ok(f, "8.2-1\n")
		ups, err := m.ListUpgradable(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if len(ups) != 1 || ups[0].Name != "vim" || ups[0].Architecture != "x86_64" || ups[0].NewVersion != "8.2-2" || ups[0].CurrentVersion != "8.2-1" {
			t.Fatalf("ups=%+v", ups)
		}
	})
	t.Run("other non-zero is an error", func(t *testing.T) {
		m, f := dnfM(t)
		f.Push(sysexec.Result{ExitCode: 1, Stderr: "x"}, nil)
		if _, err := m.ListUpgradable(ctx); err == nil {
			t.Fatal("want error")
		}
	})
	t.Run("exec error", func(t *testing.T) {
		m, f := dnfM(t)
		f.Push(sysexec.Result{}, errors.New("boom"))
		if _, err := m.ListUpgradable(ctx); err == nil {
			t.Fatal("want error")
		}
	})
}

func TestDnf_Show(t *testing.T) {
	ctx := context.Background()
	t.Run("installed", func(t *testing.T) {
		m, f := dnfM(t)
		ok(f, "Version      : 8.2\nRelease      : 1.fc39\nArchitecture : x86_64\nSize         : 3.0 M\nSummary      : Vi IMproved\nRepository   : updates\n")
		f.Push(sysexec.Result{ExitCode: 0}, nil)
		p, err := m.Show(ctx, "vim")
		if err != nil {
			t.Fatal(err)
		}
		if p.Status != PackageStatusInstalled || p.Version != "8.2-1.fc39" || p.Architecture != "x86_64" || p.Size != 3*1024*1024 {
			t.Fatalf("p=%+v", p)
		}
	})
	t.Run("available not installed", func(t *testing.T) {
		m, f := dnfM(t)
		ok(f, "Version : 8.2\n")
		f.Push(sysexec.Result{ExitCode: 1}, nil)
		p, err := m.Show(ctx, "vim")
		if err != nil {
			t.Fatal(err)
		}
		if p.Status != PackageStatusAvailable {
			t.Fatalf("p=%+v", p)
		}
	})
	t.Run("exec error", func(t *testing.T) {
		m, f := dnfM(t)
		f.Push(sysexec.Result{}, errors.New("boom"))
		if _, err := m.Show(ctx, "vim"); err == nil {
			t.Fatal("want error")
		}
	})
	t.Run("bad name", func(t *testing.T) {
		m, f := dnfM(t)
		if _, err := m.Show(ctx, "v;m"); err == nil || len(f.Calls()) != 0 {
			t.Fatal("want rejection")
		}
	})
}

func TestDnf_ListVersions(t *testing.T) {
	ctx := context.Background()
	t.Run("dedups and skips headers", func(t *testing.T) {
		m, f := dnfM(t)
		ok(f, "Installed Packages\nvim.x86_64 8.2-1 @updates\nAvailable Packages\nvim.x86_64 8.2-2 updates\nvim.x86_64 8.2-2 updates\nshort line\n")
		ok(f, "8.2-1\n")
		info, err := m.ListVersions(ctx, "vim")
		if err != nil {
			t.Fatal(err)
		}
		if info.Installed != "8.2-1" || len(info.Versions) != 2 {
			t.Fatalf("info=%+v", info)
		}
	})
	t.Run("installed-version runner failure propagates", func(t *testing.T) {
		m, f := dnfM(t)
		ok(f, "vim.x86_64 8.2-2 updates\n")
		f.Push(sysexec.Result{}, errors.New("rpm"))
		if _, err := m.ListVersions(ctx, "vim"); err == nil {
			t.Fatal("a runner failure in the installed-version lookup must propagate")
		}
	})
	t.Run("exec error", func(t *testing.T) {
		m, f := dnfM(t)
		f.Push(sysexec.Result{}, errors.New("boom"))
		if _, err := m.ListVersions(ctx, "vim"); err == nil {
			t.Fatal("want error")
		}
	})
	t.Run("bad name", func(t *testing.T) {
		m, f := dnfM(t)
		if _, err := m.ListVersions(ctx, "v;m"); err == nil || len(f.Calls()) != 0 {
			t.Fatal("want rejection")
		}
	})
}

func TestDnf_IsInstalled(t *testing.T) {
	ctx := context.Background()
	t.Run("installed", func(t *testing.T) {
		m, f := dnfM(t)
		f.Push(sysexec.Result{ExitCode: 0}, nil)
		if got, err := m.IsInstalled(ctx, "vim"); err != nil || !got {
			t.Fatalf("got=%v err=%v", got, err)
		}
	})
	t.Run("not installed", func(t *testing.T) {
		m, f := dnfM(t)
		f.Push(sysexec.Result{ExitCode: 1}, nil)
		if got, err := m.IsInstalled(ctx, "ghost"); err != nil || got {
			t.Fatalf("got=%v err=%v", got, err)
		}
	})
	t.Run("exec error", func(t *testing.T) {
		m, f := dnfM(t)
		f.Push(sysexec.Result{}, errors.New("boom"))
		if _, err := m.IsInstalled(ctx, "vim"); err == nil {
			t.Fatal("want error")
		}
	})
	t.Run("bad name", func(t *testing.T) {
		m, f := dnfM(t)
		if _, err := m.IsInstalled(ctx, "v;m"); err == nil || len(f.Calls()) != 0 {
			t.Fatal("want rejection")
		}
	})
}

func TestDnf_InstalledVersion(t *testing.T) {
	ctx := context.Background()
	t.Run("installed", func(t *testing.T) {
		m, f := dnfM(t)
		f.Push(sysexec.Result{ExitCode: 0, Stdout: "8.2-1\n"}, nil)
		if v, err := m.InstalledVersion(ctx, "vim"); err != nil || v != "8.2-1" {
			t.Fatalf("v=%q err=%v", v, err)
		}
	})
	t.Run("not installed -> ErrNotFound", func(t *testing.T) {
		m, f := dnfM(t)
		f.Push(sysexec.Result{ExitCode: 1}, nil)
		if v, err := m.InstalledVersion(ctx, "ghost"); !errors.Is(err, ErrNotFound) || v != "" {
			t.Fatalf("v=%q err=%v", v, err)
		}
	})
	t.Run("exec error", func(t *testing.T) {
		m, f := dnfM(t)
		f.Push(sysexec.Result{}, errors.New("boom"))
		if _, err := m.InstalledVersion(ctx, "vim"); err == nil {
			t.Fatal("want error")
		}
	})
	t.Run("bad name", func(t *testing.T) {
		m, f := dnfM(t)
		if _, err := m.InstalledVersion(ctx, "v;m"); err == nil || len(f.Calls()) != 0 {
			t.Fatal("want rejection")
		}
	})
}

func TestDnf_InstalledCount(t *testing.T) {
	t.Run("counts", func(t *testing.T) {
		m, f := dnfM(t)
		ok(f, ".\n.\n")
		if n, err := m.InstalledCount(context.Background()); err != nil || n != 2 {
			t.Fatalf("n=%d err=%v", n, err)
		}
	})
	t.Run("exec error", func(t *testing.T) {
		m, f := dnfM(t)
		f.Push(sysexec.Result{}, errors.New("boom"))
		if _, err := m.InstalledCount(context.Background()); err == nil {
			t.Fatal("want error")
		}
	})
}

func TestDnf_HasUpdates(t *testing.T) {
	ctx := context.Background()
	t.Run("exit 100 means updates", func(t *testing.T) {
		m, f := dnfM(t)
		f.Push(sysexec.Result{ExitCode: 100}, nil)
		if got, err := m.HasUpdates(ctx); err != nil || !got {
			t.Fatalf("got=%v err=%v", got, err)
		}
	})
	t.Run("exit 0 means none", func(t *testing.T) {
		m, f := dnfM(t)
		f.Push(sysexec.Result{ExitCode: 0}, nil)
		if got, err := m.HasUpdates(ctx); err != nil || got {
			t.Fatalf("got=%v err=%v", got, err)
		}
	})
	t.Run("unexpected exit code surfaces as an error", func(t *testing.T) {
		m, f := dnfM(t)
		f.Push(sysexec.Result{ExitCode: 1, Stderr: "metadata problem"}, nil)
		if _, err := m.HasUpdates(ctx); err == nil {
			t.Fatal("a non-0/100 check-update exit must be surfaced, not reported as 'no updates'")
		}
	})
	t.Run("security flag added", func(t *testing.T) {
		m, f := dnfM(t)
		f.Push(sysexec.Result{ExitCode: 100}, nil)
		if _, err := m.HasSecurityUpdates(ctx); err != nil {
			t.Fatal(err)
		}
		if a := argv(f.Calls()[0]); !strings.Contains(a, "--security") {
			t.Errorf("argv=%q want --security", a)
		}
	})
	t.Run("exec error", func(t *testing.T) {
		m, f := dnfM(t)
		f.Push(sysexec.Result{}, errors.New("boom"))
		if _, err := m.HasUpdates(ctx); err == nil {
			t.Fatal("want error")
		}
	})
}

func TestDnf_VersionLock(t *testing.T) {
	ctx := context.Background()
	t.Run("pin and unpin use dnf", func(t *testing.T) {
		m, f := dnfM(t)
		ok(f, "")
		ok(f, "")
		if _, err := m.Pin(ctx, "vim"); err != nil {
			t.Fatal(err)
		}
		ok(f, "")
		ok(f, "")
		if _, err := m.Unpin(ctx, "vim"); err != nil {
			t.Fatal(err)
		}
		calls := f.Calls()
		if got := argv(calls[1]); got != "dnf versionlock add vim" {
			t.Fatalf("pin argv=%q", got)
		}
		if got := argv(calls[3]); got != "dnf versionlock delete vim" {
			t.Fatalf("unpin argv=%q", got)
		}
	})
	t.Run("dnf5 uses dnf5", func(t *testing.T) {
		m, f := dnf5M(t)
		ok(f, "")
		ok(f, "")
		if _, err := m.Pin(ctx, "vim"); err != nil {
			t.Fatal(err)
		}
		if got := argv(f.Calls()[1]); got != "dnf5 versionlock add vim" {
			t.Fatalf("argv=%q", got)
		}
	})
	t.Run("dnf5 parses package stanzas", func(t *testing.T) {
		m, f := dnf5M(t)
		ok(f, "")
		ok(f, "Package name: vim\nVersion: 8.2\nCondition: >= 8.0\n\n")
		got, err := m.IsPinned(ctx, "vim")
		if err != nil || !got {
			t.Fatalf("isPinned=%v err=%v", got, err)
		}
		ok(f, "")
		ok(f, "Package name: vim\nVersion: 8.2\nCondition: >= 8.0\n")
		ok(f, "8.2\n")
		pkgs, err := m.ListPinned(ctx)
		if err != nil || len(pkgs) != 1 || pkgs[0].Name != "vim" {
			t.Fatalf("pkgs=%+v err=%v", pkgs, err)
		}
	})
	t.Run("absent subcommand is unsupported", func(t *testing.T) {
		m, f := dnfM(t)
		f.Push(sysexec.Result{ExitCode: 1}, nil)
		if _, err := m.Pin(ctx, "vim"); !errors.Is(err, ErrUnsupported) {
			t.Fatalf("err=%v", err)
		}
		if len(f.Calls()) != 1 {
			t.Fatalf("calls=%d", len(f.Calls()))
		}
	})
	t.Run("invalid name before probe", func(t *testing.T) {
		m, f := dnfM(t)
		if _, err := m.Pin(ctx, "v;m"); !errors.Is(err, ErrInvalidArgument) || len(f.Calls()) != 0 {
			t.Fatalf("err=%v calls=%d", err, len(f.Calls()))
		}
	})
	t.Run("runner error propagates", func(t *testing.T) {
		m, f := dnfM(t)
		f.Push(sysexec.Result{}, sysexec.ErrEscalationDenied)
		if _, err := m.Pin(ctx, "vim"); !errors.Is(err, sysexec.ErrEscalationDenied) {
			t.Fatalf("err=%v", err)
		}
	})
	t.Run("is pinned and list pinned", func(t *testing.T) {
		m, f := dnfM(t)
		ok(f, "")
		ok(f, "vim-8.2-1.x86_64\n")
		got, err := m.IsPinned(ctx, "vim")
		if err != nil || !got {
			t.Fatalf("isPinned=%v err=%v", got, err)
		}
		ok(f, "")
		ok(f, "vim-8.2-1.x86_64\n")
		ok(f, "8.2-1\n")
		pkgs, err := m.ListPinned(ctx)
		if err != nil || len(pkgs) != 1 || pkgs[0].Name != "vim" {
			t.Fatalf("pkgs=%+v err=%v", pkgs, err)
		}
	})
}

func TestDnf_NEVRAParsing(t *testing.T) {
	cases := map[string]string{
		"vim-8.2-1.x86_64":      "vim",
		"glibc-langpack-en-2.3": "glibc-langpack-en",
		"noversion":             "noversion",
		"2048-cli-0.9":          "2048-cli",
	}
	for in, want := range cases {
		if got := parseNEVRAName(in); got != want {
			t.Errorf("parseNEVRAName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestDnf_ParseSize(t *testing.T) {
	cases := map[string]int64{
		"3.0 M": 3 * 1024 * 1024,
		"512 k": 512 * 1024,
		"2 G":   2 * 1024 * 1024 * 1024,
		"100":   100,
	}
	for in, want := range cases {
		got, ok := parseSize(in)
		if !ok {
			t.Errorf("parseSize(%q) reported a parse failure on valid input", in)
			continue
		}
		if got != want {
			t.Errorf("parseSize(%q) = %d, want %d", in, got, want)
		}
	}

	for _, in := range []string{
		"",
		"bad MB",
		"unknown",
		"3.0 MB extra text",
	} {
		if got, ok := parseSize(in); ok {
			t.Errorf("parseSize(%q) = (%d, true), want ok=false for unparseable input", in, got)
		} else if got != 0 {
			t.Errorf("parseSize(%q) failed but returned %d, want 0", in, got)
		}
	}
}

func TestDnf_EnrichmentRunnerFailuresPropagate(t *testing.T) {
	ctx := context.Background()
	t.Run("ListUpgradable: InstalledVersion runner failure", func(t *testing.T) {
		m, f := dnfM(t)
		f.Push(sysexec.Result{ExitCode: 100, Stdout: "vim.x86_64 8.2-2 updates\n"}, nil)
		f.Push(sysexec.Result{}, errors.New("rpm"))
		if _, err := m.ListUpgradable(ctx); err == nil {
			t.Fatal("an InstalledVersion runner failure must propagate")
		}
	})
	t.Run("Show: IsInstalled runner failure", func(t *testing.T) {
		m, f := dnfM(t)
		ok(f, "Version : 8.2\n")
		f.Push(sysexec.Result{}, errors.New("rpm"))
		if _, err := m.Show(ctx, "vim"); err == nil {
			t.Fatal("an IsInstalled runner failure must propagate")
		}
	})
	t.Run("ListPinned: InstalledVersion runner failure", func(t *testing.T) {
		m, f := dnfM(t)
		f.Push(sysexec.Result{ExitCode: 0}, nil)
		ok(f, "vim-8.2-1.x86_64\n")
		f.Push(sysexec.Result{}, errors.New("rpm"))
		if _, err := m.ListPinned(ctx); err == nil {
			t.Fatal("an InstalledVersion runner failure must propagate")
		}
	})
}
