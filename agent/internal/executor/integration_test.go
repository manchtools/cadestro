//go:build integration

package executor

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	pb "github.com/manchtools/cadestro/contract/gen/go/cadestro/v1"
	sysexec "github.com/manchtools/cadestro/sdk/sys/exec"

	"github.com/manchtools/cadestro/agent/internal/store"
)

func testRunner() sysexec.Runner {
	backend := sysexec.Sudo
	if os.Geteuid() == 0 {
		backend = sysexec.Direct
	}
	r, err := sysexec.NewRunner(backend)
	if err != nil {
		panic("failed to build integration test runner: " + err.Error())
	}
	return r
}

func newTestExecutor() *Executor {

	e := NewExecutor(testRunner())

	insecure := &http.Client{
		Transport: &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}},
	}
	e.httpClient = insecure
	remoteHTTPClient = insecure
	tmpDir, err := os.MkdirTemp("", "cadestro-executor-test-*")
	if err != nil {
		panic("failed to create temp dir: " + err.Error())
	}

	testExecutorTmpDirsMu.Lock()
	testExecutorTmpDirs = append(testExecutorTmpDirs, tmpDir)
	testExecutorTmpDirsMu.Unlock()
	s, err := store.New(tmpDir)
	if err != nil {
		panic("failed to create test store: " + err.Error())
	}
	e.SetStore(s)

	e.SetLpsPasswordStore(testLpsReports)
	return e
}

var testLpsReports = &recordingLpsStore{}

type recordingLpsStore struct {
	mu        sync.Mutex
	rotations []*pb.LpsPasswordRotation
}

func (r *recordingLpsStore) StorePasswords(_ context.Context, _ string, rotations []*pb.LpsPasswordRotation) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.rotations = append(r.rotations, rotations...)
	return nil
}

func (r *recordingLpsStore) reportedFor(username string) *pb.LpsPasswordRotation {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i := len(r.rotations) - 1; i >= 0; i-- {
		if r.rotations[i].GetUsername() == username {
			return r.rotations[i]
		}
	}
	return nil
}

var testActionCounter int

func makeAction(t *testing.T, actionType pb.ActionType, state pb.DesiredState) *pb.Action {
	t.Helper()
	testActionCounter++
	return &pb.Action{
		Id:           &pb.ActionId{Value: fmt.Sprintf("test%04d", testActionCounter)},
		Type:         actionType,
		DesiredState: state,
	}
}

func testAction(action *pb.Action) *pb.Action { return action }

func assertSuccess(t *testing.T, result *pb.ActionResult) {
	t.Helper()
	if result.Status != pb.ExecutionStatus_EXECUTION_STATUS_SUCCESS {
		t.Errorf("expected SUCCESS, got %s (error: %s, stdout: %s, stderr: %s)",
			result.Status, result.Error,
			truncate(safeStdout(result), 200),
			truncate(safeStderr(result), 200))
	}
}

func assertNotApplicable(t *testing.T, result *pb.ActionResult, wantReason string) {
	t.Helper()
	if result.Status != pb.ExecutionStatus_EXECUTION_STATUS_NOT_APPLICABLE {
		t.Errorf("expected NOT_APPLICABLE, got %s (error: %s, stdout: %s)",
			result.Status, result.Error, truncate(safeStdout(result), 200))
	}
	if !strings.Contains(result.Error, wantReason) {
		t.Errorf("expected reason %q in result error, got: %s", wantReason, result.Error)
	}
	if result.Changed {
		t.Errorf("expected changed=false on a not-applicable result")
	}
}

func assertFailed(t *testing.T, result *pb.ActionResult) {
	t.Helper()
	if result.Status != pb.ExecutionStatus_EXECUTION_STATUS_FAILED {
		t.Errorf("expected FAILED, got %s (stdout: %s)",
			result.Status, truncate(safeStdout(result), 200))
	}
}

func assertChanged(t *testing.T, result *pb.ActionResult, want bool) {
	t.Helper()
	if result.Changed != want {
		t.Errorf("expected changed=%v, got changed=%v (stdout: %s)",
			want, result.Changed, truncate(safeStdout(result), 200))
	}
}

func safeStdout(r *pb.ActionResult) string {
	if r.Output != nil {
		return r.Output.Stdout
	}
	return ""
}

func safeStderr(r *pb.ActionResult) string {
	if r.Output != nil {
		return r.Output.Stderr
	}
	return ""
}

func truncate(s string, n int) string {
	if len(s) > n {
		return s[:n] + "..."
	}
	return s
}

func skipIfNoApt(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("apt-get"); err != nil {
		t.Skip("apt-get not found, skipping")
	}
}

func skipIfNoDnf(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("dnf"); err != nil {
		t.Skip("dnf not found, skipping")
	}
}

func skipIfNoPacman(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("pacman"); err != nil {
		t.Skip("pacman not found, skipping")
	}
}

func skipIfNoZypper(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("zypper"); err != nil {
		t.Skip("zypper not found, skipping")
	}
}

func skipIfNoRpmBuild(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("rpmbuild"); err != nil {
		t.Skip("rpmbuild not found, skipping")
	}
}

func isRpmInstalled(pkg string) bool {
	return checkCmdSuccess("rpm", "-q", pkg)
}

func isPacmanInstalled(pkg string) bool {
	return checkCmdSuccess("pacman", "-Q", pkg)
}

func createTestRpm(t *testing.T) []byte {
	t.Helper()
	dir := t.TempDir()

	for _, sub := range []string{"BUILD", "RPMS", "SOURCES", "SPECS", "SRPMS"} {
		if err := os.MkdirAll(filepath.Join(dir, sub), 0755); err != nil {
			t.Fatal(err)
		}
	}

	spec := `Name: cadestrotestrpm
Version: 1.0.0
Release: 1
Summary: Test RPM for integration tests
License: MIT
BuildArch: noarch

%description
Test package for cadestro integration tests.

%install
mkdir -p %{buildroot}/usr/share/cadestrotestrpm
echo "test" > %{buildroot}/usr/share/cadestrotestrpm/marker

%files
/usr/share/cadestrotestrpm/marker
`
	specFile := filepath.Join(dir, "SPECS", "cadestrotestrpm.spec")
	if err := os.WriteFile(specFile, []byte(spec), 0644); err != nil {
		t.Fatal(err)
	}

	cmd := exec.Command("rpmbuild", "--define", fmt.Sprintf("_topdir %s", dir), "-bb", specFile)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("rpmbuild failed: %v: %s", err, out)
	}

	matches, err := filepath.Glob(filepath.Join(dir, "RPMS", "noarch", "cadestrotestrpm-*.rpm"))
	if err != nil || len(matches) == 0 {
		t.Fatal("no RPM found after rpmbuild")
	}

	data, err := os.ReadFile(matches[0])
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func startFileServer(t *testing.T, files map[string][]byte) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	for path, content := range files {
		body := content
		mux.HandleFunc(path, func(w http.ResponseWriter, r *http.Request) {
			w.Write(body)
		})
	}
	ts := httptest.NewTLSServer(mux)
	t.Cleanup(ts.Close)
	return ts
}

func createTestDeb(t *testing.T) []byte {
	t.Helper()
	dir := t.TempDir()
	pkgDir := filepath.Join(dir, "cadestro-testpkg")
	debianDir := filepath.Join(pkgDir, "DEBIAN")
	if err := os.MkdirAll(debianDir, 0755); err != nil {
		t.Fatal(err)
	}
	control := `Package: cadestro-testpkg
Version: 1.0.0
Architecture: all
Maintainer: test <test@test.com>
Description: Test package for integration tests
`
	if err := os.WriteFile(filepath.Join(debianDir, "control"), []byte(control), 0644); err != nil {
		t.Fatal(err)
	}
	debFile := filepath.Join(dir, "cadestro-testpkg_1.0.0_all.deb")
	cmd := exec.Command("dpkg-deb", "--build", pkgDir, debFile)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("dpkg-deb failed: %v: %s", err, out)
	}
	data, err := os.ReadFile(debFile)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func ensureTestUser(t *testing.T, username string) {
	t.Helper()
	v, err := userExists(context.Background(), username)
	if err != nil {

		t.Fatalf("userExists(%s): %v", username, err)
	}
	if v {
		return
	}
	cmd := sudoRun("useradd", "--no-create-home", "--shell", "/bin/bash", username)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("useradd %s: %v: %s", username, err, out)
	}
}

func cleanupTestUser(t *testing.T, username string) {
	t.Helper()
	sudoRun("userdel", "-r", username).Run()
}

func cleanupTestGroup(t *testing.T, groupName string) {
	t.Helper()
	sudoRun("groupdel", groupName).Run()
}

func sudoRun(name string, args ...string) *exec.Cmd {
	if os.Geteuid() == 0 {
		return exec.Command(name, args...)
	}
	sudoArgs := append([]string{"-n", name}, args...)
	return exec.Command("sudo", sudoArgs...)
}

func sudoRemove(path string) {
	sudoRun("rm", "-f", path).Run()
}

func sudoRemoveAll(path string) {
	sudoRun("rm", "-rf", path).Run()
}

func sudoWriteFile(path string, content []byte) error {
	cmd := sudoRun("tee", path)
	cmd.Stdin = bytes.NewReader(content)
	cmd.Stdout = io.Discard
	return cmd.Run()
}

func sudoFileExists(path string) bool {
	return sudoRun("sh", "-c", fmt.Sprintf("test -e %s", path)).Run() == nil
}

func removePacmanSection(content, name string) string {
	sectionHeader := "[" + name + "]"
	lines := strings.Split(content, "\n")
	var result []string
	inSection := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == sectionHeader {
			inSection = true
			continue
		}
		if inSection && strings.HasPrefix(trimmed, "[") {
			inSection = false
		}
		if !inSection {
			result = append(result, line)
		}
	}
	return strings.Join(result, "\n")
}

func TestIntegration_Package(t *testing.T) {
	skipIfNoApt(t)
	e := newTestExecutor()
	ctx := context.Background()

	sudoRun("apt-get", "remove", "-y", "sl").Run()

	t.Run("Install", func(t *testing.T) {
		action := makeAction(t, pb.ActionType_ACTION_TYPE_PACKAGE, pb.DesiredState_DESIRED_STATE_PRESENT)
		action.Params = &pb.Action_Package{Package: &pb.PackageParams{Name: "sl"}}
		result := e.ExecuteAction(ctx, testAction(action))
		assertSuccess(t, result)
		assertChanged(t, result, true)
		if !checkCmdSuccess("dpkg", "-s", "sl") {
			t.Error("sl not installed after action")
		}
	})

	t.Run("InstallIdempotent", func(t *testing.T) {
		action := makeAction(t, pb.ActionType_ACTION_TYPE_PACKAGE, pb.DesiredState_DESIRED_STATE_PRESENT)
		action.Params = &pb.Action_Package{Package: &pb.PackageParams{Name: "sl"}}
		result := e.ExecuteAction(ctx, testAction(action))
		assertSuccess(t, result)
		assertChanged(t, result, false)
	})

	t.Run("Remove", func(t *testing.T) {
		action := makeAction(t, pb.ActionType_ACTION_TYPE_PACKAGE, pb.DesiredState_DESIRED_STATE_ABSENT)
		action.Params = &pb.Action_Package{Package: &pb.PackageParams{Name: "sl"}}
		result := e.ExecuteAction(ctx, testAction(action))
		assertSuccess(t, result)
		assertChanged(t, result, true)
		if checkCmdSuccess("dpkg", "-s", "sl") {
			t.Error("sl still installed after removal")
		}
	})

	t.Run("RemoveNotInstalled", func(t *testing.T) {
		action := makeAction(t, pb.ActionType_ACTION_TYPE_PACKAGE, pb.DesiredState_DESIRED_STATE_ABSENT)
		action.Params = &pb.Action_Package{Package: &pb.PackageParams{Name: "sl"}}
		result := e.ExecuteAction(ctx, testAction(action))
		assertSuccess(t, result)
		assertChanged(t, result, false)
	})

	t.Run("InstallNonExistent", func(t *testing.T) {
		action := makeAction(t, pb.ActionType_ACTION_TYPE_PACKAGE, pb.DesiredState_DESIRED_STATE_PRESENT)
		action.Params = &pb.Action_Package{Package: &pb.PackageParams{Name: "this-package-does-not-exist-xyz"}}
		result := e.ExecuteAction(ctx, testAction(action))
		assertFailed(t, result)
	})

	t.Run("NilPkgManager", func(t *testing.T) {
		nopm := &Executor{
			httpClient: e.httpClient,
			pkgManager: nil,
			logger:     e.logger,
			now:        e.now,
		}
		action := makeAction(t, pb.ActionType_ACTION_TYPE_PACKAGE, pb.DesiredState_DESIRED_STATE_PRESENT)
		action.Params = &pb.Action_Package{Package: &pb.PackageParams{Name: "sl"}}
		result := nopm.ExecuteAction(ctx, testAction(action))
		assertFailed(t, result)
	})
}

func TestIntegration_Package_GracefulSkip(t *testing.T) {
	skipIfNoApt(t)
	e := newTestExecutor()
	ctx := context.Background()

	t.Run("DnfNameOnly", func(t *testing.T) {
		action := makeAction(t, pb.ActionType_ACTION_TYPE_PACKAGE, pb.DesiredState_DESIRED_STATE_PRESENT)
		action.Params = &pb.Action_Package{Package: &pb.PackageParams{DnfName: "some-dnf-pkg"}}
		result := e.ExecuteAction(ctx, testAction(action))
		assertNotApplicable(t, result, "no package name configured")
	})

	t.Run("PacmanNameOnly", func(t *testing.T) {
		action := makeAction(t, pb.ActionType_ACTION_TYPE_PACKAGE, pb.DesiredState_DESIRED_STATE_PRESENT)
		action.Params = &pb.Action_Package{Package: &pb.PackageParams{PacmanName: "some-pacman-pkg"}}
		result := e.ExecuteAction(ctx, testAction(action))
		assertNotApplicable(t, result, "no package name configured")
	})

	t.Run("ZypperNameOnly", func(t *testing.T) {
		action := makeAction(t, pb.ActionType_ACTION_TYPE_PACKAGE, pb.DesiredState_DESIRED_STATE_PRESENT)
		action.Params = &pb.Action_Package{Package: &pb.PackageParams{ZypperName: "some-zypper-pkg"}}
		result := e.ExecuteAction(ctx, testAction(action))
		assertNotApplicable(t, result, "no package name configured")
	})
}

func TestIntegration_Update(t *testing.T) {
	skipIfNoApt(t)
	e := newTestExecutor()
	ctx := context.Background()

	t.Run("AptUpgrade", func(t *testing.T) {
		action := makeAction(t, pb.ActionType_ACTION_TYPE_UPDATE, pb.DesiredState_DESIRED_STATE_PRESENT)
		action.Params = &pb.Action_Update{Update: &pb.UpdateParams{}}
		result := e.ExecuteAction(ctx, testAction(action))
		assertSuccess(t, result)
	})
}

