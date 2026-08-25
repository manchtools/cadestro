package executor

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	pb "github.com/manchtools/cadestro/contract/gen/go/cadestro/v1"
	sysexec "github.com/manchtools/cadestro/sdk/sys/exec"
)

type failingBaseRunner struct{}

func (failingBaseRunner) Run(_ context.Context, _ sysexec.Command) (sysexec.Result, error) {
	return sysexec.Result{}, errors.New("no such file or directory")
}

func (failingBaseRunner) Stream(_ context.Context, _ sysexec.Command, _ sysexec.OutputCallback) (sysexec.Result, error) {
	return sysexec.Result{}, errors.New("no such file or directory")
}

func (failingBaseRunner) Backend() sysexec.PrivilegeBackend { return sysexec.Direct }

func TestComplianceShellWithoutDetectionScriptFailsClosed(t *testing.T) {
	prev := executorRunner
	t.Cleanup(func() { executorRunner = prev })
	rec := &recordingBaseRunner{}
	executorRunner = rec

	e := NewExecutor(nil)
	e.runner = rec
	execOut, detectionOut, changed, err := e.executeShell(context.Background(), &pb.ShellParams{
		IsCompliance:    true,
		Script:          "touch /tmp/cadestro-compliance-remediation-must-never-run",
		DetectionScript: "",
		RunAsRoot:       true,
	})

	assert.Empty(t, rec.cmds,
		"the compliance path must never dispatch a script when detection is empty")
	require.Error(t, err, "a compliance action without a detection script must fail closed")
	assert.Nil(t, execOut)
	assert.Nil(t, detectionOut)
	assert.False(t, changed)
}

func TestComplianceShellRunsDetectionOnly(t *testing.T) {
	prev := executorRunner
	t.Cleanup(func() { executorRunner = prev })
	rec := &recordingBaseRunner{}
	executorRunner = rec

	const (
		detection   = "test -f /etc/cadestro-compliance-probe"
		remediation = "touch /tmp/cadestro-compliance-remediation-must-never-run"
	)
	e := NewExecutor(nil)
	e.runner = rec
	execOut, detectionOut, changed, err := e.executeShell(context.Background(), &pb.ShellParams{
		IsCompliance:    true,
		Script:          remediation,
		DetectionScript: detection,
		RunAsRoot:       true,
	})

	require.NoError(t, err)
	require.Len(t, rec.cmds, 1, "detection runs exactly once and nothing else is dispatched")
	joined := strings.Join(rec.cmds[0].Args, " ")
	assert.Contains(t, joined, detection)
	assert.NotContains(t, joined, remediation,
		"the execution script is never run by the compliance path")
	assert.NotNil(t, detectionOut, "compliance reports its detection findings")
	assert.Nil(t, execOut)
	assert.False(t, changed)
}

func TestComplianceShellReportsNotCompliantWhenDetectionCannotRun(t *testing.T) {
	prev := executorRunner
	t.Cleanup(func() { executorRunner = prev })
	executorRunner = failingBaseRunner{}

	e := NewExecutor(nil)
	e.runner = failingBaseRunner{}
	result := e.ExecuteAction(context.Background(), &pb.Action{
		Id:   &pb.ActionId{Value: "01J0000000000000000000000A"},
		Type: pb.ActionType_ACTION_TYPE_SHELL,
		Params: &pb.Action_Shell{Shell: &pb.ShellParams{
			IsCompliance:    true,
			DetectionScript: "test -f /etc/cadestro-compliance-probe",
			RunAsRoot:       true,
		}},
	})

	assert.False(t, result.Compliant, "a detection script that never ran proves nothing")
	assert.Equal(t, pb.ExecutionStatus_EXECUTION_STATUS_FAILED, result.Status)
	assert.NotEmpty(t, result.Error)
}
