package executor

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"testing"
	"time"

	pb "github.com/manchtools/cadestro/contract/gen/go/cadestro/v1"
	"github.com/manchtools/cadestro/sdk/pkg"
	sysexec "github.com/manchtools/cadestro/sdk/sys/exec"
)

type upgradeFakeMgr struct {
	pkg.Manager
	backend            pkg.Backend
	hasUpdates         bool
	hasSecurityUpdates bool
	normalUpgradeErr   error
	securityUpgradeErr error
	normalUpgraded     bool
	securityUpgraded   bool
}

func (f *upgradeFakeMgr) Backend() pkg.Backend { return f.backend }
func (f *upgradeFakeMgr) Update(_ context.Context) (sysexec.Result, error) {
	return sysexec.Result{Stdout: "index refreshed"}, nil
}
func (f *upgradeFakeMgr) HasUpdates(_ context.Context) (bool, error) {
	return f.hasUpdates, nil
}
func (f *upgradeFakeMgr) HasSecurityUpdates(_ context.Context) (bool, error) {
	return f.hasSecurityUpdates, nil
}
func (f *upgradeFakeMgr) UpgradeAll(_ context.Context) (sysexec.Result, error) {
	if f.normalUpgradeErr != nil {
		return sysexec.Result{}, f.normalUpgradeErr
	}
	f.normalUpgraded = true
	return sysexec.Result{Stdout: "upgraded"}, nil
}
func (f *upgradeFakeMgr) UpgradeSecurity(_ context.Context) (sysexec.Result, error) {
	if f.securityUpgradeErr != nil {
		return sysexec.Result{}, f.securityUpgradeErr
	}
	f.securityUpgraded = true
	return sysexec.Result{Stdout: "upgraded"}, nil
}

func updateTestExecutor(t *testing.T, fake *upgradeFakeMgr) *Executor {
	t.Helper()
	t.Setenv("PATH", t.TempDir())
	return &Executor{
		logger:     slog.Default(),
		now:        time.Now,
		pkgBackend: fake.backend,
		pkgManager: fake,
		repairFS:   func(context.Context) bool { return true },
	}
}

func TestExecuteUpdate_SecurityOnlyUnsupported_NotApplicable(t *testing.T) {
	fake := &upgradeFakeMgr{backend: pkg.Pacman, securityUpgradeErr: pkg.ErrUnsupported}
	e := updateTestExecutor(t, fake)

	_, changed, err := e.executeUpdate(context.Background(), &pb.UpdateParams{SecurityOnly: true})

	if !errors.Is(err, errNotApplicable) {
		t.Fatalf("expected errNotApplicable, got: %v", err)
	}
	if !strings.Contains(err.Error(), "security-only") {
		t.Errorf("reason must name the security-only limitation, got: %v", err)
	}
	if changed {
		t.Error("expected changed=false for a not-applicable security-only update")
	}
	if fake.securityUpgraded || fake.normalUpgraded {
		t.Error("fail-closed violated: an upgrade ran despite security-only being unsupported")
	}
}

func TestExecuteUpdate_SecurityOnlyToolingMissing_NotApplicable(t *testing.T) {
	fake := &upgradeFakeMgr{
		backend:            pkg.Apt,
		hasUpdates:         true,
		securityUpgradeErr: fmt.Errorf("apt security upgrade: %w", sysexec.ErrBackendUnavailable),
	}
	e := updateTestExecutor(t, fake)

	_, changed, err := e.executeUpdate(context.Background(), &pb.UpdateParams{SecurityOnly: true})

	if !errors.Is(err, errNotApplicable) {
		t.Fatalf("expected errNotApplicable, got: %v", err)
	}
	if changed {
		t.Error("expected changed=false: nothing was upgraded even though updates were available")
	}
}

func TestExecuteUpdate_SecurityOnlyFalse_BackendErrorStaysFailed(t *testing.T) {
	fake := &upgradeFakeMgr{
		backend:          pkg.Apt,
		normalUpgradeErr: fmt.Errorf("apt-get vanished mid-flight: %w", sysexec.ErrBackendUnavailable),
	}
	e := updateTestExecutor(t, fake)

	_, _, err := e.executeUpdate(context.Background(), &pb.UpdateParams{SecurityOnly: false})

	if err == nil {
		t.Fatal("expected an error")
	}
	if errors.Is(err, errNotApplicable) {
		t.Fatalf("a backend failure on a normal update must stay FAILED, got not-applicable: %v", err)
	}
}

func TestExecuteUpdate_SecurityOnlySupported_Proceeds(t *testing.T) {
	fake := &upgradeFakeMgr{backend: pkg.Dnf, hasSecurityUpdates: true}
	e := updateTestExecutor(t, fake)

	_, changed, err := e.executeUpdate(context.Background(), &pb.UpdateParams{SecurityOnly: true})

	if err != nil {
		t.Fatalf("expected success, got: %v", err)
	}
	if !fake.securityUpgraded {
		t.Error("expected UpgradeSecurity to run on a capable backend")
	}
	if !changed {
		t.Error("expected changed=true when updates were applied")
	}
}

func TestSecurityOnlyNotApplicable_Decision(t *testing.T) {
	sentinel := pkg.ErrUnsupported
	wrapped := fmt.Errorf("apt security upgrade: %w", sysexec.ErrBackendUnavailable)
	rebootFail := errors.New("schedule reboot: shutdown refused")

	cases := []struct {
		name         string
		securityOnly bool
		upgradeErr   error
		lastErr      error
		want         bool
	}{
		{"sentinel alone → NA", true, sentinel, sentinel, true},
		{"wrapped backend-unavailable alone → NA", true, wrapped, wrapped, true},
		{"reboot failure joined after sentinel → stays FAILED", true, sentinel, errors.Join(sentinel, rebootFail), false},
		{"not security-only → never NA", false, sentinel, sentinel, false},
		{"no upgrade error → never NA", true, nil, nil, false},
		{"unrelated upgrade error → stays FAILED", true, errors.New("dpkg exploded"), errors.New("dpkg exploded"), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := securityOnlyNotApplicable(tc.securityOnly, tc.upgradeErr, tc.lastErr); got != tc.want {
				t.Errorf("securityOnlyNotApplicable(%v, %v, %v) = %v, want %v",
					tc.securityOnly, tc.upgradeErr, tc.lastErr, got, tc.want)
			}
		})
	}
}

func TestExecuteAction_SecurityOnly_NotApplicableStatus(t *testing.T) {
	fake := &upgradeFakeMgr{backend: pkg.Pacman, securityUpgradeErr: pkg.ErrUnsupported}
	e := updateTestExecutor(t, fake)

	action := &pb.Action{
		Id:     &pb.ActionId{Value: "01JZTESTNOTAPPLICABLE0000A"},
		Type:   pb.ActionType_ACTION_TYPE_UPDATE,
		Params: &pb.Action_Update{Update: &pb.UpdateParams{SecurityOnly: true}},
	}
	result := e.ExecuteAction(context.Background(), action)

	if result.Status != pb.ExecutionStatus_EXECUTION_STATUS_NOT_APPLICABLE {
		t.Fatalf("expected NOT_APPLICABLE, got %s (error: %s)", result.Status, result.Error)
	}
	if !strings.Contains(result.Error, "security-only") {
		t.Errorf("result error must carry the reason, got: %q", result.Error)
	}
	if result.Changed {
		t.Error("expected Changed=false on a not-applicable result")
	}
}