func TestIntegration_Shell(t *testing.T) {
	e := newTestExecutor()
	ctx := context.Background()

	t.Run("BasicScript", func(t *testing.T) {
		action := makeAction(t, pb.ActionType_ACTION_TYPE_SHELL, pb.DesiredState_DESIRED_STATE_PRESENT)
		action.Params = &pb.Action_Shell{Shell: &pb.ShellParams{Script: "echo hello", RunAsRoot: true}}
		result := e.ExecuteAction(ctx, testAction(action))
		assertSuccess(t, result)
		if !strings.Contains(safeStdout(result), "hello") {
			t.Errorf("expected 'hello' in stdout, got: %s", safeStdout(result))
		}
	})

	t.Run("NonZeroExit", func(t *testing.T) {
		action := makeAction(t, pb.ActionType_ACTION_TYPE_SHELL, pb.DesiredState_DESIRED_STATE_PRESENT)
		action.Params = &pb.Action_Shell{Shell: &pb.ShellParams{Script: "exit 42", RunAsRoot: true}}
		result := e.ExecuteAction(ctx, testAction(action))

		if result.Status != pb.ExecutionStatus_EXECUTION_STATUS_FAILED {
			t.Errorf("expected FAILED, got %s", result.Status)
		}
		if result.Output == nil || result.Output.ExitCode != 42 {
			t.Errorf("expected exit code 42, got %d", result.Output.ExitCode)
		}
	})

	t.Run("RunAsRoot", func(t *testing.T) {
		action := makeAction(t, pb.ActionType_ACTION_TYPE_SHELL, pb.DesiredState_DESIRED_STATE_PRESENT)
		action.Params = &pb.Action_Shell{Shell: &pb.ShellParams{
			Script:    "whoami",
			RunAsRoot: true,
		}}
		result := e.ExecuteAction(ctx, testAction(action))
		assertSuccess(t, result)
		if !strings.Contains(safeStdout(result), "root") {
			t.Errorf("expected 'root' in stdout, got: %s", safeStdout(result))
		}
	})

	t.Run("Environment", func(t *testing.T) {
		action := makeAction(t, pb.ActionType_ACTION_TYPE_SHELL, pb.DesiredState_DESIRED_STATE_PRESENT)
		action.Params = &pb.Action_Shell{Shell: &pb.ShellParams{
			Script:      "echo $MY_TEST_VAR",
			Environment: map[string]string{"MY_TEST_VAR": "test123"},
			RunAsRoot:   true,
		}}
		result := e.ExecuteAction(ctx, testAction(action))
		assertSuccess(t, result)
		if !strings.Contains(safeStdout(result), "test123") {
			t.Errorf("expected 'test123' in stdout, got: %s", safeStdout(result))
		}
	})

	t.Run("WorkingDirectory", func(t *testing.T) {
		action := makeAction(t, pb.ActionType_ACTION_TYPE_SHELL, pb.DesiredState_DESIRED_STATE_PRESENT)
		action.Params = &pb.Action_Shell{Shell: &pb.ShellParams{
			Script:           "pwd",
			WorkingDirectory: "/tmp",
			RunAsRoot:        true,
		}}
		result := e.ExecuteAction(ctx, testAction(action))
		assertSuccess(t, result)
		if !strings.Contains(safeStdout(result), "/tmp") {
			t.Errorf("expected '/tmp' in stdout, got: %s", safeStdout(result))
		}
	})
}

func TestIntegration_File(t *testing.T) {
	e := newTestExecutor()
	ctx := context.Background()
	testFile := "/tmp/cadestro-integration-test-file"

	t.Cleanup(func() {
		sudoRemove(testFile)
	})

	t.Run("Create", func(t *testing.T) {
		action := makeAction(t, pb.ActionType_ACTION_TYPE_FILE, pb.DesiredState_DESIRED_STATE_PRESENT)
		action.Params = &pb.Action_File{File: &pb.FileParams{
			Path:    testFile,
			Content: "hello world\n",
			Mode:    "0644",
			Owner:   "root",
			Group:   "root",
		}}
		result := e.ExecuteAction(ctx, testAction(action))
		assertSuccess(t, result)
		assertChanged(t, result, true)

		data, err := os.ReadFile(testFile)
		if err != nil {
			t.Fatal(err)
		}
		if string(data) != "hello world\n" {
			t.Errorf("file content mismatch: %q", string(data))
		}
	})

	t.Run("CreateIdempotent", func(t *testing.T) {
		action := makeAction(t, pb.ActionType_ACTION_TYPE_FILE, pb.DesiredState_DESIRED_STATE_PRESENT)
		action.Params = &pb.Action_File{File: &pb.FileParams{
			Path:    testFile,
			Content: "hello world\n",
			Mode:    "0644",
			Owner:   "root",
			Group:   "root",
		}}
		result := e.ExecuteAction(ctx, testAction(action))
		assertSuccess(t, result)
		assertChanged(t, result, false)
	})

	t.Run("Ownership", func(t *testing.T) {
		owner, group := getFileOwnership(testFile)
		if owner != "root" || group != "root" {
			t.Errorf("expected root:root ownership, got %s:%s", owner, group)
		}
	})

	t.Run("Remove", func(t *testing.T) {
		action := makeAction(t, pb.ActionType_ACTION_TYPE_FILE, pb.DesiredState_DESIRED_STATE_ABSENT)
		action.Params = &pb.Action_File{File: &pb.FileParams{Path: testFile}}
		result := e.ExecuteAction(ctx, testAction(action))
		assertSuccess(t, result)
		assertChanged(t, result, true)
		if _, err := os.Stat(testFile); !os.IsNotExist(err) {
			t.Error("file still exists after removal")
		}
	})

	t.Run("RemoveAbsent", func(t *testing.T) {
		action := makeAction(t, pb.ActionType_ACTION_TYPE_FILE, pb.DesiredState_DESIRED_STATE_ABSENT)
		action.Params = &pb.Action_File{File: &pb.FileParams{Path: testFile}}
		result := e.ExecuteAction(ctx, testAction(action))
		assertSuccess(t, result)
		assertChanged(t, result, false)
	})

	t.Run("ManagedBlock", func(t *testing.T) {
		mbFile := "/tmp/cadestro-integration-test-mb"
		t.Cleanup(func() { sudoRemove(mbFile) })

		os.WriteFile(mbFile, []byte("existing content\n"), 0644)

		action := makeAction(t, pb.ActionType_ACTION_TYPE_FILE, pb.DesiredState_DESIRED_STATE_PRESENT)
		action.Params = &pb.Action_File{File: &pb.FileParams{
			Path:         mbFile,
			Content:      "# managed block\n",
			ManagedBlock: true,
		}}
		result := e.ExecuteAction(ctx, testAction(action))
		assertSuccess(t, result)
		assertChanged(t, result, true)

		data, _ := os.ReadFile(mbFile)
		if !strings.Contains(string(data), "existing content") {
			t.Error("existing content was lost")
		}
		if !strings.Contains(string(data), "# managed block") {
			t.Error("managed block content not found")
		}
	})
}

func TestIntegration_Directory(t *testing.T) {
	e := newTestExecutor()
	ctx := context.Background()
	testDir := "/tmp/cadestro-integration-test-dir"

	t.Cleanup(func() {
		sudoRemoveAll(testDir)
		sudoRemoveAll("/tmp/cadestro-integration-deep")
	})

	t.Run("Create", func(t *testing.T) {
		action := makeAction(t, pb.ActionType_ACTION_TYPE_DIRECTORY, pb.DesiredState_DESIRED_STATE_PRESENT)
		action.Params = &pb.Action_Directory{Directory: &pb.DirectoryParams{
			Path: testDir,
			Mode: "0755",
		}}
		result := e.ExecuteAction(ctx, testAction(action))
		assertSuccess(t, result)
		assertChanged(t, result, true)
		info, err := os.Stat(testDir)
		if err != nil {
			t.Fatal(err)
		}
		if !info.IsDir() {
			t.Error("not a directory")
		}
	})

	t.Run("CreateIdempotent", func(t *testing.T) {
		action := makeAction(t, pb.ActionType_ACTION_TYPE_DIRECTORY, pb.DesiredState_DESIRED_STATE_PRESENT)
		action.Params = &pb.Action_Directory{Directory: &pb.DirectoryParams{
			Path: testDir,
			Mode: "0755",
		}}
		result := e.ExecuteAction(ctx, testAction(action))
		assertSuccess(t, result)
		assertChanged(t, result, false)
	})

	t.Run("CreateRecursive", func(t *testing.T) {
		deepDir := "/tmp/cadestro-integration-deep/a/b/c"
		action := makeAction(t, pb.ActionType_ACTION_TYPE_DIRECTORY, pb.DesiredState_DESIRED_STATE_PRESENT)
		action.Params = &pb.Action_Directory{Directory: &pb.DirectoryParams{
			Path:      deepDir,
			Recursive: true,
		}}
		result := e.ExecuteAction(ctx, testAction(action))
		assertSuccess(t, result)
		assertChanged(t, result, true)
		if _, err := os.Stat(deepDir); err != nil {
			t.Errorf("deep directory not created: %v", err)
		}
	})

	t.Run("Remove", func(t *testing.T) {
		action := makeAction(t, pb.ActionType_ACTION_TYPE_DIRECTORY, pb.DesiredState_DESIRED_STATE_ABSENT)
		action.Params = &pb.Action_Directory{Directory: &pb.DirectoryParams{Path: testDir}}
		result := e.ExecuteAction(ctx, testAction(action))
		assertSuccess(t, result)
		assertChanged(t, result, true)
		if _, err := os.Stat(testDir); !os.IsNotExist(err) {
			t.Error("directory still exists")
		}
	})

	t.Run("ProtectedPath", func(t *testing.T) {
		action := makeAction(t, pb.ActionType_ACTION_TYPE_DIRECTORY, pb.DesiredState_DESIRED_STATE_ABSENT)
		action.Params = &pb.Action_Directory{Directory: &pb.DirectoryParams{Path: "/usr"}}
		result := e.ExecuteAction(ctx, testAction(action))
		assertFailed(t, result)
	})
}

func TestIntegration_User(t *testing.T) {
	e := newTestExecutor()
	ctx := context.Background()
	username := "cadestrotestuser"

	t.Cleanup(func() { cleanupTestUser(t, username) })

	t.Run("Create", func(t *testing.T) {
		action := makeAction(t, pb.ActionType_ACTION_TYPE_USER, pb.DesiredState_DESIRED_STATE_PRESENT)
		action.Params = &pb.Action_User{User: &pb.UserParams{
			Username: username,
			Comment:  "Integration Test User",
		}}
		result := e.ExecuteAction(ctx, testAction(action))
		assertSuccess(t, result)
		assertChanged(t, result, true)
		if v, _ := userExists(context.Background(), username); !v {
			t.Error("user not created")
		}

		if testLpsReports.reportedFor(username) == nil {
			t.Error("expected the temporary password to be reported to control")
		}
	})

	t.Run("CreateIdempotent", func(t *testing.T) {
		action := makeAction(t, pb.ActionType_ACTION_TYPE_USER, pb.DesiredState_DESIRED_STATE_PRESENT)
		action.Params = &pb.Action_User{User: &pb.UserParams{
			Username: username,
			Comment:  "Integration Test User",
		}}
		result := e.ExecuteAction(ctx, testAction(action))
		assertSuccess(t, result)
		assertChanged(t, result, false)
	})

	t.Run("UpdateShell", func(t *testing.T) {
		action := makeAction(t, pb.ActionType_ACTION_TYPE_USER, pb.DesiredState_DESIRED_STATE_PRESENT)
		action.Params = &pb.Action_User{User: &pb.UserParams{
			Username: username,
			Shell:    "/bin/sh",
		}}
		result := e.ExecuteAction(ctx, testAction(action))
		assertSuccess(t, result)
		assertChanged(t, result, true)
		info, err := userMgr.Get(ctx, username)
		if err != nil {
			t.Fatal(err)
		}
		if info.Shell != "/bin/sh" {
			t.Errorf("expected shell /bin/sh, got %s", info.Shell)
		}
	})

	t.Run("Remove", func(t *testing.T) {
		action := makeAction(t, pb.ActionType_ACTION_TYPE_USER, pb.DesiredState_DESIRED_STATE_ABSENT)
		action.Params = &pb.Action_User{User: &pb.UserParams{Username: username}}
		result := e.ExecuteAction(ctx, testAction(action))
		assertSuccess(t, result)
		assertChanged(t, result, true)
		if v, _ := userExists(context.Background(), username); v {
			t.Error("user still exists")
		}
	})

	t.Run("RemoveAbsent", func(t *testing.T) {
		action := makeAction(t, pb.ActionType_ACTION_TYPE_USER, pb.DesiredState_DESIRED_STATE_ABSENT)
		action.Params = &pb.Action_User{User: &pb.UserParams{Username: username}}
		result := e.ExecuteAction(ctx, testAction(action))
		assertSuccess(t, result)
		assertChanged(t, result, false)
	})
}

func TestIntegration_User_CreateHomeRespected(t *testing.T) {
	e := newTestExecutor()
	ctx := context.Background()

	t.Run("ExplicitFalse_NoHomeCreated", func(t *testing.T) {
		username := "cadestrotestnohome"
		homeDir := "/home/" + username
		t.Cleanup(func() {
			cleanupTestUser(t, username)

			sudoRun("rm", "-rf", homeDir).Run()
		})

		action := makeAction(t, pb.ActionType_ACTION_TYPE_USER, pb.DesiredState_DESIRED_STATE_PRESENT)
		action.Params = &pb.Action_User{User: &pb.UserParams{
			Username:   username,
			Shell:      "/usr/sbin/nologin",
			CreateHome: false,
			Comment:    "regression test for create_home false",
		}}
		result := e.ExecuteAction(ctx, testAction(action))
		assertSuccess(t, result)
		if v, _ := userExists(context.Background(), username); !v {
			t.Fatal("user not created")
		}

		if _, err := os.Stat(homeDir); !os.IsNotExist(err) {
			t.Errorf("home directory %s exists but create_home was false: stat err = %v", homeDir, err)
		}
	})

	t.Run("ExplicitTrue_HomeCreated", func(t *testing.T) {
		username := "cadestrotestwithhome"
		homeDir := "/home/" + username
		t.Cleanup(func() {
			cleanupTestUser(t, username)
			sudoRun("rm", "-rf", homeDir).Run()
		})

		action := makeAction(t, pb.ActionType_ACTION_TYPE_USER, pb.DesiredState_DESIRED_STATE_PRESENT)
		action.Params = &pb.Action_User{User: &pb.UserParams{
			Username:   username,
			CreateHome: true,
			Comment:    "regression test for create_home true",
		}}
		result := e.ExecuteAction(ctx, testAction(action))
		assertSuccess(t, result)
		if v, _ := userExists(context.Background(), username); !v {
			t.Fatal("user not created")
		}

		if info, err := os.Stat(homeDir); err != nil {
			t.Errorf("home directory %s missing but create_home was true: stat err = %v", homeDir, err)
		} else if !info.IsDir() {
			t.Errorf("%s exists but is not a directory", homeDir)
		}
	})
}

func TestIntegration_User_NoPassword(t *testing.T) {
	e := newTestExecutor()
	ctx := context.Background()

	t.Run("NoPasswordTrue_NoChpasswdNoLpsMetadata", func(t *testing.T) {
		username := "cadestrotestnopass"
		t.Cleanup(func() { cleanupTestUser(t, username) })

		action := makeAction(t, pb.ActionType_ACTION_TYPE_USER, pb.DesiredState_DESIRED_STATE_PRESENT)
		action.Params = &pb.Action_User{User: &pb.UserParams{
			Username:   username,
			Shell:      "/usr/sbin/nologin",
			CreateHome: false,
			NoPassword: true,
			Comment:    "no_password flag regression test",
		}}
		result := e.ExecuteAction(ctx, testAction(action))
		assertSuccess(t, result)
		if v, _ := userExists(context.Background(), username); !v {
			t.Fatal("user not created")
		}

		if rot := testLpsReports.reportedFor(username); rot != nil {
			t.Errorf("expected no password reported when NoPassword=true, got one for %q", rot.GetUsername())
		}

		info, err := userMgr.Get(context.Background(), username)
		if err != nil {
			t.Fatalf("userMgr.Get(%s): %v", username, err)
		}
		if info.Locked {
			t.Error("enabled no_password account must be unlocked (Info.Locked=false) so the terminal handler accepts it; got Locked=true")
		}
	})

	t.Run("NoPasswordFalse_StillGeneratesPassword", func(t *testing.T) {

		username := "cadestrotestwithpass"
		t.Cleanup(func() { cleanupTestUser(t, username) })

		action := makeAction(t, pb.ActionType_ACTION_TYPE_USER, pb.DesiredState_DESIRED_STATE_PRESENT)
		action.Params = &pb.Action_User{User: &pb.UserParams{
			Username: username,
			Comment:  "no_password=false regression guard",
		}}
		result := e.ExecuteAction(ctx, testAction(action))
		assertSuccess(t, result)
		if testLpsReports.reportedFor(username) == nil {
			t.Error("expected a password reported when NoPassword=false (default), got none")
		}
	})
}

