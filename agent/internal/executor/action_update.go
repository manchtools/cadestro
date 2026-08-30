package executor

import (
	"context"
	"fmt"
	"strings"

	pb "github.com/manchtools/cadestro/contract/gen/go/cadestro/v1"
)

func (e *Executor) executeUpdate(ctx context.Context, _ *pb.UpdateActionParams) (*pb.CommandOutput, bool, error) {
	if e.pkgManager == nil {
		return nil, false, fmt.Errorf("no supported package manager found")
	}
	var stdout strings.Builder
	index, err := e.pkgManager.Update(ctx)
	stdout.WriteString(index.Stdout)
	if err != nil {
		return &pb.CommandOutput{ExitCode: int32(index.ExitCode), Stdout: stdout.String(), Stderr: index.Stderr}, false, fmt.Errorf("update package index: %w", err)
	}
	available, probeErr := e.pkgManager.HasUpdates(ctx)
	if probeErr == nil && !available {
		return &pb.CommandOutput{Stdout: stdout.String() + "system is already up to date"}, false, nil
	}
	upgrade, err := e.pkgManager.UpgradeAll(ctx)
	stdout.WriteString(upgrade.Stdout)
	output := &pb.CommandOutput{ExitCode: int32(upgrade.ExitCode), Stdout: stdout.String(), Stderr: upgrade.Stderr}
	if err != nil {
		return output, false, fmt.Errorf("upgrade system packages: %w", err)
	}
	if upgrade.ExitCode != 0 {
		return output, false, fmt.Errorf("package upgrade exited with status %d", upgrade.ExitCode)
	}
	return output, true, nil
}
