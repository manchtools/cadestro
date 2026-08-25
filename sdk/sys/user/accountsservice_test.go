package user

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/manchtools/cadestro/sdk/sys/exec/exectest"
)

func useAccountsSeams(t *testing.T, dir string) (writes map[string]string, removes *[]string) {
	t.Helper()
	f := newFakeFS().install(t)
	rdir := accountsServiceDir
	accountsServiceDir = dir
	t.Cleanup(func() { accountsServiceDir = rdir })
	return f.writes, &f.removes
}

func TestSetHiddenOnLoginScreen_HideWritesSystemAccount(t *testing.T) {
	dir := t.TempDir()
	writes, _ := useAccountsSeams(t, dir)

	if err := mgr(t, exectest.New(0)).SetHiddenOnLoginScreen(context.Background(), "deploy", true); err != nil {
		t.Fatal(err)
	}
	content, ok := writes[filepath.Join(dir, "deploy")]
	if !ok {
		t.Fatalf("no AccountsService file written; writes=%v", writes)
	}
	if content != "[User]\nSystemAccount=true\n" {
		t.Errorf("content = %q", content)
	}
}

func TestSetHiddenOnLoginScreen_UnhideRemovesFile(t *testing.T) {
	dir := t.TempDir()
	_, removes := useAccountsSeams(t, dir)

	if err := mgr(t, exectest.New(0)).SetHiddenOnLoginScreen(context.Background(), "deploy", false); err != nil {
		t.Fatal(err)
	}
	if len(*removes) != 1 || (*removes)[0] != filepath.Join(dir, "deploy") {
		t.Errorf("removes = %v, want the single AccountsService file", *removes)
	}
}

func TestSetHiddenOnLoginScreen_NotInstalledErrors(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "does-not-exist")
	writes, removes := useAccountsSeams(t, missing)
	if err := mgr(t, exectest.New(0)).SetHiddenOnLoginScreen(context.Background(), "deploy", true); err == nil {
		t.Error("hiding with AccountsService absent returned nil error")
	}
	if len(writes) != 0 || len(*removes) != 0 {
		t.Errorf("error path touched the filesystem: writes=%v removes=%v", writes, *removes)
	}
}

func TestSetHiddenOnLoginScreen_RejectsInvalidName(t *testing.T) {
	writes, removes := useAccountsSeams(t, t.TempDir())
	if err := mgr(t, exectest.New(0)).SetHiddenOnLoginScreen(context.Background(), "-evil", true); err == nil {
		t.Error("accepted a flag-shaped name")
	}
	if len(writes) != 0 || len(*removes) != 0 {
		t.Errorf("invalid name reached the filesystem: writes=%v removes=%v", writes, *removes)
	}
}