func TestIntegration_User_ReapplyNoPasswordStaysStar(t *testing.T) {
	e := newTestExecutor()
	ctx := context.Background()
	username := "cadestrotestreapplylock"
	t.Cleanup(func() { cleanupTestUser(t, username) })

	params := &pb.UserParams{
		Username:   username,
		Shell:      "/usr/sbin/nologin",
		NoPassword: true,
		Comment:    "no_password reapply regression (#94)",
	}
	assertUnlocked := func(stage string) {
		t.Helper()
		info, err := userMgr.Get(context.Background(), username)
		if err != nil {
			t.Fatalf("%s: userMgr.Get(%s): %v", stage, username, err)
		}
		if info.Locked {
			t.Fatalf("%s: enabled no_password account must stay UNLOCKED across re-apply; got Locked=true", stage)
		}
	}
	apply := func(stage string) *pb.ActionResult {
		t.Helper()
		a := makeAction(t, pb.ActionType_ACTION_TYPE_USER, pb.DesiredState_DESIRED_STATE_PRESENT)
		a.Params = &pb.Action_User{User: params}
		res := e.ExecuteAction(ctx, testAction(a))
		assertSuccess(t, res)
		if rot := testLpsReports.reportedFor(username); rot != nil {
			t.Errorf("%s: a no_password account must not report a password (none is set), got one for %q", stage, rot.GetUsername())
		}
		return res
	}

	apply("create")
	if v, _ := userExists(context.Background(), username); !v {
		t.Fatal("user not created")
	}
	assertUnlocked("after create")

	apply("re-apply")
	assertUnlocked("after re-apply")
}

func TestIntegration_User_DisabledNoPasswordIsLocked(t *testing.T) {
	e := newTestExecutor()
	ctx := context.Background()
	username := "cadestrotestdisablednopass"
	t.Cleanup(func() { cleanupTestUser(t, username) })

	action := makeAction(t, pb.ActionType_ACTION_TYPE_USER, pb.DesiredState_DESIRED_STATE_PRESENT)
	action.Params = &pb.Action_User{User: &pb.UserParams{
		Username:   username,
		Shell:      "/usr/sbin/nologin",
		NoPassword: true,
		Disabled:   true,
		Comment:    "disabled no_password (lock=disabled gate)",
	}}
	assertSuccess(t, e.ExecuteAction(ctx, testAction(action)))

	info, err := userMgr.Get(context.Background(), username)
	if err != nil {
		t.Fatalf("userMgr.Get(%s): %v", username, err)
	}
	if !info.Locked {
		t.Error("disabled no_password account must be LOCKED (Info.Locked=true) so the terminal handler refuses it; got Locked=false")
	}
}

func TestIntegration_Group(t *testing.T) {
	e := newTestExecutor()
	ctx := context.Background()
	groupName := "cadestrotestgroup"

	t.Cleanup(func() {
		cleanupTestGroup(t, groupName)
		cleanupTestUser(t, "cadestrogrpuser1")
		cleanupTestUser(t, "cadestrogrpuser2")
	})

	t.Run("Create", func(t *testing.T) {
		action := makeAction(t, pb.ActionType_ACTION_TYPE_GROUP, pb.DesiredState_DESIRED_STATE_PRESENT)
		action.Params = &pb.Action_Group{Group: &pb.GroupParams{Name: groupName}}
		result := e.ExecuteAction(ctx, testAction(action))
		assertSuccess(t, result)
		assertChanged(t, result, true)
		if v, _ := groupExists(context.Background(), groupName); !v {
			t.Error("group not created")
		}
	})

	t.Run("CreateIdempotent", func(t *testing.T) {
		action := makeAction(t, pb.ActionType_ACTION_TYPE_GROUP, pb.DesiredState_DESIRED_STATE_PRESENT)
		action.Params = &pb.Action_Group{Group: &pb.GroupParams{Name: groupName}}
		result := e.ExecuteAction(ctx, testAction(action))
		assertSuccess(t, result)
		assertChanged(t, result, false)
	})

	t.Run("AddMembers", func(t *testing.T) {
		ensureTestUser(t, "cadestrogrpuser1")
		ensureTestUser(t, "cadestrogrpuser2")
		action := makeAction(t, pb.ActionType_ACTION_TYPE_GROUP, pb.DesiredState_DESIRED_STATE_PRESENT)
		action.Params = &pb.Action_Group{Group: &pb.GroupParams{
			Name:    groupName,
			Members: []string{"cadestrogrpuser1", "cadestrogrpuser2"},
		}}
		result := e.ExecuteAction(ctx, testAction(action))
		assertSuccess(t, result)
		assertChanged(t, result, true)
		if !userInGroup(ctx, "cadestrogrpuser1", groupName) {
			t.Error("cadestrogrpuser1 not in group")
		}
		if !userInGroup(ctx, "cadestrogrpuser2", groupName) {
			t.Error("cadestrogrpuser2 not in group")
		}
	})

	t.Run("EmptyMembersRemovesAll", func(t *testing.T) {

		action := makeAction(t, pb.ActionType_ACTION_TYPE_GROUP, pb.DesiredState_DESIRED_STATE_PRESENT)
		action.Params = &pb.Action_Group{Group: &pb.GroupParams{
			Name:    groupName,
			Members: []string{},
		}}
		result := e.ExecuteAction(ctx, testAction(action))
		assertSuccess(t, result)
		assertChanged(t, result, true)
		if v, _ := groupExists(context.Background(), groupName); !v {
			t.Error("group should still exist")
		}
		if userInGroup(ctx, "cadestrogrpuser1", groupName) {
			t.Error("cadestrogrpuser1 should have been removed from group")
		}
		if userInGroup(ctx, "cadestrogrpuser2", groupName) {
			t.Error("cadestrogrpuser2 should have been removed from group")
		}
	})

	t.Run("Remove", func(t *testing.T) {
		action := makeAction(t, pb.ActionType_ACTION_TYPE_GROUP, pb.DesiredState_DESIRED_STATE_ABSENT)
		action.Params = &pb.Action_Group{Group: &pb.GroupParams{Name: groupName}}
		result := e.ExecuteAction(ctx, testAction(action))
		assertSuccess(t, result)
		assertChanged(t, result, true)
		if v, _ := groupExists(context.Background(), groupName); v {
			t.Error("group still exists")
		}
	})
}

func TestIntegration_Sudo(t *testing.T) {
	e := newTestExecutor()
	ctx := context.Background()
	actionID := "sudotest01"

	ensureTestUser(t, "cadestrosudouser")
	t.Cleanup(func() {
		sudoRemove(sudoersFilePath(actionID))
		sudoRun("groupdel", sanitizeSudoGroupName(actionID)).Run()
		cleanupTestUser(t, "cadestrosudouser")
	})

	t.Run("SetupFullAccess", func(t *testing.T) {
		action := &pb.Action{
			Id:           &pb.ActionId{Value: actionID},
			Type:         pb.ActionType_ACTION_TYPE_ADMIN_POLICY,
			DesiredState: pb.DesiredState_DESIRED_STATE_PRESENT,
			Params: &pb.Action_AdminPolicy{AdminPolicy: &pb.AdminPolicyParams{
				AccessLevel: pb.AdminAccessLevel_ADMIN_ACCESS_LEVEL_FULL,
				Users:       []string{"cadestrosudouser"},
			}},
		}
		result := e.ExecuteAction(ctx, testAction(action))
		assertSuccess(t, result)
		assertChanged(t, result, true)

		filePath := sudoersFilePath(actionID)
		if !sudoFileExists(filePath) {
			t.Error("sudoers file not created")
		}
		cmd := sudoRun("visudo", "-c", "-f", filePath)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Errorf("visudo validation failed: %v: %s", err, out)
		}
	})

	t.Run("SetupIdempotent", func(t *testing.T) {
		action := &pb.Action{
			Id:           &pb.ActionId{Value: actionID},
			Type:         pb.ActionType_ACTION_TYPE_ADMIN_POLICY,
			DesiredState: pb.DesiredState_DESIRED_STATE_PRESENT,
			Params: &pb.Action_AdminPolicy{AdminPolicy: &pb.AdminPolicyParams{
				AccessLevel: pb.AdminAccessLevel_ADMIN_ACCESS_LEVEL_FULL,
				Users:       []string{"cadestrosudouser"},
			}},
		}
		result := e.ExecuteAction(ctx, testAction(action))
		assertSuccess(t, result)
		assertChanged(t, result, false)
	})

	t.Run("Remove", func(t *testing.T) {
		action := &pb.Action{
			Id:           &pb.ActionId{Value: actionID},
			Type:         pb.ActionType_ACTION_TYPE_ADMIN_POLICY,
			DesiredState: pb.DesiredState_DESIRED_STATE_ABSENT,
			Params: &pb.Action_AdminPolicy{AdminPolicy: &pb.AdminPolicyParams{
				AccessLevel: pb.AdminAccessLevel_ADMIN_ACCESS_LEVEL_FULL,
				Users:       []string{"cadestrosudouser"},
			}},
		}
		result := e.ExecuteAction(ctx, testAction(action))
		assertSuccess(t, result)
		assertChanged(t, result, true)
		filePath := sudoersFilePath(actionID)
		if sudoFileExists(filePath) {
			t.Error("sudoers file still exists")
		}
		if v, _ := groupExists(context.Background(), sanitizeSudoGroupName(actionID)); v {
			t.Error("sudo group still exists")
		}
	})
}

func TestIntegration_SSH(t *testing.T) {
	e := newTestExecutor()
	ctx := context.Background()
	actionID := "sshtest01"

	t.Cleanup(func() {
		sudoRemove(sshConfigPath(actionID))
		sudoRun("groupdel", sshGroupName(actionID)).Run()
		cleanupTestUser(t, "cadestrosshuser")
	})

	ensureTestUser(t, "cadestrosshuser")

	t.Run("SetupAccess", func(t *testing.T) {
		action := &pb.Action{
			Id:           &pb.ActionId{Value: actionID},
			Type:         pb.ActionType_ACTION_TYPE_SSH,
			DesiredState: pb.DesiredState_DESIRED_STATE_PRESENT,
			Params: &pb.Action_Ssh{Ssh: &pb.SshParams{
				Users:         []string{"cadestrosshuser"},
				AllowPubkey:   true,
				AllowPassword: false,
			}},
		}
		result := e.ExecuteAction(ctx, testAction(action))
		assertSuccess(t, result)
		assertChanged(t, result, true)
		configPath := sshConfigPath(actionID)
		if !sudoFileExists(configPath) {
			t.Error("SSH config not created")
		}
		if v, _ := groupExists(context.Background(), sshGroupName(actionID)); !v {
			t.Error("SSH group not created")
		}
	})

	t.Run("SetupIdempotent", func(t *testing.T) {
		action := &pb.Action{
			Id:           &pb.ActionId{Value: actionID},
			Type:         pb.ActionType_ACTION_TYPE_SSH,
			DesiredState: pb.DesiredState_DESIRED_STATE_PRESENT,
			Params: &pb.Action_Ssh{Ssh: &pb.SshParams{
				Users:         []string{"cadestrosshuser"},
				AllowPubkey:   true,
				AllowPassword: false,
			}},
		}
		result := e.ExecuteAction(ctx, testAction(action))
		assertSuccess(t, result)
		assertChanged(t, result, false)
	})

	t.Run("RemoveAccess", func(t *testing.T) {
		action := &pb.Action{
			Id:           &pb.ActionId{Value: actionID},
			Type:         pb.ActionType_ACTION_TYPE_SSH,
			DesiredState: pb.DesiredState_DESIRED_STATE_ABSENT,
			Params: &pb.Action_Ssh{Ssh: &pb.SshParams{
				Users: []string{"cadestrosshuser"},
			}},
		}
		result := e.ExecuteAction(ctx, testAction(action))
		assertSuccess(t, result)
		assertChanged(t, result, true)
		configPath := sshConfigPath(actionID)
		if sudoFileExists(configPath) {
			t.Error("SSH config still exists")
		}
	})
}

func TestIntegration_SSHD(t *testing.T) {
	e := newTestExecutor()
	ctx := context.Background()
	actionID := "sshdtest01"
	priority := uint32(50)
	configPath := fmt.Sprintf("/etc/ssh/sshd_config.d/%04d-cadestro-%s.conf", priority, actionID)

	t.Cleanup(func() { sudoRemove(configPath) })

	t.Run("SetupDirectives", func(t *testing.T) {
		action := &pb.Action{
			Id:           &pb.ActionId{Value: actionID},
			Type:         pb.ActionType_ACTION_TYPE_SSHD,
			DesiredState: pb.DesiredState_DESIRED_STATE_PRESENT,
			Params: &pb.Action_Sshd{Sshd: &pb.SshdParams{
				Priority: priority,
				Directives: []*pb.SshdDirective{
					{Key: "MaxAuthTries", Value: "3"},
					{Key: "LoginGraceTime", Value: "60"},
				},
			}},
		}
		result := e.ExecuteAction(ctx, testAction(action))
		assertSuccess(t, result)
		assertChanged(t, result, true)
		if !sudoFileExists(configPath) {
			t.Error("SSHD config not created")
		}
	})

	t.Run("SetupIdempotent", func(t *testing.T) {
		action := &pb.Action{
			Id:           &pb.ActionId{Value: actionID},
			Type:         pb.ActionType_ACTION_TYPE_SSHD,
			DesiredState: pb.DesiredState_DESIRED_STATE_PRESENT,
			Params: &pb.Action_Sshd{Sshd: &pb.SshdParams{
				Priority: priority,
				Directives: []*pb.SshdDirective{
					{Key: "MaxAuthTries", Value: "3"},
					{Key: "LoginGraceTime", Value: "60"},
				},
			}},
		}
		result := e.ExecuteAction(ctx, testAction(action))
		assertSuccess(t, result)
		assertChanged(t, result, false)
	})

	t.Run("Remove", func(t *testing.T) {
		action := &pb.Action{
			Id:           &pb.ActionId{Value: actionID},
			Type:         pb.ActionType_ACTION_TYPE_SSHD,
			DesiredState: pb.DesiredState_DESIRED_STATE_ABSENT,
			Params:       &pb.Action_Sshd{Sshd: &pb.SshdParams{Priority: priority}},
		}
		result := e.ExecuteAction(ctx, testAction(action))
		assertSuccess(t, result)
		assertChanged(t, result, true)
		if sudoFileExists(configPath) {
			t.Error("SSHD config still exists")
		}
	})
}

func TestIntegration_Systemd(t *testing.T) {
	e := newTestExecutor()
	ctx := context.Background()

	t.Run("ProtectAgent", func(t *testing.T) {
		action := makeAction(t, pb.ActionType_ACTION_TYPE_SERVICE, pb.DesiredState_DESIRED_STATE_PRESENT)
		action.Params = &pb.Action_Service{Service: &pb.ServiceParams{
			UnitName:     "cadestrod",
			DesiredState: pb.ServiceUnitState_SERVICE_UNIT_STATE_STOPPED,
		}}
		result := e.ExecuteAction(ctx, testAction(action))
		assertFailed(t, result)
	})

	t.Run("WriteUnitFile", func(t *testing.T) {
		unitName := "cadestro-integration-test.service"
		unitContent := `[Unit]
Description=Cadestro Integration Test

[Service]
ExecStart=/bin/true

[Install]
WantedBy=multi-user.target
`
		unitPath := "/etc/systemd/system/" + unitName
		t.Cleanup(func() { sudoRemove(unitPath) })

		action := makeAction(t, pb.ActionType_ACTION_TYPE_SERVICE, pb.DesiredState_DESIRED_STATE_PRESENT)
		action.Params = &pb.Action_Service{Service: &pb.ServiceParams{
			UnitName:    unitName,
			UnitContent: unitContent,
		}}
		result := e.ExecuteAction(ctx, testAction(action))

		assertFailed(t, result)
		if _, err := os.Stat(unitPath); err != nil {
			t.Errorf("unit file not created despite daemon-reload failure: %v", err)
		}
	})
}

