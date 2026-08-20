// Package executor provides implementations for action executors.
package executor

import (
	"context"
	"fmt"

	pb "github.com/manchtools/cadestro/contract/gen/go/cadestro/v1"
	sysreboot "github.com/manchtools/cadestro/sdk/sys/reboot"
)

// executeReboot schedules a system reboot in 5 minutes.
func (e *Executor) executeReboot(ctx context.Context) (*pb.CommandOutput, error) {
	e.ensureDeps()
	// Fail closed when this executor has no privilege runner (the
	// NewExecutor(nil) unit-test convention). Without this, a test that
	// dispatches a REBOOT action through a real handler+executor would
	// fall back to the process-global Direct runner and issue a REAL
	// `shutdown -r +5` on the developer's machine — which is exactly what
	// happened (systemd-logind grants reboot to an active desktop session via
	// polkit, no sudo needed). A reboot with no privilege backend can never
	// succeed anyway, so refusing is both safe and correct; production always
	// passes a runner, so this never trips at runtime.
	if e.runner == nil {
		return nil, fmt.Errorf("no privilege runner configured; refusing to schedule reboot")
	}

	// Best-effort heads-up to logged-in users; never block the reboot on it.
	e.notifyAll(ctx, "System Reboot", "This system will reboot in 5 minutes. Please save your work.")

	rb, err := sysreboot.New(e.runner)
	if err != nil {
		return nil, fmt.Errorf("failed to build reboot manager: %w", err)
	}
	if err := rb.Schedule(ctx, sysreboot.ScheduleOptions{Delay: "+5", Message: "Cadestro: scheduled reboot"}); err != nil {
		return nil, fmt.Errorf("failed to schedule reboot: %w", err)
	}
	return &pb.CommandOutput{Stdout: "Reboot scheduled in 5 minutes\n"}, nil
}

// Reboot schedules the same safe delayed reboot used by the agent runtime.
func (e *Executor) Reboot(ctx context.Context) error {
	_, err := e.executeReboot(ctx)
	return err
}
