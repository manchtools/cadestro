package executor

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	pb "github.com/manchtools/cadestro/contract/gen/go/cadestro/v1"
	sysexec "github.com/manchtools/cadestro/sdk/sys/exec"
	sysuser "github.com/manchtools/cadestro/sdk/sys/user"

	"github.com/manchtools/cadestro/agent/internal/store"
)

// These tests pin the ordering invariant: never rotate a credential that
// cannot first be returned to control for the operator.

// lpsRecorder observes both sides of the rotation in one ordered log, so a test
// can assert not just that the password was reported and set, but that the
// report came FIRST.
type lpsRecorder struct {
	mu        sync.Mutex
	events    []string
	reported  []*pb.LpsPasswordRotation
	setCalls  []string // revealed plaintexts, in call order
	storeErr  error
	actionIDs []string
}

func (r *lpsRecorder) StorePasswords(_ context.Context, actionID string, rotations []*pb.LpsPasswordRotation) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, "report")
	r.actionIDs = append(r.actionIDs, actionID)
	if r.storeErr != nil {
		return r.storeErr
	}
	r.reported = append(r.reported, rotations...)
	return nil
}

// lpsRecorderUser is the sysuser fake, writing into the same log. Every
// unlisted method panics via the embedded nil interface.
type lpsRecorderUser struct {
	sysuser.Manager
	rec *lpsRecorder
}

func (f *lpsRecorderUser) Exists(context.Context, string) (bool, error) { return true, nil }
func (f *lpsRecorderUser) SetPassword(_ context.Context, _ string, pw sysexec.Secret) error {
	f.rec.mu.Lock()
	defer f.rec.mu.Unlock()
	f.rec.events = append(f.rec.events, "set")
	f.rec.setCalls = append(f.rec.setCalls, pw.Reveal())
	return nil
}
func (f *lpsRecorderUser) KillSessions(context.Context, string) error { return nil }

// newLpsExecutor wires an executor with a store and the recorder installed on
// both the user manager and the password store.
func newLpsExecutor(t *testing.T, rec *lpsRecorder, wireStore bool) *Executor {
	t.Helper()
	e := NewExecutor(nil)
	s, err := store.New(t.TempDir())
	if err != nil {
		t.Fatalf("store: %v", err)
	}
	e.SetStore(s)
	if wireStore {
		e.SetLpsPasswordStore(rec)
	}

	e.deps.user = &lpsRecorderUser{rec: rec}
	e.deps.notify = noopNotify{}
	return e
}

type noopNotify struct{}

func (noopNotify) NotifyAll(context.Context, string, string) error             { return nil }
func (noopNotify) NotifyUsers(context.Context, []string, string, string) error { return nil }

func runLps(t *testing.T, e *Executor, actionID string) (bool, map[string]string, error) {
	t.Helper()
	// Cancel the ctx so the 60s post-rotation grace returns immediately.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, changed, metadata, err := e.executeLps(ctx, &pb.LpsParams{
		Usernames:            []string{"alice"},
		PasswordLength:       20,
		RotationIntervalDays: 30,
	}, pb.DesiredState_DESIRED_STATE_PRESENT, actionID)
	return changed, metadata, err
}

// With no route to control the action fails BEFORE any account is touched: a
// disconnected agent cannot safely rotate a password it could not return.
func TestExecuteLps_NotConnectedFailsClosedBeforeRotation(t *testing.T) {
	rec := &lpsRecorder{}
	e := newLpsExecutor(t, rec, false) // no password store wired

	_, _, err := runLps(t, e, "01HKACTION0000000000000000")
	if err == nil {
		t.Fatal("executeLps without a connection to the server must fail")
	}
	if !strings.Contains(err.Error(), "connection to the server") {
		t.Errorf("expected a not-connected error, got: %v", err)
	}
	if len(rec.setCalls) != 0 {
		t.Errorf("SetPassword was called %d times while disconnected — must not rotate a password it cannot report", len(rec.setCalls))
	}
}

// The ordering itself, which is the whole point: the password reaches control
// BEFORE it is applied locally. A test that only checked "both happened" would
// pass on the reverse order, which is the order that strands a credential.
func TestExecuteLps_ReportsBeforeSettingThePassword(t *testing.T) {
	rec := &lpsRecorder{}
	e := newLpsExecutor(t, rec, true)
	const actionID = "01HKACTION0000000000000000"

	changed, metadata, err := runLps(t, e, actionID)
	if err != nil {
		t.Fatalf("executeLps: %v", err)
	}
	if !changed {
		t.Fatal("expected the action to report a change")
	}
	if len(rec.setCalls) != 1 {
		t.Fatalf("expected exactly one SetPassword, got %d", len(rec.setCalls))
	}
	if len(rec.reported) != 1 {
		t.Fatalf("expected exactly one reported rotation, got %d", len(rec.reported))
	}

	want := []string{"report", "set"}
	if len(rec.events) != 2 || rec.events[0] != want[0] || rec.events[1] != want[1] {
		t.Fatalf("wrong order: got %v, want %v — a password set before it is reported is one the operator can lose", rec.events, want)
	}

	if string(rec.reported[0].GetPassword()) != rec.setCalls[0] {
		t.Error("the reported password is not the one set on the account")
	}
	if rec.reported[0].GetUsername() != "alice" {
		t.Errorf("reported username = %q, want alice", rec.reported[0].GetUsername())
	}
	if rec.reported[0].GetReason() == pb.RotationReason_ROTATION_REASON_UNSPECIFIED {
		t.Error("reported rotation carries no reason; control stores UNSPECIFIED")
	}
	if len(rec.actionIDs) != 1 || rec.actionIDs[0] != actionID {
		t.Errorf("reported under action %v, want %q", rec.actionIDs, actionID)
	}

	// The action result must carry no password. The credential travels only as
	// a dedicated authenticated stream field.
	for k, v := range metadata {
		if strings.Contains(v, rec.setCalls[0]) {
			t.Errorf("action metadata %q leaks the rotated password", k)
		}
	}
	if metadata["lps.rotations"] != "" {
		t.Errorf("lps.rotations metadata is still emitted (%q); passwords belong only in authenticated stream fields",
			metadata["lps.rotations"])
	}
}

// A report that control REJECTS must leave the account alone. This is the case
// the old suite could not express: sealing failed only on local misconfiguration,
// whereas a rejected report is a routine server-side outcome.
func TestExecuteLps_ReportRejectedLeavesPasswordUnchanged(t *testing.T) {
	rec := &lpsRecorder{storeErr: errors.New("control refused the rotation")}
	e := newLpsExecutor(t, rec, true)

	changed, _, err := runLps(t, e, "01HKACTION0000000000000000")
	if err == nil {
		t.Fatal("a rejected report must surface as an action error")
	}
	if changed {
		t.Error("action reported a change although no password was rotated")
	}
	if len(rec.setCalls) != 0 {
		t.Errorf("SetPassword ran %d times after the report was rejected — the account now holds a credential control never received", len(rec.setCalls))
	}
}