func TestIntegration_LPS(t *testing.T) {
	e := newTestExecutor()
	ctx := context.Background()
	actionID := "lpstest01"
	username := "cadestrolpsuser"

	ensureTestUser(t, username)
	t.Cleanup(func() {
		cleanupTestUser(t, username)
		_ = e.store.DeleteLpsState(ctx, actionID)
	})

	t.Run("InitialRotation", func(t *testing.T) {
		action := &pb.Action{
			Id:           &pb.ActionId{Value: actionID},
			Type:         pb.ActionType_ACTION_TYPE_LPS,
			DesiredState: pb.DesiredState_DESIRED_STATE_PRESENT,
			Params: &pb.Action_Lps{Lps: &pb.LpsParams{
				Usernames:            []string{username},
				PasswordLength:       16,
				Complexity:           pb.LpsPasswordComplexity_LPS_PASSWORD_COMPLEXITY_ALPHANUMERIC,
				RotationIntervalDays: 365,
			}},
		}
		result := e.ExecuteAction(ctx, testAction(action))
		assertSuccess(t, result)
		assertChanged(t, result, true)
		if testLpsReports.reportedFor(username) == nil {
			t.Error("expected the rotated password to be reported to control")
		}
	})

	t.Run("IdempotentNoRotation", func(t *testing.T) {
		action := &pb.Action{
			Id:           &pb.ActionId{Value: actionID},
			Type:         pb.ActionType_ACTION_TYPE_LPS,
			DesiredState: pb.DesiredState_DESIRED_STATE_PRESENT,
			Params: &pb.Action_Lps{Lps: &pb.LpsParams{
				Usernames:            []string{username},
				PasswordLength:       16,
				Complexity:           pb.LpsPasswordComplexity_LPS_PASSWORD_COMPLEXITY_ALPHANUMERIC,
				RotationIntervalDays: 365,
			}},
		}
		result := e.ExecuteAction(ctx, testAction(action))
		assertSuccess(t, result)
		assertChanged(t, result, false)
	})

	t.Run("RemoveManagement", func(t *testing.T) {
		action := &pb.Action{
			Id:           &pb.ActionId{Value: actionID},
			Type:         pb.ActionType_ACTION_TYPE_LPS,
			DesiredState: pb.DesiredState_DESIRED_STATE_ABSENT,
			Params: &pb.Action_Lps{Lps: &pb.LpsParams{
				Usernames: []string{username},
			}},
		}
		result := e.ExecuteAction(ctx, testAction(action))
		assertSuccess(t, result)
		assertChanged(t, result, true)
		states, err := e.store.GetLpsState(ctx, actionID)
		if err != nil {
			t.Fatalf("failed to check LPS state: %v", err)
		}
		if len(states) > 0 {
			t.Error("LPS state still exists after removal")
		}
	})
}

func TestIntegration_Deb(t *testing.T) {
	skipIfNoApt(t)
	e := newTestExecutor()
	ctx := context.Background()

	t.Cleanup(func() {
		sudoRun("dpkg", "-r", "cadestro-testpkg").Run()
	})

	t.Run("Install", func(t *testing.T) {
		debData := createTestDeb(t)
		ts := startFileServer(t, map[string][]byte{
			"/cadestro-testpkg_1.0.0_all.deb": debData,
		})

		action := makeAction(t, pb.ActionType_ACTION_TYPE_DEB, pb.DesiredState_DESIRED_STATE_PRESENT)
		action.Params = &pb.Action_App{App: &pb.AppInstallParams{
			Url:            ts.URL + "/cadestro-testpkg_1.0.0_all.deb",
			ChecksumSha256: sha256hex(debData),
		}}
		result := e.ExecuteAction(ctx, testAction(action))
		assertSuccess(t, result)
		assertChanged(t, result, true)
		if !checkCmdSuccess("dpkg", "-s", "cadestro-testpkg") {
			t.Error("cadestro-testpkg not installed")
		}
	})

	t.Run("RemoveAbsent", func(t *testing.T) {

		sudoRun("dpkg", "-r", "cadestro-testpkg").Run()

		action := makeAction(t, pb.ActionType_ACTION_TYPE_DEB, pb.DesiredState_DESIRED_STATE_ABSENT)
		action.Params = &pb.Action_App{App: &pb.AppInstallParams{
			Url: "http://example.com/cadestro-notinstalled_1.0.0_all.deb",
		}}
		result := e.ExecuteAction(ctx, testAction(action))
		assertSuccess(t, result)
		assertChanged(t, result, false)
	})
}

func TestIntegration_AppImage(t *testing.T) {
	e := newTestExecutor()
	ctx := context.Background()
	installDir := "/tmp/cadestro-integration-appimages"
	fileName := "test-app.AppImage"

	t.Cleanup(func() { sudoRemoveAll(installDir) })

	dummyContent := []byte("#!/bin/sh\necho test\n")
	checksum := sha256.Sum256(dummyContent)
	checksumHex := hex.EncodeToString(checksum[:])
	ts := startFileServer(t, map[string][]byte{
		"/" + fileName: dummyContent,
	})

	t.Run("Install", func(t *testing.T) {
		action := makeAction(t, pb.ActionType_ACTION_TYPE_APP_IMAGE, pb.DesiredState_DESIRED_STATE_PRESENT)
		action.Params = &pb.Action_App{App: &pb.AppInstallParams{
			Url:            ts.URL + "/" + fileName,
			ChecksumSha256: checksumHex,
			InstallPath:    installDir,
		}}
		result := e.ExecuteAction(ctx, testAction(action))
		assertSuccess(t, result)
		assertChanged(t, result, true)
		fullPath := filepath.Join(installDir, fileName)
		info, err := os.Stat(fullPath)
		if err != nil {
			t.Fatalf("AppImage not installed: %v", err)
		}
		if info.Mode()&0111 == 0 {
			t.Error("AppImage not executable")
		}
	})

	t.Run("InstallIdempotent", func(t *testing.T) {

		action := makeAction(t, pb.ActionType_ACTION_TYPE_APP_IMAGE, pb.DesiredState_DESIRED_STATE_PRESENT)
		action.Params = &pb.Action_App{App: &pb.AppInstallParams{
			Url:            ts.URL + "/" + fileName,
			ChecksumSha256: checksumHex,
			InstallPath:    installDir,
		}}
		result := e.ExecuteAction(ctx, testAction(action))
		assertSuccess(t, result)
		assertChanged(t, result, false)
	})

	t.Run("Remove", func(t *testing.T) {
		action := makeAction(t, pb.ActionType_ACTION_TYPE_APP_IMAGE, pb.DesiredState_DESIRED_STATE_ABSENT)
		action.Params = &pb.Action_App{App: &pb.AppInstallParams{
			Url:         ts.URL + "/" + fileName,
			InstallPath: installDir,
		}}
		result := e.ExecuteAction(ctx, testAction(action))
		assertSuccess(t, result)
		assertChanged(t, result, true)
		fullPath := filepath.Join(installDir, fileName)
		if _, err := os.Stat(fullPath); !os.IsNotExist(err) {
			t.Error("AppImage still exists")
		}
	})
}

func TestIntegration_Repository(t *testing.T) {
	skipIfNoApt(t)
	e := newTestExecutor()
	ctx := context.Background()
	repoName := "cadestrotestrepo"

	t.Cleanup(func() {
		sudoRemove(fmt.Sprintf("/etc/apt/sources.list.d/%s.sources", repoName))
		sudoRemove(fmt.Sprintf("/etc/apt/keyrings/%s.gpg", repoName))
	})

	t.Run("AddApt", func(t *testing.T) {
		action := makeAction(t, pb.ActionType_ACTION_TYPE_REPOSITORY, pb.DesiredState_DESIRED_STATE_PRESENT)
		action.Params = &pb.Action_Repository{Repository: &pb.RepositoryParams{
			Name: repoName,
			Apt: &pb.AptRepository{
				Url:          "https://example.com/apt",
				Distribution: "bookworm",
				Components:   []string{"main"},
				Trusted:      true,
			},
		}}
		result := e.ExecuteAction(ctx, testAction(action))
		assertSuccess(t, result)

		sourcesFile := fmt.Sprintf("/etc/apt/sources.list.d/%s.sources", repoName)
		if _, err := os.Stat(sourcesFile); err != nil {
			t.Errorf("sources file not created: %v", err)
		}
	})

	t.Run("RemoveApt", func(t *testing.T) {
		action := makeAction(t, pb.ActionType_ACTION_TYPE_REPOSITORY, pb.DesiredState_DESIRED_STATE_ABSENT)
		action.Params = &pb.Action_Repository{Repository: &pb.RepositoryParams{
			Name: repoName,
			Apt: &pb.AptRepository{
				Url: "https://example.com/apt",
			},
		}}
		result := e.ExecuteAction(ctx, testAction(action))
		assertSuccess(t, result)

		sourcesFile := fmt.Sprintf("/etc/apt/sources.list.d/%s.sources", repoName)
		if _, err := os.Stat(sourcesFile); !os.IsNotExist(err) {
			t.Error("sources file still exists")
		}
	})
}

func TestIntegration_Package_Dnf(t *testing.T) {
	skipIfNoDnf(t)
	e := newTestExecutor()
	ctx := context.Background()

	sudoRun("dnf", "remove", "-y", "tree").Run()

	t.Run("Install", func(t *testing.T) {
		action := makeAction(t, pb.ActionType_ACTION_TYPE_PACKAGE, pb.DesiredState_DESIRED_STATE_PRESENT)
		action.Params = &pb.Action_Package{Package: &pb.PackageParams{Name: "tree"}}
		result := e.ExecuteAction(ctx, testAction(action))
		assertSuccess(t, result)
		assertChanged(t, result, true)
		if !isRpmInstalled("tree") {
			t.Error("tree not installed after action")
		}
	})

	t.Run("InstallIdempotent", func(t *testing.T) {
		action := makeAction(t, pb.ActionType_ACTION_TYPE_PACKAGE, pb.DesiredState_DESIRED_STATE_PRESENT)
		action.Params = &pb.Action_Package{Package: &pb.PackageParams{Name: "tree"}}
		result := e.ExecuteAction(ctx, testAction(action))
		assertSuccess(t, result)
		assertChanged(t, result, false)
	})

	t.Run("Remove", func(t *testing.T) {
		action := makeAction(t, pb.ActionType_ACTION_TYPE_PACKAGE, pb.DesiredState_DESIRED_STATE_ABSENT)
		action.Params = &pb.Action_Package{Package: &pb.PackageParams{Name: "tree"}}
		result := e.ExecuteAction(ctx, testAction(action))
		assertSuccess(t, result)
		assertChanged(t, result, true)
		if isRpmInstalled("tree") {
			t.Error("tree still installed after removal")
		}
	})

	t.Run("InstallNonExistent", func(t *testing.T) {
		action := makeAction(t, pb.ActionType_ACTION_TYPE_PACKAGE, pb.DesiredState_DESIRED_STATE_PRESENT)
		action.Params = &pb.Action_Package{Package: &pb.PackageParams{Name: "this-package-does-not-exist-xyz"}}
		result := e.ExecuteAction(ctx, testAction(action))
		assertFailed(t, result)
	})
}

func TestIntegration_Package_GracefulSkip_Dnf(t *testing.T) {
	skipIfNoDnf(t)
	e := newTestExecutor()
	ctx := context.Background()

	t.Run("AptNameOnly", func(t *testing.T) {
		action := makeAction(t, pb.ActionType_ACTION_TYPE_PACKAGE, pb.DesiredState_DESIRED_STATE_PRESENT)
		action.Params = &pb.Action_Package{Package: &pb.PackageParams{AptName: "some-apt-pkg"}}
		result := e.ExecuteAction(ctx, testAction(action))
		assertNotApplicable(t, result, "no package name configured")
	})

	t.Run("PacmanNameOnly", func(t *testing.T) {
		action := makeAction(t, pb.ActionType_ACTION_TYPE_PACKAGE, pb.DesiredState_DESIRED_STATE_PRESENT)
		action.Params = &pb.Action_Package{Package: &pb.PackageParams{PacmanName: "some-pacman-pkg"}}
		result := e.ExecuteAction(ctx, testAction(action))
		assertNotApplicable(t, result, "no package name configured")
	})

	t.Run("ZypperNameOnly", func(t *testing.T) {
		action := makeAction(t, pb.ActionType_ACTION_TYPE_PACKAGE, pb.DesiredState_DESIRED_STATE_PRESENT)
		action.Params = &pb.Action_Package{Package: &pb.PackageParams{ZypperName: "some-zypper-pkg"}}
		result := e.ExecuteAction(ctx, testAction(action))
		assertNotApplicable(t, result, "no package name configured")
	})
}

func TestIntegration_Update_Dnf(t *testing.T) {
	skipIfNoDnf(t)
	e := newTestExecutor()
	ctx := context.Background()

	t.Run("DnfUpgrade", func(t *testing.T) {
		action := makeAction(t, pb.ActionType_ACTION_TYPE_UPDATE, pb.DesiredState_DESIRED_STATE_PRESENT)
		action.Params = &pb.Action_Update{Update: &pb.UpdateParams{}}
		result := e.ExecuteAction(ctx, testAction(action))
		assertSuccess(t, result)
	})
}

func TestIntegration_Repository_Dnf(t *testing.T) {
	skipIfNoDnf(t)
	e := newTestExecutor()
	ctx := context.Background()
	repoName := "cadestrotestrepo"

	t.Cleanup(func() {
		sudoRemove(fmt.Sprintf("/etc/yum.repos.d/%s.repo", repoName))
	})

	t.Run("AddDnf", func(t *testing.T) {
		action := makeAction(t, pb.ActionType_ACTION_TYPE_REPOSITORY, pb.DesiredState_DESIRED_STATE_PRESENT)
		action.Params = &pb.Action_Repository{Repository: &pb.RepositoryParams{
			Name: repoName,
			Dnf: &pb.DnfRepository{
				Baseurl:     "https://example.com/repo",
				Description: "Cadestro Test Repo",
				Enabled:     true,
				Gpgcheck:    false,
			},
		}}
		result := e.ExecuteAction(ctx, testAction(action))
		assertSuccess(t, result)

		repoFile := fmt.Sprintf("/etc/yum.repos.d/%s.repo", repoName)
		if _, err := os.Stat(repoFile); err != nil {
			t.Errorf("repo file not created: %v", err)
		}
	})

	t.Run("RemoveDnf", func(t *testing.T) {
		action := makeAction(t, pb.ActionType_ACTION_TYPE_REPOSITORY, pb.DesiredState_DESIRED_STATE_ABSENT)
		action.Params = &pb.Action_Repository{Repository: &pb.RepositoryParams{
			Name: repoName,
			Dnf: &pb.DnfRepository{
				Baseurl: "https://example.com/repo",
			},
		}}
		result := e.ExecuteAction(ctx, testAction(action))
		assertSuccess(t, result)

		repoFile := fmt.Sprintf("/etc/yum.repos.d/%s.repo", repoName)
		if _, err := os.Stat(repoFile); !os.IsNotExist(err) {
			t.Error("repo file still exists")
		}
	})
}

