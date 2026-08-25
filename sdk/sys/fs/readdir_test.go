package fs

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	sysexec "github.com/manchtools/cadestro/sdk/sys/exec"
	"github.com/manchtools/cadestro/sdk/sys/exec/exectest"
)

func names(entries []DirEntry) []string {
	out := make([]string, len(entries))
	for i, e := range entries {
		kind := "f"
		if e.IsDir {
			kind = "d"
		}
		out[i] = e.Name + "(" + kind + ")"
	}
	sort.Strings(out)
	return out
}

func TestReadDir_Sudo_ParsesFindOutput(t *testing.T) {
	f := exectest.New(sysexec.Sudo)

	f.Push(sysexec.Result{Stdout: "f/foo.sources\nd/sub\nl/legacy.list\n"}, nil)
	m := mustManager(t, f)

	got, err := m.ReadDir(context.Background(), "/etc/apt/sources.list.d")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"foo.sources(f)", "legacy.list(f)", "sub(d)"}
	if strings.Join(names(got), ",") != strings.Join(want, ",") {
		t.Errorf("entries = %v, want %v", names(got), want)
	}

	if got := argv(f.Calls()[0]); got != `find /etc/apt/sources.list.d/ -maxdepth 1 -mindepth 1 -printf %y/%f\n` {
		t.Errorf("argv = %q", got)
	}
	if !f.Calls()[0].Escalate {
		t.Error("ReadDir must escalate so it can list dirs the caller cannot traverse")
	}
}

func TestReadDir_Sudo_EmptyDir(t *testing.T) {
	f := exectest.New(sysexec.Sudo)
	got, err := mustManager(t, f).ReadDir(context.Background(), "/empty")
	if err != nil || len(got) != 0 {
		t.Fatalf("ReadDir(empty) = (%v, %v), want (empty, nil)", got, err)
	}
}

func TestReadDir_Sudo_MissingDirIsErrNotExist(t *testing.T) {
	f := exectest.New(sysexec.Sudo)
	f.Push(sysexec.Result{ExitCode: 1, Stderr: "find: '/x': No such file or directory"}, nil)
	got, err := mustManager(t, f).ReadDir(context.Background(), "/x")

	if !errors.Is(err, os.ErrNotExist) || got != nil {
		t.Fatalf("ReadDir(missing) = (%v, %v), want (nil, ErrNotExist)", got, err)
	}
}

func TestReadDir_Sudo_OtherErrorIsCommandError(t *testing.T) {
	f := exectest.New(sysexec.Sudo)
	f.Push(sysexec.Result{ExitCode: 1, Stderr: "find: '/etc/secret': Permission denied"}, nil)
	if _, err := mustManager(t, f).ReadDir(context.Background(), "/etc/secret"); !errors.As(err, new(*sysexec.CommandError)) {
		t.Fatalf("err = %v, want *exec.CommandError for a non-absent find failure", err)
	}
}

func TestReadDir_Sudo_RunnerErrorPropagates(t *testing.T) {
	f := exectest.New(sysexec.Sudo)
	f.Push(sysexec.Result{}, sysexec.ErrEscalationUnavailable)
	if _, err := mustManager(t, f).ReadDir(context.Background(), "/x"); !errors.Is(err, sysexec.ErrEscalationUnavailable) {
		t.Fatalf("err = %v, want ErrEscalationUnavailable", err)
	}
}

func TestReadDir_Sudo_NotADirIsError(t *testing.T) {
	f := exectest.New(sysexec.Sudo)
	f.Push(sysexec.Result{ExitCode: 1, Stderr: "find: '/etc/hostname/': Not a directory"}, nil)
	got, err := mustManager(t, f).ReadDir(context.Background(), "/etc/hostname")
	if err == nil || got != nil {
		t.Fatalf("ReadDir(regular file) = (%v, %v), want (nil, error)", got, err)
	}
	if errors.Is(err, os.ErrNotExist) {
		t.Fatalf("err = %v, want a non-directory command error, not the absence sentinel", err)
	}
	if !errors.As(err, new(*sysexec.CommandError)) {
		t.Fatalf("err = %v, want *exec.CommandError", err)
	}
}

func TestReadDir_Sudo_FindTargetsDirectoryWithTrailingSlash(t *testing.T) {
	f := exectest.New(sysexec.Sudo)
	f.Push(sysexec.Result{Stdout: "f/a.conf\n"}, nil)
	if _, err := mustManager(t, f).ReadDir(context.Background(), "/etc/app"); err != nil {
		t.Fatal(err)
	}
	calls := f.Calls()
	if len(calls) != 1 || calls[0].Name != "find" {
		t.Fatalf("calls = %+v, want a single find", calls)
	}
	if len(calls[0].Args) == 0 || calls[0].Args[0] != "/etc/app/" {
		t.Fatalf("find target = %q, want \"/etc/app/\" (trailing slash enforces directory-only semantics)", calls[0].Args)
	}
}

func TestReadDir_Sudo_InvalidPathRejectedBeforeExec(t *testing.T) {
	f := exectest.New(sysexec.Sudo)
	if _, err := mustManager(t, f).ReadDir(context.Background(), "-rf"); !errors.Is(err, ErrInvalidPath) {
		t.Fatalf("err = %v, want ErrInvalidPath", err)
	}
	if n := len(f.Calls()); n != 0 {
		t.Errorf("a flag-shaped path reached find (%d calls)", n)
	}
}

func TestReadDir_Sudo_SkipsMalformedAndBlankLines(t *testing.T) {
	f := exectest.New(sysexec.Sudo)

	f.Push(sysexec.Result{Stdout: "f/ok.sources\n\nbogusnoslash\nd/d1\n"}, nil)
	got, err := mustManager(t, f).ReadDir(context.Background(), "/d")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"d1(d)", "ok.sources(f)"}
	if strings.Join(names(got), ",") != strings.Join(want, ",") {
		t.Errorf("entries = %v, want %v (malformed/blank lines skipped)", names(got), want)
	}
}

func TestReadDir_Direct_ListsRealDir(t *testing.T) {
	m := directManager(t)
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.repo"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir, "child"), 0o755); err != nil {
		t.Fatal(err)
	}
	got, err := m.ReadDir(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"a.repo(f)", "child(d)"}
	if strings.Join(names(got), ",") != strings.Join(want, ",") {
		t.Errorf("entries = %v, want %v", names(got), want)
	}
}

func TestReadDir_Direct_MissingDirIsErrNotExist(t *testing.T) {
	m := directManager(t)
	got, err := m.ReadDir(context.Background(), filepath.Join(t.TempDir(), "does-not-exist"))

	if !errors.Is(err, os.ErrNotExist) || got != nil {
		t.Fatalf("ReadDir(missing) = (%v, %v), want (nil, ErrNotExist)", got, err)
	}
}

func TestReadDir_Direct_NotADirIsError(t *testing.T) {
	m := directManager(t)
	file := filepath.Join(t.TempDir(), "regular")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := m.ReadDir(context.Background(), file); err == nil {
		t.Fatal("ReadDir(regular file) err = nil, want a non-directory error")
	}
}

func TestReadDir_Direct_InvalidPathRejected(t *testing.T) {
	m := directManager(t)
	if _, err := m.ReadDir(context.Background(), "-rf"); !errors.Is(err, ErrInvalidPath) {
		t.Fatalf("err = %v, want ErrInvalidPath", err)
	}
}
