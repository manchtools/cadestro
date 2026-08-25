package executor

import (
	"context"
	"errors"
	"fmt"
	"strings"

	pb "github.com/manchtools/cadestro/contract/gen/go/cadestro/v1"
	sysexec "github.com/manchtools/cadestro/sdk/sys/exec"
	sysreboot "github.com/manchtools/cadestro/sdk/sys/reboot"
)

func (e *Executor) notifyAll(ctx context.Context, title, body string) {
	_ = e.deps.notify.NotifyAll(ctx, title, body)
}
func (e *Executor) notifyUsers(ctx context.Context, users []string, title, body string) {
	_ = e.deps.notify.NotifyUsers(ctx, users, title, body)
}

func (e *Executor) repairFilesystem(ctx context.Context) bool {
	e.ensureDeps()
	mounts, err := e.deps.fs.ListMounts(ctx)
	if err != nil {
		e.logger.Warn("could not list mounts", "error", err)
		return true
	}

	allOk := true
	for _, mnt := range mounts {

		if !strings.HasPrefix(mnt.Source, "/dev/") {
			continue
		}
		if !mnt.ReadOnly {
			continue
		}

		e.logger.Warn("filesystem is mounted read-only, attempting remount",
			"mount", mnt.Target, "device", mnt.Source,
		)

		if err := e.deps.fs.RemountRW(ctx, mnt.Target); err != nil {
			e.logger.Error("failed to remount filesystem as read-write",
				"mount", mnt.Target, "device", mnt.Source, "error", err,
			)
			e.logger.Error("filesystem may have errors - system likely needs reboot and fsck",
				"mount", mnt.Target,
			)
			allOk = false
		} else {
			e.logger.Info("successfully remounted filesystem as read-write",
				"mount", mnt.Target, "device", mnt.Source,
			)
		}
	}

	return allOk
}

func (e *Executor) executeUpdate(ctx context.Context, params *pb.UpdateParams) (*pb.CommandOutput, bool, error) {
	e.ensureDeps()

	mgr := e.pkgManagerForCtx(ctx)
	if mgr == nil {
		return nil, false, fmt.Errorf("no supported package manager found")
	}

	if out, err := e.requireWritableFS(ctx); err != nil {
		return out, false, err
	}

	var allOutput strings.Builder
	var lastErr error

	securityOnly := params != nil && params.SecurityOnly
	var updatesAvailable bool
	var hasUpdErr error
	if securityOnly {
		updatesAvailable, hasUpdErr = mgr.HasSecurityUpdates(ctx)
	} else {
		updatesAvailable, hasUpdErr = mgr.HasUpdates(ctx)
	}
	if hasUpdErr != nil {
		updatesAvailable = true
	}

	rebootRequiredBefore := e.rebootRequired(ctx)

	allOutput.WriteString("=== Package Index Update ===\n")
	if updateResult, err := mgr.Update(ctx); err != nil {
		allOutput.WriteString(updateResult.Stdout)
		allOutput.WriteString(updateResult.Stderr)
		allOutput.WriteString(fmt.Sprintf("Warning: update failed: %v\n\n", err))
	} else {
		allOutput.WriteString(updateResult.Stdout)
		if updateResult.Stderr != "" {
			allOutput.WriteString(updateResult.Stderr)
		}
		allOutput.WriteString("\n")
	}

	if !updatesAvailable {
		var u bool
		var err error
		if securityOnly {
			u, err = mgr.HasSecurityUpdates(ctx)
		} else {
			u, err = mgr.HasUpdates(ctx)
		}
		if err == nil {
			updatesAvailable = u
		}
	}

	allOutput.WriteString("=== Package Upgrade ===\n")

	var upgradeResult sysexec.Result
	var upgradeErr error
	if securityOnly {
		upgradeResult, upgradeErr = mgr.UpgradeSecurity(ctx)
	} else {
		upgradeResult, upgradeErr = mgr.UpgradeAll(ctx)
	}
	allOutput.WriteString(upgradeResult.Stdout)
	allOutput.WriteString(upgradeResult.Stderr)
	if upgradeErr != nil {
		allOutput.WriteString(fmt.Sprintf("Error: %v\n", upgradeErr))
		lastErr = upgradeErr
	}

	autoremoved := false
	if params != nil && params.Autoremove {
		allOutput.WriteString("\n=== Autoremove Unused Packages ===\n")
		countBefore, _ := mgr.InstalledCount(ctx)
		arOut, autoremoveErr := mgr.Autoremove(ctx)
		allOutput.WriteString(arOut.Stdout)
		if autoremoveErr != nil {
			allOutput.WriteString(arOut.Stderr)
		}
		countAfter, _ := mgr.InstalledCount(ctx)
		autoremoved = countBefore > 0 && countAfter > 0 && countBefore != countAfter
		if autoremoveErr != nil && lastErr == nil {
			lastErr = fmt.Errorf("autoremove: %w", autoremoveErr)
		}
	}

	rebootRequiredAfter := e.rebootRequired(ctx)
	newRebootRequired := rebootRequiredAfter && !rebootRequiredBefore
	if rebootRequiredAfter {
		allOutput.WriteString("\n*** REBOOT REQUIRED ***\n")
		if newRebootRequired && params != nil && params.RebootIfRequired {

			if rebootErr := e.scheduleRebootAfterUpdate(ctx, &allOutput); rebootErr != nil {
				lastErr = errors.Join(lastErr, rebootErr)
			}
		}
	}

	if securityOnlyNotApplicable(securityOnly, upgradeErr, lastErr) {
		return &pb.CommandOutput{
			ExitCode: 0,
			Stdout:   allOutput.String(),
		}, autoremoved || newRebootRequired, notApplicable("security-only upgrades unsupported on backend %q: %v", e.pkgBackend, upgradeErr)
	}

	exitCode := int32(0)
	if lastErr != nil {
		exitCode = 1
	}

	changed := updatesAvailable || autoremoved || newRebootRequired
	return &pb.CommandOutput{
		ExitCode: exitCode,
		Stdout:   allOutput.String(),
	}, changed, lastErr
}

func (e *Executor) scheduleRebootAfterUpdate(ctx context.Context, output *strings.Builder) error {

	if e.runner == nil {
		output.WriteString("FAILED to schedule reboot: no privilege runner configured\n")
		return fmt.Errorf("schedule reboot: no privilege runner configured")
	}
	rb, err := sysreboot.New(e.runner)
	if err == nil {
		err = rb.Schedule(ctx, sysreboot.ScheduleOptions{Delay: "+1", Message: "System update requires reboot"})
	}
	if err != nil {
		output.WriteString(fmt.Sprintf("FAILED to schedule reboot: %v\n", err))
		return fmt.Errorf("schedule reboot: %w", err)
	}
	e.notifyAll(ctx, "System Reboot", "A system update requires a reboot. This system will reboot in 1 minute.")
	output.WriteString("Scheduled reboot in 1 minute.\n")
	return nil
}

func (e *Executor) rebootRequired(ctx context.Context) bool {
	rb, err := sysreboot.New(e.runner)
	if err != nil {
		return false
	}
	required, _ := rb.IsRequired(ctx)
	return required
}