func TestIntegration_Rpm(t *testing.T) {
	if _, err := exec.LookPath("rpm"); err != nil {
		t.Skip("rpm not found, skipping")
	}
	skipIfNoRpmBuild(t)
	e := newTestExecutor()
	ctx := context.Background()

	t.Cleanup(func() {
		sudoRun("rpm", "-e", "cadestrotestrpm").Run()
	})

	t.Run("Install", func(t *testing.T) {
		rpmData := createTestRpm(t)
		ts := startFileServer(t, map[string][]byte{
			"/cadestrotestrpm-1.0.0-1.noarch.rpm": rpmData,
		})

		action := makeAction(t, pb.ActionType_ACTION_TYPE_RPM, pb.DesiredState_DESIRED_STATE_PRESENT)
		action.Params = &pb.Action_App{App: &pb.AppInstallParams{
			Url:            ts.URL + "/cadestrotestrpm-1.0.0-1.noarch.rpm",
			ChecksumSha256: sha256hex(rpmData),
		}}
		result := e.ExecuteAction(ctx, testAction(action))
		assertSuccess(t, result)
		assertChanged(t, result, true)
		if !isRpmInstalled("cadestrotestrpm") {
			t.Error("cadestrotestrpm not installed")
		}
	})

	t.Run("InstallIdempotent", func(t *testing.T) {
		rpmData := createTestRpm(t)
		ts := startFileServer(t, map[string][]byte{
			"/cadestrotestrpm-1.0.0-1.noarch.rpm": rpmData,
		})

		action := makeAction(t, pb.ActionType_ACTION_TYPE_RPM, pb.DesiredState_DESIRED_STATE_PRESENT)
		action.Params = &pb.Action_App{App: &pb.AppInstallParams{
			Url:            ts.URL + "/cadestrotestrpm-1.0.0-1.noarch.rpm",
			ChecksumSha256: sha256hex(rpmData),
		}}
		result := e.ExecuteAction(ctx, testAction(action))
		assertSuccess(t, result)
		assertChanged(t, result, false)
	})

	t.Run("Remove", func(t *testing.T) {

		rpmData := createTestRpm(t)
		ts := startFileServer(t, map[string][]byte{
			"/cadestrotestrpm-1.0.0-1.noarch.rpm": rpmData,
		})

		action := makeAction(t, pb.ActionType_ACTION_TYPE_RPM, pb.DesiredState_DESIRED_STATE_ABSENT)
		action.Params = &pb.Action_App{App: &pb.AppInstallParams{
			Url:            ts.URL + "/cadestrotestrpm-1.0.0-1.noarch.rpm",
			ChecksumSha256: sha256hex(rpmData),
		}}
		result := e.ExecuteAction(ctx, testAction(action))
		assertSuccess(t, result)
		assertChanged(t, result, true)
		if isRpmInstalled("cadestrotestrpm") {
			t.Error("cadestrotestrpm still installed after removal")
		}
	})

	t.Run("RemoveAbsent", func(t *testing.T) {

		rpmData := createTestRpm(t)
		ts := startFileServer(t, map[string][]byte{
			"/cadestrotestrpm-1.0.0-1.noarch.rpm": rpmData,
		})

		action := makeAction(t, pb.ActionType_ACTION_TYPE_RPM, pb.DesiredState_DESIRED_STATE_ABSENT)
		action.Params = &pb.Action_App{App: &pb.AppInstallParams{
			Url:            ts.URL + "/cadestrotestrpm-1.0.0-1.noarch.rpm",
			ChecksumSha256: sha256hex(rpmData),
		}}
		result := e.ExecuteAction(ctx, testAction(action))
		assertSuccess(t, result)
		assertChanged(t, result, false)
	})
}

func TestIntegration_Package_Pacman(t *testing.T) {
	skipIfNoPacman(t)
	e := newTestExecutor()
	ctx := context.Background()

	sudoRun("pacman", "-Rns", "--noconfirm", "tree").Run()

	t.Run("Install", func(t *testing.T) {
		action := makeAction(t, pb.ActionType_ACTION_TYPE_PACKAGE, pb.DesiredState_DESIRED_STATE_PRESENT)
		action.Params = &pb.Action_Package{Package: &pb.PackageParams{Name: "tree"}}
		result := e.ExecuteAction(ctx, testAction(action))
		assertSuccess(t, result)
		assertChanged(t, result, true)
		if !isPacmanInstalled("tree") {
			t.Error("tree not installed after action")
		}
	})

	t.Run("InstallIdempotent", func(t *testing.T) {
		action := makeAction(t, pb.ActionType_ACTION_TYPE_PACKAGE, pb.DesiredState_DESIRED_STATE_PRESENT)
		action.Params = &pb.Action_Package{Package: &pb.PackageParams{Name: "tree"}}
		result := e.ExecuteAction(ctx, testAction(action))
		assertSuccess(t, result)
		assertChanged(t, result, false)
	})

	t.Run("Remove", func(t *testing.T) {
		action := makeAction(t, pb.ActionType_ACTION_TYPE_PACKAGE, pb.DesiredState_DESIRED_STATE_ABSENT)
		action.Params = &pb.Action_Package{Package: &pb.PackageParams{Name: "tree"}}
		result := e.ExecuteAction(ctx, testAction(action))
		assertSuccess(t, result)
		assertChanged(t, result, true)
		if isPacmanInstalled("tree") {
			t.Error("tree still installed after removal")
		}
	})

	t.Run("InstallNonExistent", func(t *testing.T) {
		action := makeAction(t, pb.ActionType_ACTION_TYPE_PACKAGE, pb.DesiredState_DESIRED_STATE_PRESENT)
		action.Params = &pb.Action_Package{Package: &pb.PackageParams{Name: "this-package-does-not-exist-xyz"}}
		result := e.ExecuteAction(ctx, testAction(action))
		assertFailed(t, result)
	})
}

func TestIntegration_Package_GracefulSkip_Pacman(t *testing.T) {
	skipIfNoPacman(t)
	e := newTestExecutor()
	ctx := context.Background()

	t.Run("AptNameOnly", func(t *testing.T) {
		action := makeAction(t, pb.ActionType_ACTION_TYPE_PACKAGE, pb.DesiredState_DESIRED_STATE_PRESENT)
		action.Params = &pb.Action_Package{Package: &pb.PackageParams{AptName: "some-apt-pkg"}}
		result := e.ExecuteAction(ctx, testAction(action))
		assertNotApplicable(t, result, "no package name configured")
	})

	t.Run("DnfNameOnly", func(t *testing.T) {
		action := makeAction(t, pb.ActionType_ACTION_TYPE_PACKAGE, pb.DesiredState_DESIRED_STATE_PRESENT)
		action.Params = &pb.Action_Package{Package: &pb.PackageParams{DnfName: "some-dnf-pkg"}}
		result := e.ExecuteAction(ctx, testAction(action))
		assertNotApplicable(t, result, "no package name configured")
	})

	t.Run("ZypperNameOnly", func(t *testing.T) {
		action := makeAction(t, pb.ActionType_ACTION_TYPE_PACKAGE, pb.DesiredState_DESIRED_STATE_PRESENT)
		action.Params = &pb.Action_Package{Package: &pb.PackageParams{ZypperName: "some-zypper-pkg"}}
		result := e.ExecuteAction(ctx, testAction(action))
		assertNotApplicable(t, result, "no package name configured")
	})
}

func TestIntegration_Update_Pacman(t *testing.T) {
	skipIfNoPacman(t)
	e := newTestExecutor()
	ctx := context.Background()

	t.Run("PacmanUpgrade", func(t *testing.T) {
		action := makeAction(t, pb.ActionType_ACTION_TYPE_UPDATE, pb.DesiredState_DESIRED_STATE_PRESENT)
		action.Params = &pb.Action_Update{Update: &pb.UpdateParams{}}
		result := e.ExecuteAction(ctx, testAction(action))
		assertSuccess(t, result)
	})
}

func TestIntegration_Repository_Pacman(t *testing.T) {
	skipIfNoPacman(t)
	e := newTestExecutor()
	ctx := context.Background()
	repoName := "cadestrotestrepo"

	t.Cleanup(func() {

		content, err := os.ReadFile("/etc/pacman.conf")
		if err == nil {
			cleaned := removePacmanSection(string(content), repoName)
			sudoWriteFile("/etc/pacman.conf", []byte(cleaned))
		}
	})

	t.Run("AddPacman", func(t *testing.T) {
		action := makeAction(t, pb.ActionType_ACTION_TYPE_REPOSITORY, pb.DesiredState_DESIRED_STATE_PRESENT)
		action.Params = &pb.Action_Repository{Repository: &pb.RepositoryParams{
			Name: repoName,
			Pacman: &pb.PacmanRepository{
				Server:   "https://example.com/$repo/os/$arch",
				SigLevel: "Optional TrustAll",
			},
		}}
		result := e.ExecuteAction(ctx, testAction(action))
		assertSuccess(t, result)

		content, err := os.ReadFile("/etc/pacman.conf")
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(content), "["+repoName+"]") {
			t.Error("repo section not found in pacman.conf")
		}
	})

	t.Run("RemovePacman", func(t *testing.T) {
		action := makeAction(t, pb.ActionType_ACTION_TYPE_REPOSITORY, pb.DesiredState_DESIRED_STATE_ABSENT)
		action.Params = &pb.Action_Repository{Repository: &pb.RepositoryParams{
			Name: repoName,
			Pacman: &pb.PacmanRepository{
				Server: "https://example.com/$repo/os/$arch",
			},
		}}
		result := e.ExecuteAction(ctx, testAction(action))
		assertSuccess(t, result)

		content, err := os.ReadFile("/etc/pacman.conf")
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(content), "["+repoName+"]") {
			t.Error("repo section still in pacman.conf")
		}
	})
}

func TestIntegration_Package_Zypper(t *testing.T) {
	skipIfNoZypper(t)
	e := newTestExecutor()
	ctx := context.Background()

	sudoRun("zypper", "--non-interactive", "remove", "tree").Run()

	t.Run("Install", func(t *testing.T) {
		action := makeAction(t, pb.ActionType_ACTION_TYPE_PACKAGE, pb.DesiredState_DESIRED_STATE_PRESENT)
		action.Params = &pb.Action_Package{Package: &pb.PackageParams{Name: "tree"}}
		result := e.ExecuteAction(ctx, testAction(action))
		assertSuccess(t, result)
		assertChanged(t, result, true)
		if !isRpmInstalled("tree") {
			t.Error("tree not installed after action")
		}
	})

	t.Run("InstallIdempotent", func(t *testing.T) {
		action := makeAction(t, pb.ActionType_ACTION_TYPE_PACKAGE, pb.DesiredState_DESIRED_STATE_PRESENT)
		action.Params = &pb.Action_Package{Package: &pb.PackageParams{Name: "tree"}}
		result := e.ExecuteAction(ctx, testAction(action))
		assertSuccess(t, result)
		assertChanged(t, result, false)
	})

	t.Run("Remove", func(t *testing.T) {
		action := makeAction(t, pb.ActionType_ACTION_TYPE_PACKAGE, pb.DesiredState_DESIRED_STATE_ABSENT)
		action.Params = &pb.Action_Package{Package: &pb.PackageParams{Name: "tree"}}
		result := e.ExecuteAction(ctx, testAction(action))
		assertSuccess(t, result)
		assertChanged(t, result, true)
		if isRpmInstalled("tree") {
			t.Error("tree still installed after removal")
		}
	})

	t.Run("InstallNonExistent", func(t *testing.T) {
		action := makeAction(t, pb.ActionType_ACTION_TYPE_PACKAGE, pb.DesiredState_DESIRED_STATE_PRESENT)
		action.Params = &pb.Action_Package{Package: &pb.PackageParams{Name: "this-package-does-not-exist-xyz"}}
		result := e.ExecuteAction(ctx, testAction(action))
		assertFailed(t, result)
	})
}

func TestIntegration_Package_GracefulSkip_Zypper(t *testing.T) {
	skipIfNoZypper(t)
	e := newTestExecutor()
	ctx := context.Background()

	t.Run("AptNameOnly", func(t *testing.T) {
		action := makeAction(t, pb.ActionType_ACTION_TYPE_PACKAGE, pb.DesiredState_DESIRED_STATE_PRESENT)
		action.Params = &pb.Action_Package{Package: &pb.PackageParams{AptName: "some-apt-pkg"}}
		result := e.ExecuteAction(ctx, testAction(action))
		assertNotApplicable(t, result, "no package name configured")
	})

	t.Run("DnfNameOnly", func(t *testing.T) {
		action := makeAction(t, pb.ActionType_ACTION_TYPE_PACKAGE, pb.DesiredState_DESIRED_STATE_PRESENT)
		action.Params = &pb.Action_Package{Package: &pb.PackageParams{DnfName: "some-dnf-pkg"}}
		result := e.ExecuteAction(ctx, testAction(action))
		assertNotApplicable(t, result, "no package name configured")
	})

	t.Run("PacmanNameOnly", func(t *testing.T) {
		action := makeAction(t, pb.ActionType_ACTION_TYPE_PACKAGE, pb.DesiredState_DESIRED_STATE_PRESENT)
		action.Params = &pb.Action_Package{Package: &pb.PackageParams{PacmanName: "some-pacman-pkg"}}
		result := e.ExecuteAction(ctx, testAction(action))
		assertNotApplicable(t, result, "no package name configured")
	})
}

func TestIntegration_Update_Zypper(t *testing.T) {
	skipIfNoZypper(t)
	e := newTestExecutor()
	ctx := context.Background()

	t.Run("ZypperUpdate", func(t *testing.T) {
		action := makeAction(t, pb.ActionType_ACTION_TYPE_UPDATE, pb.DesiredState_DESIRED_STATE_PRESENT)
		action.Params = &pb.Action_Update{Update: &pb.UpdateParams{}}
		result := e.ExecuteAction(ctx, testAction(action))
		assertSuccess(t, result)
	})
}

func TestIntegration_Repository_Zypper(t *testing.T) {
	skipIfNoZypper(t)
	e := newTestExecutor()
	ctx := context.Background()
	repoName := "cadestrotestrepo"

	t.Cleanup(func() {
		sudoRun("zypper", "--non-interactive", "removerepo", repoName).Run()
	})

	t.Run("AddZypper", func(t *testing.T) {
		action := makeAction(t, pb.ActionType_ACTION_TYPE_REPOSITORY, pb.DesiredState_DESIRED_STATE_PRESENT)
		action.Params = &pb.Action_Repository{Repository: &pb.RepositoryParams{
			Name: repoName,
			Zypper: &pb.ZypperRepository{
				Url:         "https://example.com/repo",
				Description: "Cadestro Test Repo",
				Enabled:     true,
				Gpgcheck:    false,
			},
		}}
		result := e.ExecuteAction(ctx, testAction(action))
		assertSuccess(t, result)

		if !checkCmdSuccess("zypper", "lr", repoName) {
			t.Error("repository not listed by zypper")
		}
	})

	t.Run("RemoveZypper", func(t *testing.T) {
		action := makeAction(t, pb.ActionType_ACTION_TYPE_REPOSITORY, pb.DesiredState_DESIRED_STATE_ABSENT)
		action.Params = &pb.Action_Repository{Repository: &pb.RepositoryParams{
			Name: repoName,
			Zypper: &pb.ZypperRepository{
				Url: "https://example.com/repo",
			},
		}}
		result := e.ExecuteAction(ctx, testAction(action))
		assertSuccess(t, result)
	})
}

func skipIfNotPrivileged(t *testing.T) {
	t.Helper()
	testDir := "/tmp/cadestro-priv-check"
	os.MkdirAll(testDir, 0755)
	defer sudoRemoveAll(testDir)

	cmd := exec.Command("sudo", "-n", "mount", "-t", "tmpfs", "-o", "size=1M", "tmpfs", testDir)
	if err := cmd.Run(); err != nil {
		t.Skip("container not privileged, skipping (need --privileged for mount)")
	}
	exec.Command("sudo", "-n", "umount", testDir).Run()
}

func startFailingServer(t *testing.T, statusCode int) *httptest.Server {
	t.Helper()

	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(statusCode)
		w.Write([]byte(http.StatusText(statusCode)))
	}))
	t.Cleanup(ts.Close)
	return ts
}

func startSlowServer(t *testing.T, delay time.Duration) *httptest.Server {
	t.Helper()
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(delay)
		w.Write([]byte("slow response"))
	}))
	t.Cleanup(ts.Close)
	return ts
}

func TestIntegration_EdgeCase_LpsNoPriorState(t *testing.T) {
	e := newTestExecutor()
	ctx := context.Background()
	actionID := "lpsedge01"

	ensureTestUser(t, "cadestrolpsedge")
	t.Cleanup(func() {
		_ = e.store.DeleteLpsState(ctx, actionID)
		cleanupTestUser(t, "cadestrolpsedge")
	})

	action := &pb.Action{
		Id:           &pb.ActionId{Value: actionID},
		Type:         pb.ActionType_ACTION_TYPE_LPS,
		DesiredState: pb.DesiredState_DESIRED_STATE_PRESENT,
		Params: &pb.Action_Lps{Lps: &pb.LpsParams{
			Usernames:            []string{"cadestrolpsedge"},
			PasswordLength:       16,
			RotationIntervalDays: 30,
			Complexity:           pb.LpsPasswordComplexity_LPS_PASSWORD_COMPLEXITY_ALPHANUMERIC,
		}},
	}
	result := e.ExecuteAction(ctx, testAction(action))
	assertSuccess(t, result)

	states, err := e.store.GetLpsState(ctx, actionID)
	if err != nil {
		t.Fatalf("failed to get LPS state: %v", err)
	}
	if len(states) == 0 {
		t.Error("LPS state not written after initial rotation")
	}
	if us, ok := states["cadestrolpsedge"]; !ok {
		t.Error("LPS state missing for user cadestrolpsedge")
	} else if us.PasswordHash == "" {
		t.Error("LPS password hash is empty")
	}
}

