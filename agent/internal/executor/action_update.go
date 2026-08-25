// Package executor provides implementations for action executors.
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

// Notification seams so the update/LPS paths can be exercised with fixtures
// instead of a live host. Production binds them to the real notify Manager;
// tests stub them.
func (e *Executor) notifyAll(ctx context.Context, title, body string) {
	_ = e.deps.notify.NotifyAll(ctx, title, body)
}
func (e *Executor) notifyUsers(ctx context.Context, users []string, title, body string) {
	_ = e.deps.notify.NotifyUsers(ctx, users, title, body)
}

// repairFilesystem attempts to fix read-only filesystem issues.
// This can happen when the kernel remounts the filesystem as read-only due to errors.
// It checks all real (non-virtual) filesystem mounts, not just /, because partitions
// like /usr may be mounted separately and go read-only independently.
// Returns true if all filesystems are writable, false if any repair failed.
func (e *Executor) repairFilesystem(ctx context.Context) bool {
	e.ensureDeps()
	mounts, err := e.deps.fs.ListMounts(ctx)
	if err != nil {
		e.logger.Warn("could not list mounts", "error", err)
		return true // Assume writable, let operations fail naturally
	}

	allOk := true
	for _, mnt := range mounts {
		// Only real block-device filesystems: skip virtual mounts (proc, sysfs,
		// cgroup, tmpfs, …) which are legitimately read-only and must never be
		// remounted rw.
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

// executeUpdate performs a system-wide package update.
// It respects version pinning (apt-mark hold / dnf versionlock).
func (e *Executor) executeUpdate(ctx context.Context, params *pb.UpdateParams) (*pb.CommandOutput, bool, error) {
	e.ensureDeps()
	// WS16 #3: bind the package manager to the action ctx so the per-action
	// timeout reaches the index-update and generic-upgrade subprocesses. The
	// apt/dnf-specific upgrade paths already build a ctx-bound backend.
	mgr := e.pkgManagerForCtx(ctx)
	if mgr == nil {
		return nil, false, fmt.Errorf("no supported package manager found")
	}

	// Repair filesystem if mounted read-only (common after kernel errors)
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

	// Record pre-update reboot state to detect new reboot requirements.
	rebootRequiredBefore := e.rebootRequired(ctx)

	// Update package index
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

	// Re-check after index update (new updates may now be visible).
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

	// Perform the upgrade
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

	// Autoremove if requested. Delegate to the SDK Manager (a no-op on backends
	// with no native equivalent) and detect change via the SDK installed-count
	// comparison. Surface a call failure so the result reflects "we tried to
	// autoremove and it failed", not a clean success over stale packages.
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

	// Check if this run created a new reboot requirement.
	rebootRequiredAfter := e.rebootRequired(ctx)
	newRebootRequired := rebootRequiredAfter && !rebootRequiredBefore
	if rebootRequiredAfter {
		allOutput.WriteString("\n*** REBOOT REQUIRED ***\n")
		if newRebootRequired && params != nil && params.RebootIfRequired {
			// errors.Join keeps the reboot failure visible even when an
			// earlier upgrade error already occupied lastErr — a
			// first-error-wins guard would silently demote a reboot the
			// operator explicitly asked for. Join only on a real failure so
			// lastErr keeps its identity otherwise (the NA classification
			// below relies on it).
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

// scheduleRebootAfterUpdate issues the reboot the operator requested via
// RebootIfRequired and notifies signed-in users ONLY if the shutdown actually
// went out. A failure to schedule a reboot the operator explicitly asked for
// is a real action failure (returned, not a logged warning); the desktop
// notification is gated on success so users are never told "your system will
// reboot" when it won't. Returns nil when the reboot was scheduled.
func (e *Executor) scheduleRebootAfterUpdate(ctx context.Context, output *strings.Builder) error {
	// Fail closed without a privilege runner, exactly as executeReboot does: a
	// reboot must NEVER fall through to the process-global Direct runner, which
	// systemd-logind honors via polkit with no sudo. sysreboot.New already
	// rejects a nil runner, but guarding here keeps both reboot entry points
	// identical and defends against a future New that defaults the runner. See
	// action_reboot.go for the original workstation-reboot incident.
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

// rebootRequired reports whether the system needs a reboot, via the SDK reboot
// Manager (the reboot-required marker on Debian/Ubuntu, needs-restarting on
// Fedora/RHEL). A nil runner or a probe error reports false — best-effort,
// matching the prior behavior.
func (e *Executor) rebootRequired(ctx context.Context) bool {
	rb, err := sysreboot.New(e.runner)
	if err != nil {
		return false
	}
	required, _ := rb.IsRequired(ctx)
	return required
}
