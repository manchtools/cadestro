package executor

import (
	"context"
	"errors"
	"fmt"
	"strings"

	pb "github.com/manchtools/cadestro/contract/gen/go/cadestro/v1"
	"github.com/manchtools/cadestro/sdk/pkg"
)

func (e *Executor) executeUpdate(ctx context.Context, _ *pb.UpdateActionParams) (*pb.CommandOutput, error) {
	if e.pkgManager == nil {
		return nil, fmt.Errorf("no supported package manager found")
	}
	var stdout strings.Builder
	index, err := e.pkgManager.Update(ctx)
	stdout.WriteString(index.Stdout)
	if err != nil && !errors.Is(err, pkg.ErrUnsupported) {
		return &pb.CommandOutput{ExitCode: int32(index.ExitCode), Stdout: stdout.String(), Stderr: index.Stderr}, fmt.Errorf("update package index: %w", err)
	}
	upgrade, err := e.pkgManager.UpgradeAll(ctx)
	stdout.WriteString(upgrade.Stdout)
	output := &pb.CommandOutput{ExitCode: int32(upgrade.ExitCode), Stdout: stdout.String(), Stderr: upgrade.Stderr}
	if err != nil {
		return output, fmt.Errorf("upgrade system packages: %w", err)
	}
	if upgrade.ExitCode != 0 {
		return output, fmt.Errorf("package upgrade exited with status %d", upgrade.ExitCode)
	}
	return output, nil
}