func TestIntegration_EdgeCase_MissingSudoersDir(t *testing.T) {
	e := newTestExecutor()
	ctx := context.Background()
	actionID := "sudoedge01"

	ensureTestUser(t, "cadestrosudoedge")

	backupDir := "/etc/sudoers.d.bak"
	origDir := "/etc/sudoers.d"

	sudoRun("cp", "/etc/sudoers", "/etc/sudoers.bak").Run()
	sudoRun("sh", "-c", "cat /etc/sudoers.d/cadestro >> /etc/sudoers").Run()

	sudoRun("cp", "-a", origDir, backupDir).Run()
	sudoRun("rm", "-rf", origDir).Run()

	t.Cleanup(func() {

		sudoRun("rm", "-rf", origDir).Run()
		sudoRun("mv", backupDir, origDir).Run()

		sudoRun("mv", "/etc/sudoers.bak", "/etc/sudoers").Run()
		cleanupTestUser(t, "cadestrosudoedge")
	})

	action := &pb.Action{
		Id:           &pb.ActionId{Value: actionID},
		Type:         pb.ActionType_ACTION_TYPE_ADMIN_POLICY,
		DesiredState: pb.DesiredState_DESIRED_STATE_PRESENT,
		Params: &pb.Action_AdminPolicy{AdminPolicy: &pb.AdminPolicyParams{
			AccessLevel: pb.AdminAccessLevel_ADMIN_ACCESS_LEVEL_FULL,
			Users:       []string{"cadestrosudoedge"},
		}},
	}
	result := e.ExecuteAction(ctx, testAction(action))

	assertFailed(t, result)
}

func TestIntegration_EdgeCase_MissingSshdConfigDir(t *testing.T) {
	e := newTestExecutor()
	ctx := context.Background()
	actionID := "sshdedge01"

	backupDir := "/etc/ssh/sshd_config.d.bak"
	origDir := "/etc/ssh/sshd_config.d"

	sudoRun("cp", "-a", origDir, backupDir).Run()
	sudoRun("rm", "-rf", origDir).Run()

	t.Cleanup(func() {
		sudoRun("rm", "-rf", origDir).Run()
		sudoRun("mv", backupDir, origDir).Run()
	})

	action := &pb.Action{
		Id:           &pb.ActionId{Value: actionID},
		Type:         pb.ActionType_ACTION_TYPE_SSHD,
		DesiredState: pb.DesiredState_DESIRED_STATE_PRESENT,
		Params: &pb.Action_Sshd{Sshd: &pb.SshdParams{
			Priority: 50,
			Directives: []*pb.SshdDirective{
				{Key: "MaxAuthTries", Value: "5"},
			},
		}},
	}
	result := e.ExecuteAction(ctx, testAction(action))

	assertSuccess(t, result)

	if _, err := os.Stat(origDir); err != nil {
		t.Errorf("sshd_config.d not re-created: %v", err)
	}
}

func TestIntegration_EdgeCase_DownloadHttp500(t *testing.T) {
	e := newTestExecutor()
	ctx := context.Background()

	ts := startFailingServer(t, 500)

	t.Run("AppImage", func(t *testing.T) {
		action := makeAction(t, pb.ActionType_ACTION_TYPE_APP_IMAGE, pb.DesiredState_DESIRED_STATE_PRESENT)
		action.Params = &pb.Action_App{App: &pb.AppInstallParams{
			Url:         ts.URL + "/test.AppImage",
			InstallPath: t.TempDir(),

			ChecksumSha256: strings.Repeat("a", 64),
		}}
		result := e.ExecuteAction(ctx, testAction(action))
		assertFailed(t, result)
		if !strings.Contains(result.Error, "500") {
			t.Errorf("expected error to mention 500, got: %s", result.Error)
		}
	})

	t.Run("RPM", func(t *testing.T) {
		if _, err := exec.LookPath("rpm"); err != nil {
			t.Skip("rpm not found")
		}
		action := makeAction(t, pb.ActionType_ACTION_TYPE_RPM, pb.DesiredState_DESIRED_STATE_PRESENT)
		action.Params = &pb.Action_App{App: &pb.AppInstallParams{
			Url: ts.URL + "/test-1.0-1.noarch.rpm",

			ChecksumSha256: strings.Repeat("a", 64),
		}}
		result := e.ExecuteAction(ctx, testAction(action))
		assertFailed(t, result)
		if !strings.Contains(result.Error, "500") {
			t.Errorf("expected error to mention 500, got: %s", result.Error)
		}
	})

	t.Run("DEB", func(t *testing.T) {
		skipIfNoApt(t)
		action := makeAction(t, pb.ActionType_ACTION_TYPE_DEB, pb.DesiredState_DESIRED_STATE_PRESENT)
		action.Params = &pb.Action_App{App: &pb.AppInstallParams{
			Url: ts.URL + "/test.deb",

			ChecksumSha256: strings.Repeat("a", 64),
		}}
		result := e.ExecuteAction(ctx, testAction(action))
		assertFailed(t, result)
		if !strings.Contains(result.Error, "500") {
			t.Errorf("expected error to mention 500, got: %s", result.Error)
		}
	})
}

func TestIntegration_EdgeCase_DownloadHttp404(t *testing.T) {
	e := newTestExecutor()
	ctx := context.Background()

	ts := startFailingServer(t, 404)

	t.Run("AppImage", func(t *testing.T) {
		action := makeAction(t, pb.ActionType_ACTION_TYPE_APP_IMAGE, pb.DesiredState_DESIRED_STATE_PRESENT)
		action.Params = &pb.Action_App{App: &pb.AppInstallParams{
			Url:         ts.URL + "/test.AppImage",
			InstallPath: t.TempDir(),

			ChecksumSha256: strings.Repeat("a", 64),
		}}
		result := e.ExecuteAction(ctx, testAction(action))
		assertFailed(t, result)
		if !strings.Contains(result.Error, "404") {
			t.Errorf("expected error to mention 404, got: %s", result.Error)
		}
	})

	t.Run("RPM", func(t *testing.T) {
		if _, err := exec.LookPath("rpm"); err != nil {
			t.Skip("rpm not found")
		}
		action := makeAction(t, pb.ActionType_ACTION_TYPE_RPM, pb.DesiredState_DESIRED_STATE_PRESENT)
		action.Params = &pb.Action_App{App: &pb.AppInstallParams{
			Url: ts.URL + "/test-1.0-1.noarch.rpm",

			ChecksumSha256: strings.Repeat("a", 64),
		}}
		result := e.ExecuteAction(ctx, testAction(action))
		assertFailed(t, result)
		if !strings.Contains(result.Error, "404") {
			t.Errorf("expected error to mention 404, got: %s", result.Error)
		}
	})

	t.Run("DEB", func(t *testing.T) {
		skipIfNoApt(t)
		action := makeAction(t, pb.ActionType_ACTION_TYPE_DEB, pb.DesiredState_DESIRED_STATE_PRESENT)
		action.Params = &pb.Action_App{App: &pb.AppInstallParams{
			Url: ts.URL + "/test.deb",

			ChecksumSha256: strings.Repeat("a", 64),
		}}
		result := e.ExecuteAction(ctx, testAction(action))
		assertFailed(t, result)
		if !strings.Contains(result.Error, "404") {
			t.Errorf("expected error to mention 404, got: %s", result.Error)
		}
	})
}

func TestIntegration_EdgeCase_DownloadChecksumMismatch(t *testing.T) {
	e := newTestExecutor()
	ctx := context.Background()

	content := []byte("fake appimage binary content")
	ts := startFileServer(t, map[string][]byte{
		"/test.AppImage": content,
	})

	installDir := t.TempDir()
	wrongChecksum := "0000000000000000000000000000000000000000000000000000000000000000"

	action := makeAction(t, pb.ActionType_ACTION_TYPE_APP_IMAGE, pb.DesiredState_DESIRED_STATE_PRESENT)
	action.Params = &pb.Action_App{App: &pb.AppInstallParams{
		Url:            ts.URL + "/test.AppImage",
		InstallPath:    installDir,
		ChecksumSha256: wrongChecksum,
	}}
	result := e.ExecuteAction(ctx, testAction(action))
	assertFailed(t, result)

	if !strings.Contains(result.Error, "mismatch") {
		t.Errorf("expected an integrity/sha256-mismatch error, got: %s", result.Error)
	}

	installPath := filepath.Join(installDir, "test.AppImage")
	if _, err := os.Stat(installPath); err == nil {
		t.Error("partial file should have been cleaned up after checksum mismatch")
	}
}

func TestIntegration_EdgeCase_DownloadTimeout(t *testing.T) {
	e := newTestExecutor()
	ctx := context.Background()

	ts := startSlowServer(t, 5*time.Second)

	installDir := t.TempDir()

	action := makeAction(t, pb.ActionType_ACTION_TYPE_APP_IMAGE, pb.DesiredState_DESIRED_STATE_PRESENT)
	action.Params = &pb.Action_App{App: &pb.AppInstallParams{
		Url:         ts.URL + "/test.AppImage",
		InstallPath: installDir,

		ChecksumSha256: strings.Repeat("a", 64),
	}}

	action.TimeoutSeconds = 1

	result := e.ExecuteAction(ctx, testAction(action))

	if result.Status != pb.ExecutionStatus_EXECUTION_STATUS_TIMEOUT {
		t.Errorf("expected TIMEOUT status, got %s (error: %s)", result.Status, result.Error)
	}
}

func TestIntegration_EdgeCase_NilParams(t *testing.T) {
	e := newTestExecutor()
	ctx := context.Background()

	tests := []struct {
		name       string
		actionType pb.ActionType
	}{
		{"Package", pb.ActionType_ACTION_TYPE_PACKAGE},
		{"File", pb.ActionType_ACTION_TYPE_FILE},
		{"Directory", pb.ActionType_ACTION_TYPE_DIRECTORY},
		{"AppImage", pb.ActionType_ACTION_TYPE_APP_IMAGE},
		{"RPM", pb.ActionType_ACTION_TYPE_RPM},
		{"DEB", pb.ActionType_ACTION_TYPE_DEB},
		{"User", pb.ActionType_ACTION_TYPE_USER},
		{"Group", pb.ActionType_ACTION_TYPE_GROUP},
		{"Sudo", pb.ActionType_ACTION_TYPE_ADMIN_POLICY},
		{"SSH", pb.ActionType_ACTION_TYPE_SSH},
		{"SSHD", pb.ActionType_ACTION_TYPE_SSHD},
		{"Systemd", pb.ActionType_ACTION_TYPE_SERVICE},
		{"Repository", pb.ActionType_ACTION_TYPE_REPOSITORY},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			action := makeAction(t, tt.actionType, pb.DesiredState_DESIRED_STATE_PRESENT)
			action.Params = nil
			result := e.ExecuteAction(ctx, testAction(action))
			assertFailed(t, result)
			if !strings.Contains(result.Error, "required") {
				t.Errorf("expected 'required' in error, got: %s", result.Error)
			}
		})
	}
}

func TestIntegration_EdgeCase_InvalidUsername(t *testing.T) {
	e := newTestExecutor()
	ctx := context.Background()

	t.Run("StartsWithDigit", func(t *testing.T) {
		action := makeAction(t, pb.ActionType_ACTION_TYPE_USER, pb.DesiredState_DESIRED_STATE_PRESENT)
		action.Params = &pb.Action_User{User: &pb.UserParams{Username: "123bad"}}
		result := e.ExecuteAction(ctx, testAction(action))
		assertFailed(t, result)
		if !strings.Contains(result.Error, "invalid username") {
			t.Errorf("expected 'invalid username' error, got: %s", result.Error)
		}
	})

	t.Run("TooLong", func(t *testing.T) {
		action := makeAction(t, pb.ActionType_ACTION_TYPE_USER, pb.DesiredState_DESIRED_STATE_PRESENT)
		action.Params = &pb.Action_User{User: &pb.UserParams{Username: strings.Repeat("a", 33)}}
		result := e.ExecuteAction(ctx, testAction(action))
		assertFailed(t, result)
		if !strings.Contains(result.Error, "invalid username") {
			t.Errorf("expected 'invalid username' error, got: %s", result.Error)
		}
	})

	t.Run("SpecialChars", func(t *testing.T) {
		action := makeAction(t, pb.ActionType_ACTION_TYPE_USER, pb.DesiredState_DESIRED_STATE_PRESENT)
		action.Params = &pb.Action_User{User: &pb.UserParams{Username: "bad!user"}}
		result := e.ExecuteAction(ctx, testAction(action))
		assertFailed(t, result)
		if !strings.Contains(result.Error, "invalid username") {
			t.Errorf("expected 'invalid username' error, got: %s", result.Error)
		}
	})

	t.Run("Empty", func(t *testing.T) {
		action := makeAction(t, pb.ActionType_ACTION_TYPE_USER, pb.DesiredState_DESIRED_STATE_PRESENT)
		action.Params = &pb.Action_User{User: &pb.UserParams{Username: ""}}
		result := e.ExecuteAction(ctx, testAction(action))
		assertFailed(t, result)
	})
}

func TestIntegration_EdgeCase_InvalidPaths(t *testing.T) {
	e := newTestExecutor()
	ctx := context.Background()

	t.Run("RelativeFilePath", func(t *testing.T) {
		action := makeAction(t, pb.ActionType_ACTION_TYPE_FILE, pb.DesiredState_DESIRED_STATE_PRESENT)
		action.Params = &pb.Action_File{File: &pb.FileParams{
			Path:    "relative/path/file.txt",
			Content: "test",
		}}
		result := e.ExecuteAction(ctx, testAction(action))
		assertFailed(t, result)
		if !strings.Contains(result.Error, "absolute") {
			t.Errorf("expected 'absolute' in error, got: %s", result.Error)
		}
	})

	t.Run("ProtectedDirDelete_Etc", func(t *testing.T) {
		action := makeAction(t, pb.ActionType_ACTION_TYPE_DIRECTORY, pb.DesiredState_DESIRED_STATE_ABSENT)
		action.Params = &pb.Action_Directory{Directory: &pb.DirectoryParams{Path: "/etc"}}
		result := e.ExecuteAction(ctx, testAction(action))
		assertFailed(t, result)
		if !strings.Contains(result.Error, "protected") {
			t.Errorf("expected 'protected' in error, got: %s", result.Error)
		}
	})

	t.Run("ProtectedDirDelete_Root", func(t *testing.T) {
		action := makeAction(t, pb.ActionType_ACTION_TYPE_DIRECTORY, pb.DesiredState_DESIRED_STATE_ABSENT)
		action.Params = &pb.Action_Directory{Directory: &pb.DirectoryParams{Path: "/"}}
		result := e.ExecuteAction(ctx, testAction(action))
		assertFailed(t, result)
		if !strings.Contains(result.Error, "protected") {
			t.Errorf("expected 'protected' in error, got: %s", result.Error)
		}
	})

	t.Run("ProtectedDirDelete_Usr", func(t *testing.T) {
		action := makeAction(t, pb.ActionType_ACTION_TYPE_DIRECTORY, pb.DesiredState_DESIRED_STATE_ABSENT)
		action.Params = &pb.Action_Directory{Directory: &pb.DirectoryParams{Path: "/usr"}}
		result := e.ExecuteAction(ctx, testAction(action))
		assertFailed(t, result)
		if !strings.Contains(result.Error, "protected") {
			t.Errorf("expected 'protected' in error, got: %s", result.Error)
		}
	})

	t.Run("EmptyDirectoryPath", func(t *testing.T) {
		action := makeAction(t, pb.ActionType_ACTION_TYPE_DIRECTORY, pb.DesiredState_DESIRED_STATE_PRESENT)
		action.Params = &pb.Action_Directory{Directory: &pb.DirectoryParams{Path: ""}}
		result := e.ExecuteAction(ctx, testAction(action))
		assertFailed(t, result)
	})
}

