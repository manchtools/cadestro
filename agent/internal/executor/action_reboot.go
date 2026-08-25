package executor

import (
	"context"
	"fmt"

	pb "github.com/manchtools/cadestro/contract/gen/go/cadestro/v1"
	sysreboot "github.com/manchtools/cadestro/sdk/sys/reboot"
)

func (e *Executor) executeReboot(ctx context.Context) (*pb.CommandOutput, error) {
	e.ensureDeps()

	if e.runner == nil {
		return nil, fmt.Errorf("no privilege runner configured; refusing to schedule reboot")
	}

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

func (e *Executor) Reboot(ctx context.Context) error {
	_, err := e.executeReboot(ctx)
	return err
}