func TestIntegration_EdgeCase_DiskFull(t *testing.T) {
	skipIfNotPrivileged(t)
	e := newTestExecutor()
	ctx := context.Background()

	mountPoint := "/tmp/cadestro-diskfull-test"
	os.MkdirAll(mountPoint, 0755)

	cmd := exec.Command("sudo", "-n", "mount", "-t", "tmpfs", "-o", "size=1M", "tmpfs", mountPoint)
	if err := cmd.Run(); err != nil {
		t.Fatalf("mount tmpfs failed: %v", err)
	}
	t.Cleanup(func() {
		exec.Command("sudo", "-n", "umount", "-f", mountPoint).Run()
		sudoRemoveAll(mountPoint)
	})

	filler := filepath.Join(mountPoint, "filler")
	exec.Command("sudo", "-n", "sh", "-c", "dd if=/dev/zero of="+filler+" bs=1M count=1").Run()

	action := makeAction(t, pb.ActionType_ACTION_TYPE_FILE, pb.DesiredState_DESIRED_STATE_PRESENT)
	action.Params = &pb.Action_File{File: &pb.FileParams{
		Path:    filepath.Join(mountPoint, "testfile.txt"),
		Content: strings.Repeat("data", 1024),
	}}
	result := e.ExecuteAction(ctx, testAction(action))

	assertFailed(t, result)
}

func TestIntegration_EdgeCase_ReadOnlyMount(t *testing.T) {
	skipIfNotPrivileged(t)
	e := newTestExecutor()
	ctx := context.Background()

	sourceDir := "/tmp/cadestro-ro-source"
	mountPoint := "/tmp/cadestro-ro-test"
	os.MkdirAll(sourceDir, 0755)
	os.MkdirAll(mountPoint, 0755)

	cmd := exec.Command("sudo", "-n", "mount", "--bind", sourceDir, mountPoint)
	if err := cmd.Run(); err != nil {
		t.Fatalf("bind mount failed: %v", err)
	}

	cmd = exec.Command("sudo", "-n", "mount", "-o", "remount,ro,bind", mountPoint)
	if err := cmd.Run(); err != nil {
		exec.Command("sudo", "-n", "umount", mountPoint).Run()
		t.Fatalf("ro remount failed: %v", err)
	}

	t.Cleanup(func() {
		exec.Command("sudo", "-n", "umount", "-f", mountPoint).Run()
		sudoRemoveAll(mountPoint)
		sudoRemoveAll(sourceDir)
	})

	action := makeAction(t, pb.ActionType_ACTION_TYPE_FILE, pb.DesiredState_DESIRED_STATE_PRESENT)
	action.Params = &pb.Action_File{File: &pb.FileParams{
		Path:    filepath.Join(mountPoint, "testfile.txt"),
		Content: "should not be written",
	}}
	result := e.ExecuteAction(ctx, testAction(action))

	assertFailed(t, result)
}

func TestIntegration_EdgeCase_UserExistsDifferentShell(t *testing.T) {
	e := newTestExecutor()
	ctx := context.Background()

	username := "cadestroedgeuser"

	sudoRun("useradd", "-s", "/bin/bash", username).Run()
	t.Cleanup(func() { cleanupTestUser(t, username) })

	action := makeAction(t, pb.ActionType_ACTION_TYPE_USER, pb.DesiredState_DESIRED_STATE_PRESENT)
	action.Params = &pb.Action_User{User: &pb.UserParams{
		Username: username,
		Shell:    "/usr/sbin/nologin",
	}}
	result := e.ExecuteAction(ctx, testAction(action))
	assertSuccess(t, result)
	assertChanged(t, result, true)

	out, _ := exec.Command("getent", "passwd", username).CombinedOutput()
	if !strings.Contains(string(out), "/usr/sbin/nologin") {
		t.Errorf("shell not updated, getent says: %s", strings.TrimSpace(string(out)))
	}
}

func TestIntegration_EdgeCase_FileExistsDifferentPerms(t *testing.T) {
	e := newTestExecutor()
	ctx := context.Background()

	filePath := "/tmp/cadestro-edge-perms-test"
	os.WriteFile(filePath, []byte("original"), 0600)
	t.Cleanup(func() { sudoRemove(filePath) })

	action := makeAction(t, pb.ActionType_ACTION_TYPE_FILE, pb.DesiredState_DESIRED_STATE_PRESENT)
	action.Params = &pb.Action_File{File: &pb.FileParams{
		Path:    filePath,
		Content: "original",
		Mode:    "0644",
	}}
	result := e.ExecuteAction(ctx, testAction(action))
	assertSuccess(t, result)
	assertChanged(t, result, true)

	info, err := os.Stat(filePath)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0644 {
		t.Errorf("expected 0644, got %o", info.Mode().Perm())
	}
}

func TestIntegration_EdgeCase_FileExistsAsDirectory(t *testing.T) {
	e := newTestExecutor()
	ctx := context.Background()

	dirPath := "/tmp/cadestro-edge-type-conflict"
	os.MkdirAll(dirPath, 0755)
	t.Cleanup(func() { sudoRemoveAll(dirPath) })

	action := makeAction(t, pb.ActionType_ACTION_TYPE_FILE, pb.DesiredState_DESIRED_STATE_PRESENT)
	action.Params = &pb.Action_File{File: &pb.FileParams{
		Path:    dirPath,
		Content: "content",
	}}
	result := e.ExecuteAction(ctx, testAction(action))
	assertFailed(t, result)

	info, err := os.Stat(dirPath)
	if err != nil || !info.IsDir() {
		t.Fatalf("directory should be left intact, got info=%v err=%v", info, err)
	}
	entries, err := os.ReadDir(dirPath)
	if err != nil {
		t.Fatalf("read dir: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("directory should remain empty, found %d entries (a temp file moved inside?)", len(entries))
	}
}

func TestIntegration_EdgeCase_EmptyFileContent(t *testing.T) {
	e := newTestExecutor()
	ctx := context.Background()

	filePath := "/tmp/cadestro-edge-empty-file"
	t.Cleanup(func() { sudoRemove(filePath) })

	action := makeAction(t, pb.ActionType_ACTION_TYPE_FILE, pb.DesiredState_DESIRED_STATE_PRESENT)
	action.Params = &pb.Action_File{File: &pb.FileParams{
		Path:    filePath,
		Content: "",
	}}
	result := e.ExecuteAction(ctx, testAction(action))
	assertSuccess(t, result)

	info, err := os.Stat(filePath)
	if err != nil {
		t.Fatalf("file not created: %v", err)
	}
	if info.Size() != 0 {
		t.Errorf("expected empty file, got size %d", info.Size())
	}
}

func TestIntegration_EdgeCase_SymlinkCircular(t *testing.T) {
	e := newTestExecutor()
	ctx := context.Background()

	t.Run("DanglingSymlink", func(t *testing.T) {

		linkPath := "/tmp/cadestro-edge-dangling-link"
		os.Remove(linkPath)
		t.Cleanup(func() { os.Remove(linkPath) })

		if err := os.Symlink("/tmp/cadestro-edge-nonexistent-target", linkPath); err != nil {
			t.Fatal(err)
		}

		action := makeAction(t, pb.ActionType_ACTION_TYPE_FILE, pb.DesiredState_DESIRED_STATE_PRESENT)
		action.Params = &pb.Action_File{File: &pb.FileParams{
			Path:    linkPath,
			Content: "test content\n",
		}}
		result := e.ExecuteAction(ctx, testAction(action))

		if result.Status != pb.ExecutionStatus_EXECUTION_STATUS_SUCCESS &&
			result.Status != pb.ExecutionStatus_EXECUTION_STATUS_FAILED {
			t.Errorf("unexpected status: %s", result.Status)
		}
	})

	t.Run("CircularSymlink", func(t *testing.T) {

		linkA := "/tmp/cadestro-edge-circular-a"
		linkB := "/tmp/cadestro-edge-circular-b"
		os.Remove(linkA)
		os.Remove(linkB)
		t.Cleanup(func() {
			os.Remove(linkA)
			os.Remove(linkB)
		})

		os.Symlink(linkB, linkA)
		os.Symlink(linkA, linkB)

		action := makeAction(t, pb.ActionType_ACTION_TYPE_FILE, pb.DesiredState_DESIRED_STATE_PRESENT)
		action.Params = &pb.Action_File{File: &pb.FileParams{
			Path:    linkA,
			Content: "test content\n",
		}}
		result := e.ExecuteAction(ctx, testAction(action))

		if result.Status != pb.ExecutionStatus_EXECUTION_STATUS_SUCCESS &&
			result.Status != pb.ExecutionStatus_EXECUTION_STATUS_FAILED {
			t.Errorf("unexpected status: %s", result.Status)
		}
	})

	t.Run("SymlinkToProtectedPath", func(t *testing.T) {

		linkPath := "/tmp/cadestro-edge-symlink-protected"
		os.Remove(linkPath)
		t.Cleanup(func() { os.Remove(linkPath) })

		os.Symlink("/etc/passwd", linkPath)

		action := makeAction(t, pb.ActionType_ACTION_TYPE_FILE, pb.DesiredState_DESIRED_STATE_ABSENT)
		action.Params = &pb.Action_File{File: &pb.FileParams{Path: linkPath}}
		_ = e.ExecuteAction(ctx, testAction(action))

		if _, err := os.Stat("/etc/passwd"); err != nil {
			t.Fatal("CRITICAL: /etc/passwd was deleted!")
		}
	})
}

func TestIntegration_EdgeCase_DNSResolutionFailure(t *testing.T) {
	e := newTestExecutor()
	ctx := context.Background()

	t.Run("AppImage", func(t *testing.T) {
		action := makeAction(t, pb.ActionType_ACTION_TYPE_APP_IMAGE, pb.DesiredState_DESIRED_STATE_PRESENT)
		action.Params = &pb.Action_App{App: &pb.AppInstallParams{
			Url:         "https://this-domain-does-not-exist-xyzzy.invalid/app.AppImage",
			InstallPath: "/tmp/cadestro-edge-dns",

			ChecksumSha256: strings.Repeat("a", 64),
		}}
		result := e.ExecuteAction(ctx, testAction(action))
		assertFailed(t, result)
	})

	t.Run("Deb", func(t *testing.T) {
		skipIfNoApt(t)
		action := makeAction(t, pb.ActionType_ACTION_TYPE_DEB, pb.DesiredState_DESIRED_STATE_PRESENT)
		action.Params = &pb.Action_App{App: &pb.AppInstallParams{
			Url: "https://this-domain-does-not-exist-xyzzy.invalid/pkg.deb",
		}}
		result := e.ExecuteAction(ctx, testAction(action))
		assertFailed(t, result)
	})

	t.Run("Rpm", func(t *testing.T) {
		if _, err := exec.LookPath("rpm"); err != nil {
			t.Skip("rpm not found")
		}
		action := makeAction(t, pb.ActionType_ACTION_TYPE_RPM, pb.DesiredState_DESIRED_STATE_PRESENT)
		action.Params = &pb.Action_App{App: &pb.AppInstallParams{
			Url: "https://this-domain-does-not-exist-xyzzy.invalid/pkg.rpm",
		}}
		result := e.ExecuteAction(ctx, testAction(action))
		assertFailed(t, result)
	})
}

func TestIntegration_EdgeCase_HTTPSCertError(t *testing.T) {
	e := newTestExecutor()
	ctx := context.Background()

	e.httpClient = &http.Client{}

	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("fake appimage content"))
	}))
	t.Cleanup(ts.Close)

	action := makeAction(t, pb.ActionType_ACTION_TYPE_APP_IMAGE, pb.DesiredState_DESIRED_STATE_PRESENT)
	action.Params = &pb.Action_App{App: &pb.AppInstallParams{
		Url:         ts.URL + "/test.AppImage",
		InstallPath: "/tmp/cadestro-edge-tls",

		ChecksumSha256: strings.Repeat("a", 64),
	}}
	result := e.ExecuteAction(ctx, testAction(action))
	assertFailed(t, result)

	if _, err := os.Stat("/tmp/cadestro-edge-tls/test.AppImage"); err == nil {
		t.Error("partial file left behind after TLS error")
		os.RemoveAll("/tmp/cadestro-edge-tls")
	}
}

func TestIntegration_EdgeCase_PartialAppImage(t *testing.T) {
	e := newTestExecutor()
	ctx := context.Background()
	installDir := "/tmp/cadestro-edge-partial-appimage"
	fileName := "partial-app.AppImage"

	os.MkdirAll(installDir, 0755)
	t.Cleanup(func() { os.RemoveAll(installDir) })

	os.WriteFile(filepath.Join(installDir, fileName), []byte{}, 0755)

	realContent := []byte("#!/bin/sh\necho real\n")
	checksum := sha256.Sum256(realContent)
	checksumHex := hex.EncodeToString(checksum[:])
	ts := startFileServer(t, map[string][]byte{
		"/" + fileName: realContent,
	})

	action := makeAction(t, pb.ActionType_ACTION_TYPE_APP_IMAGE, pb.DesiredState_DESIRED_STATE_PRESENT)
	action.Params = &pb.Action_App{App: &pb.AppInstallParams{
		Url:            ts.URL + "/" + fileName,
		ChecksumSha256: checksumHex,
		InstallPath:    installDir,
	}}
	result := e.ExecuteAction(ctx, testAction(action))
	assertSuccess(t, result)
	assertChanged(t, result, true)

	data, err := os.ReadFile(filepath.Join(installDir, fileName))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != string(realContent) {
		t.Errorf("file content mismatch: got %d bytes, want %d", len(data), len(realContent))
	}
}

func TestIntegration_EdgeCase_ShellTimeout(t *testing.T) {
	e := newTestExecutor()
	ctx := context.Background()

	action := makeAction(t, pb.ActionType_ACTION_TYPE_SHELL, pb.DesiredState_DESIRED_STATE_PRESENT)
	action.Params = &pb.Action_Shell{Shell: &pb.ShellParams{
		Script:    "sleep 30",
		RunAsRoot: true,
	}}
	action.TimeoutSeconds = 2

	result := e.ExecuteAction(ctx, testAction(action))
	if result.Status != pb.ExecutionStatus_EXECUTION_STATUS_TIMEOUT {
		t.Errorf("expected TIMEOUT, got %s (error: %s)", result.Status, result.Error)
	}
}

func TestIntegration_EdgeCase_UserDeleteWhileLoggedIn(t *testing.T) {
	e := newTestExecutor()
	ctx := context.Background()
	username := "cadestroedgelogin"

	ensureTestUser(t, username)
	t.Cleanup(func() { cleanupTestUser(t, username) })

	bgCmd := exec.Command("sudo", "-n", "sh", "-c", fmt.Sprintf("su -s /bin/sh -c 'sleep 300 &' %s", username))
	bgCmd.Start()

	time.Sleep(200 * time.Millisecond)

	action := makeAction(t, pb.ActionType_ACTION_TYPE_USER, pb.DesiredState_DESIRED_STATE_ABSENT)
	action.Params = &pb.Action_User{User: &pb.UserParams{Username: username}}
	result := e.ExecuteAction(ctx, testAction(action))
	assertSuccess(t, result)
	assertChanged(t, result, true)

	if v, _ := userExists(context.Background(), username); v {
		t.Error("user still exists despite having active processes")
	}
}

func TestIntegration_EdgeCase_GroupIsPrimaryGroup(t *testing.T) {
	e := newTestExecutor()
	ctx := context.Background()
	username := "cadestroedgeprimgrp"
	groupName := username

	t.Cleanup(func() {
		cleanupTestUser(t, username)
		cleanupTestGroup(t, groupName)
	})

	ensureTestUser(t, username)

	action := makeAction(t, pb.ActionType_ACTION_TYPE_GROUP, pb.DesiredState_DESIRED_STATE_ABSENT)
	action.Params = &pb.Action_Group{Group: &pb.GroupParams{Name: groupName}}
	result := e.ExecuteAction(ctx, testAction(action))

	assertFailed(t, result)
}

func TestIntegration_EdgeCase_BinaryFileContent(t *testing.T) {
	e := newTestExecutor()
	ctx := context.Background()

	t.Run("NullBytes", func(t *testing.T) {
		filePath := "/tmp/cadestro-edge-binary-null"
		t.Cleanup(func() { sudoRemove(filePath) })

		content := "before\x00middle\x00after\n"

		action := makeAction(t, pb.ActionType_ACTION_TYPE_FILE, pb.DesiredState_DESIRED_STATE_PRESENT)
		action.Params = &pb.Action_File{File: &pb.FileParams{
			Path:    filePath,
			Content: content,
		}}
		result := e.ExecuteAction(ctx, testAction(action))
		assertSuccess(t, result)

		data, err := os.ReadFile(filePath)
		if err != nil {
			t.Fatal(err)
		}
		if string(data) != content {
			t.Errorf("binary content mismatch: got %d bytes, want %d", len(data), len(content))
		}
	})

	t.Run("UTF8Multibyte", func(t *testing.T) {
		filePath := "/tmp/cadestro-edge-utf8"
		t.Cleanup(func() { sudoRemove(filePath) })

		content := "日本語テスト 🎉 Ünïcödé\n"

		action := makeAction(t, pb.ActionType_ACTION_TYPE_FILE, pb.DesiredState_DESIRED_STATE_PRESENT)
		action.Params = &pb.Action_File{File: &pb.FileParams{
			Path:    filePath,
			Content: content,
		}}
		result := e.ExecuteAction(ctx, testAction(action))
		assertSuccess(t, result)

		data, err := os.ReadFile(filePath)
		if err != nil {
			t.Fatal(err)
		}
		if string(data) != content {
			t.Error("UTF-8 content mismatch")
		}
	})

	t.Run("LargeContent", func(t *testing.T) {
		filePath := "/tmp/cadestro-edge-large-content"
		t.Cleanup(func() { sudoRemove(filePath) })

		content := strings.Repeat("A", 1024*1024) + "\n"

		action := makeAction(t, pb.ActionType_ACTION_TYPE_FILE, pb.DesiredState_DESIRED_STATE_PRESENT)
		action.Params = &pb.Action_File{File: &pb.FileParams{
			Path:    filePath,
			Content: content,
		}}
		result := e.ExecuteAction(ctx, testAction(action))
		assertSuccess(t, result)

		info, err := os.Stat(filePath)
		if err != nil {
			t.Fatal(err)
		}
		if info.Size() != int64(len(content)) {
			t.Errorf("expected %d bytes, got %d", len(content), info.Size())
		}
	})
}

func TestIntegration_EdgeCase_ImmutableFile(t *testing.T) {
	skipIfNotPrivileged(t)
	e := newTestExecutor()
	ctx := context.Background()
	filePath := "/tmp/cadestro-edge-immutable"

	t.Cleanup(func() {

		sudoRun("chattr", "-i", filePath).Run()
		sudoRemove(filePath)
	})

	os.WriteFile(filePath, []byte("original\n"), 0644)
	if out, err := sudoRun("chattr", "+i", filePath).CombinedOutput(); err != nil {
		t.Skipf("chattr not available or not supported: %v: %s", err, out)
	}

	action := makeAction(t, pb.ActionType_ACTION_TYPE_FILE, pb.DesiredState_DESIRED_STATE_PRESENT)
	action.Params = &pb.Action_File{File: &pb.FileParams{
		Path:    filePath,
		Content: "modified\n",
	}}
	result := e.ExecuteAction(ctx, testAction(action))

	assertFailed(t, result)

	data, _ := os.ReadFile(filePath)
	if string(data) != "original\n" {
		t.Error("immutable file was modified!")
	}
}

func TestIntegration_EdgeCase_BrokenSudoersFile(t *testing.T) {
	e := newTestExecutor()
	ctx := context.Background()

	actionID := "edgebroken01"

	brokenPath := "/etc/sudoers.d/99-cadestro-broken-test"
	t.Cleanup(func() {
		sudoRemove(brokenPath)
		sudoRemove(sudoersFilePath(actionID))
		sudoRun("groupdel", sanitizeSudoGroupName(actionID)).Run()
		cleanupTestUser(t, "cadestroedgesudo")
	})

	sudoWriteFile(brokenPath, []byte("INVALID SUDOERS SYNTAX !!!\n"))

	ensureTestUser(t, "cadestroedgesudo")

	action := &pb.Action{
		Id:           &pb.ActionId{Value: actionID},
		Type:         pb.ActionType_ACTION_TYPE_ADMIN_POLICY,
		DesiredState: pb.DesiredState_DESIRED_STATE_PRESENT,
		Params: &pb.Action_AdminPolicy{AdminPolicy: &pb.AdminPolicyParams{
			AccessLevel: pb.AdminAccessLevel_ADMIN_ACCESS_LEVEL_FULL,
			Users:       []string{"cadestroedgesudo"},
		}},
	}
	result := e.ExecuteAction(ctx, testAction(action))

	assertSuccess(t, result)

	ourFile := sudoersFilePath(actionID)
	cmd := sudoRun("visudo", "-c", "-f", ourFile)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Errorf("our sudoers file is invalid: %v: %s", err, out)
	}
}

func TestIntegration_EdgeCase_SSHDirWrongPermissions(t *testing.T) {
	e := newTestExecutor()
	ctx := context.Background()
	username := "cadestroedgesshperm"

	t.Cleanup(func() { cleanupTestUser(t, username) })
	ensureTestUser(t, username)

	homeDir := filepath.Join("/home", username)
	sshDir := filepath.Join(homeDir, ".ssh")
	authKeys := filepath.Join(sshDir, "authorized_keys")

	sudoRun("mkdir", "-p", sshDir).Run()
	sudoRun("chmod", "0777", sshDir).Run()

	sudoRun("sh", "-c", fmt.Sprintf("echo 'ssh-rsa OLD_KEY' > %s", authKeys)).Run()
	sudoRun("chmod", "0666", authKeys).Run()

	action := makeAction(t, pb.ActionType_ACTION_TYPE_USER, pb.DesiredState_DESIRED_STATE_PRESENT)
	action.Params = &pb.Action_User{User: &pb.UserParams{
		Username:          username,
		SshAuthorizedKeys: []string{"ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABgQ test@test"},
	}}
	result := e.ExecuteAction(ctx, testAction(action))
	assertSuccess(t, result)

	out, err := sudoRun("sh", "-c", fmt.Sprintf("stat -c '%%a' %s", sshDir)).Output()
	if err != nil {
		t.Fatalf("cannot stat .ssh: %v", err)
	}
	if perm := strings.TrimSpace(string(out)); perm != "700" {
		t.Errorf("expected .ssh permissions 700, got %s", perm)
	}

	out, err = sudoRun("sh", "-c", fmt.Sprintf("stat -c '%%a' %s", authKeys)).Output()
	if err != nil {
		t.Fatalf("cannot stat authorized_keys: %v", err)
	}
	if perm := strings.TrimSpace(string(out)); perm != "600" {
		t.Errorf("expected authorized_keys permissions 600, got %s", perm)
	}
}

func TestIntegration_EdgeCase_VeryLongFilePath(t *testing.T) {
	e := newTestExecutor()
	ctx := context.Background()

	baseDir := "/tmp/cadestro-edge-longpath"
	t.Cleanup(func() { os.RemoveAll(baseDir) })

	longPath := baseDir
	for len(longPath) < 3900 {
		longPath = filepath.Join(longPath, "abcdefghij")
	}
	longPath = filepath.Join(longPath, "file.txt")

	action := makeAction(t, pb.ActionType_ACTION_TYPE_FILE, pb.DesiredState_DESIRED_STATE_PRESENT)
	action.Params = &pb.Action_File{File: &pb.FileParams{
		Path:    longPath,
		Content: "test\n",
	}}
	result := e.ExecuteAction(ctx, testAction(action))

	if result.Status != pb.ExecutionStatus_EXECUTION_STATUS_SUCCESS &&
		result.Status != pb.ExecutionStatus_EXECUTION_STATUS_FAILED {
		t.Errorf("unexpected status: %s", result.Status)
	}

	t.Run("ExceedsPathMax", func(t *testing.T) {

		tooLong := baseDir
		for len(tooLong) < 4200 {
			tooLong = filepath.Join(tooLong, "abcdefghij")
		}
		tooLong = filepath.Join(tooLong, "file.txt")

		action := makeAction(t, pb.ActionType_ACTION_TYPE_FILE, pb.DesiredState_DESIRED_STATE_PRESENT)
		action.Params = &pb.Action_File{File: &pb.FileParams{
			Path:    tooLong,
			Content: "test\n",
		}}
		result := e.ExecuteAction(ctx, testAction(action))

		assertFailed(t, result)
	})
}

func TestIntegration_EdgeCase_PackagePinConflict(t *testing.T) {
	skipIfNoApt(t)
	e := newTestExecutor()
	ctx := context.Background()

	sudoRun("apt-get", "install", "-y", "sl").Run()
	t.Cleanup(func() {
		sudoRun("apt-get", "remove", "-y", "sl").Run()
	})

	action := makeAction(t, pb.ActionType_ACTION_TYPE_PACKAGE, pb.DesiredState_DESIRED_STATE_PRESENT)
	action.Params = &pb.Action_Package{Package: &pb.PackageParams{
		Name: "sl",
		Pin:  true,
	}}
	result := e.ExecuteAction(ctx, testAction(action))
	assertSuccess(t, result)

	action2 := makeAction(t, pb.ActionType_ACTION_TYPE_PACKAGE, pb.DesiredState_DESIRED_STATE_PRESENT)
	action2.Params = &pb.Action_Package{Package: &pb.PackageParams{
		Name: "sl",
		Pin:  true,
	}}
	result2 := e.ExecuteAction(ctx, testAction(action2))
	assertSuccess(t, result2)
	assertChanged(t, result2, false)
}

func TestIntegration_EdgeCase_SystemdInvalidUnit(t *testing.T) {
	e := newTestExecutor()
	ctx := context.Background()

	unitName := "cadestro-edge-invalid.service"
	unitPath := "/etc/systemd/system/" + unitName
	t.Cleanup(func() { sudoRemove(unitPath) })

	t.Run("InvalidSyntax", func(t *testing.T) {

		action := makeAction(t, pb.ActionType_ACTION_TYPE_SERVICE, pb.DesiredState_DESIRED_STATE_PRESENT)
		action.Params = &pb.Action_Service{Service: &pb.ServiceParams{
			UnitName:    unitName,
			UnitContent: "THIS IS NOT VALID SYSTEMD UNIT CONTENT\n[[[invalid\n",
		}}
		result := e.ExecuteAction(ctx, testAction(action))

		assertFailed(t, result)

		if _, err := os.Stat(unitPath); err != nil {
			t.Error("unit file not written")
		}
	})

	t.Run("EmptyUnitContent", func(t *testing.T) {

		action := makeAction(t, pb.ActionType_ACTION_TYPE_SERVICE, pb.DesiredState_DESIRED_STATE_PRESENT)
		action.Params = &pb.Action_Service{Service: &pb.ServiceParams{
			UnitName:    unitName,
			UnitContent: "",
		}}
		result := e.ExecuteAction(ctx, testAction(action))

		if result.Status != pb.ExecutionStatus_EXECUTION_STATUS_SUCCESS &&
			result.Status != pb.ExecutionStatus_EXECUTION_STATUS_FAILED {
			t.Errorf("unexpected status: %s", result.Status)
		}
	})
}

func TestIntegration_EdgeCase_ConcurrentFileWrites(t *testing.T) {
	e := newTestExecutor()
	filePath := "/tmp/cadestro-edge-concurrent"
	t.Cleanup(func() { sudoRemove(filePath) })

	var wg sync.WaitGroup
	errors := make(chan string, 10)

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			ctx := context.Background()
			content := fmt.Sprintf("content from goroutine %d\n", idx)
			action := makeAction(t, pb.ActionType_ACTION_TYPE_FILE, pb.DesiredState_DESIRED_STATE_PRESENT)
			action.Params = &pb.Action_File{File: &pb.FileParams{
				Path:    filePath,
				Content: content,
			}}
			result := e.ExecuteAction(ctx, testAction(action))
			if result.Status != pb.ExecutionStatus_EXECUTION_STATUS_SUCCESS {
				errors <- fmt.Sprintf("goroutine %d failed: %s", idx, result.Error)
			}
		}(i)
	}

	wg.Wait()
	close(errors)

	var errs []string
	for e := range errors {
		errs = append(errs, e)
	}

	successCount := 10 - len(errs)
	if successCount == 0 {
		t.Error("all concurrent writes failed — expected at least one to succeed")
	}

	data, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("file not readable after concurrent writes: %v", err)
	}

	content := string(data)
	if !strings.HasPrefix(content, "content from goroutine ") {
		t.Errorf("file content corrupt after concurrent writes: %q", content)
	}
}

func TestIntegration_EdgeCase_LargeShellOutput(t *testing.T) {
	e := newTestExecutor()
	ctx := context.Background()

	t.Run("LargeStdout", func(t *testing.T) {

		action := makeAction(t, pb.ActionType_ACTION_TYPE_SHELL, pb.DesiredState_DESIRED_STATE_PRESENT)
		action.Params = &pb.Action_Shell{Shell: &pb.ShellParams{
			Script:    "dd if=/dev/zero bs=1024 count=2048 | tr '\\0' 'A'",
			RunAsRoot: true,
		}}
		action.TimeoutSeconds = 30
		result := e.ExecuteAction(ctx, testAction(action))
		assertSuccess(t, result)
	})

	t.Run("LargeStderr", func(t *testing.T) {

		action := makeAction(t, pb.ActionType_ACTION_TYPE_SHELL, pb.DesiredState_DESIRED_STATE_PRESENT)
		action.Params = &pb.Action_Shell{Shell: &pb.ShellParams{
			Script:    "dd if=/dev/zero bs=1024 count=1024 | tr '\\0' 'E' >&2",
			RunAsRoot: true,
		}}
		action.TimeoutSeconds = 30
		result := e.ExecuteAction(ctx, testAction(action))
		assertSuccess(t, result)
	})

	t.Run("InterleavedOutput", func(t *testing.T) {

		action := makeAction(t, pb.ActionType_ACTION_TYPE_SHELL, pb.DesiredState_DESIRED_STATE_PRESENT)
		action.Params = &pb.Action_Shell{Shell: &pb.ShellParams{
			Script:    `for i in $(seq 1 1000); do echo "stdout line $i"; echo "stderr line $i" >&2; done`,
			RunAsRoot: true,
		}}
		action.TimeoutSeconds = 30
		result := e.ExecuteAction(ctx, testAction(action))
		assertSuccess(t, result)
	})
}

func TestIntegration_EdgeCase_RepositoryExpiredGPGKey(t *testing.T) {
	skipIfNoApt(t)
	e := newTestExecutor()
	ctx := context.Background()

	repoName := "cadestroedgeexpiredgpg"
	t.Cleanup(func() {
		sudoRemove(fmt.Sprintf("/etc/apt/sources.list.d/%s.sources", repoName))
		sudoRemove(fmt.Sprintf("/etc/apt/keyrings/%s.gpg", repoName))
	})

	action := makeAction(t, pb.ActionType_ACTION_TYPE_REPOSITORY, pb.DesiredState_DESIRED_STATE_PRESENT)
	action.Params = &pb.Action_Repository{Repository: &pb.RepositoryParams{
		Name: repoName,
		Apt: &pb.AptRepository{
			Url:          "https://example.com/apt",
			Distribution: "bookworm",
			Components:   []string{"main"},
			GpgKeyUrl:    "https://this-domain-does-not-exist-xyzzy.invalid/key.gpg",
		},
	}}
	result := e.ExecuteAction(ctx, testAction(action))

	assertFailed(t, result)
}
