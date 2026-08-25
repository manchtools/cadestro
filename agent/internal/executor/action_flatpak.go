package executor

import (
	"context"
	"fmt"
	"strings"

	pb "github.com/manchtools/cadestro/contract/gen/go/cadestro/v1"
	packageSDK "github.com/manchtools/cadestro/sdk/pkg"
	"github.com/manchtools/cadestro/sdk/sys/desktop"
)

func (e *Executor) executeFlatpak(ctx context.Context, params *pb.FlatpakParams, state pb.DesiredState) (*pb.CommandOutput, bool, error) {
	e.ensureDeps()
	if params == nil {
		return nil, false, fmt.Errorf("flatpak params required")
	}

	if params.GetAppId().GetValue() == "" {
		return nil, false, fmt.Errorf("flatpak app_id is required")
	}
	if err := packageSDK.ValidatePackageName(params.GetAppId().GetValue()); err != nil {
		return nil, false, fmt.Errorf("invalid flatpak app_id: %w", err)
	}

	remote := params.Remote
	if remote == "" {
		remote = "flathub"
	}
	if err := packageSDK.ValidateRemoteName(remote); err != nil {
		return nil, false, fmt.Errorf("invalid flatpak remote: %w", err)
	}

	if !packageSDK.FlatpakAvailable() {
		return nil, false, notApplicable("flatpak not available on this system")
	}

	if params.SystemWide {
		return e.executeFlatpakSystem(ctx, params, state, remote)
	}
	return e.executeFlatpakPerUser(ctx, params, state, remote)
}

func (e *Executor) newPerUserFlatpak(s desktop.Session) (*packageSDK.FlatpakManager, error) {
	ru, err := desktop.RunAsRunner(e.runnerOrDirect(), s)
	if err != nil {
		return nil, fmt.Errorf("build run-as runner for %s: %w", s.Username, err)
	}
	return packageSDK.NewUserFlatpak(ru)
}

func (e *Executor) executeFlatpakSystem(ctx context.Context, params *pb.FlatpakParams, state pb.DesiredState, remote string) (*pb.CommandOutput, bool, error) {
	mgr, err := packageSDK.NewFlatpak(e.runnerOrDirect())
	if err != nil {
		return nil, false, fmt.Errorf("build flatpak manager: %w", err)
	}

	installed, err := mgr.IsInstalled(ctx, params.GetAppId().GetValue())
	if err != nil {
		return nil, false, fmt.Errorf("check flatpak %s installed: %w", params.GetAppId().GetValue(), err)
	}

	switch state {
	case pb.DesiredState_DESIRED_STATE_PRESENT:
		if installed {
			out := &pb.CommandOutput{
				ExitCode: 0,
				Stdout:   fmt.Sprintf("flatpak %s is already installed", params.GetAppId().GetValue()),
			}

			if params.Pin {
				changed, pinErr := ensureFlatpakPinned(ctx, mgr, params.GetAppId().GetValue())
				if pinErr != nil {
					out.ExitCode = 1
					out.Stderr = pinErr.Error()
					return out, false, pinErr
				}
				if changed {
					out.Stdout += "\npinned"
				}
				return out, changed, nil
			}
			return out, false, nil
		}

		if out, err := e.requireWritableFS(ctx); err != nil {
			return out, false, err
		}

		out, _, instErr := packageResult(mgr.Install(ctx, remote, params.GetAppId().GetValue()))
		if instErr != nil {
			return out, false, fmt.Errorf("flatpak install failed: %w", instErr)
		}

		if params.Pin {
			if _, pinErr := ensureFlatpakPinned(ctx, mgr, params.GetAppId().GetValue()); pinErr != nil {
				if out == nil {
					out = &pb.CommandOutput{}
				}
				out.Stderr += "\n" + pinErr.Error()
				return out, true, fmt.Errorf("flatpak installed but pin failed: %w", pinErr)
			}
		}
		return out, true, nil

	case pb.DesiredState_DESIRED_STATE_ABSENT:
		if !installed {
			return &pb.CommandOutput{
				ExitCode: 0,
				Stdout:   fmt.Sprintf("flatpak %s is already not installed", params.GetAppId().GetValue()),
			}, false, nil
		}

		if out, err := e.requireWritableFS(ctx); err != nil {
			return out, false, err
		}

		if _, err := mgr.Unpin(ctx, params.GetAppId().GetValue()); err != nil {
			e.logger.Debug("flatpak ABSENT: unmask before uninstall failed (often expected if not pinned)",
				"app_id", params.GetAppId().GetValue(), "error", err)
		}

		return packageResult(mgr.Remove(ctx, packageSDK.RemoveOptions{}, params.GetAppId().GetValue()))
	}

	return nil, false, fmt.Errorf("unknown desired state: %v", state)
}

func (e *Executor) executeFlatpakPerUser(ctx context.Context, params *pb.FlatpakParams, state pb.DesiredState, remote string) (*pb.CommandOutput, bool, error) {
	switch state {
	case pb.DesiredState_DESIRED_STATE_PRESENT:
		sessions, err := e.deps.desktop.ActiveSessions(ctx)
		if err != nil {
			return nil, false, fmt.Errorf("enumerate active desktop sessions: %w", err)
		}
		if len(sessions) == 0 {
			e.logger.Warn("flatpak PRESENT: no active desktop sessions; per-user install deferred until a user signs in",
				"app_id", params.GetAppId().GetValue())
			return &pb.CommandOutput{
				ExitCode: 0,
				Stdout:   fmt.Sprintf("skipped: no signed-in desktop users to install %s for; will run again on next reconciliation", params.GetAppId().GetValue()),
			}, false, nil
		}

		if out, err := e.requireWritableFS(ctx); err != nil {
			return out, false, err
		}

		var (
			anyChanged   bool
			perUserOut   = &strings.Builder{}
			firstFailure error
		)
		for _, s := range sessions {
			line := func(body string) {
				perUserOut.WriteString("user=")
				perUserOut.WriteString(s.Username)
				perUserOut.WriteString(": ")
				perUserOut.WriteString(body)
				perUserOut.WriteString("\n")
			}

			umgr, mkErr := e.newPerUserFlatpak(s)
			if mkErr != nil {
				if firstFailure == nil {
					firstFailure = fmt.Errorf("user %s: %w", s.Username, mkErr)
				}
				e.logger.Warn("flatpak PRESENT: per-user manager setup failed",
					"user", s.Username, "app_id", params.GetAppId().GetValue(), "error", mkErr)
				line("setup failed: " + mkErr.Error())
				continue
			}

			if installed, _ := umgr.IsInstalled(ctx, params.GetAppId().GetValue()); installed {
				line(fmt.Sprintf("flatpak %s already installed; skipped", params.GetAppId().GetValue()))

				if params.Pin {
					changed, pinErr := ensureFlatpakPinned(ctx, umgr, params.GetAppId().GetValue())
					if pinErr != nil {
						if firstFailure == nil {
							firstFailure = fmt.Errorf("user %s: %w", s.Username, pinErr)
						}
						e.logger.Warn("flatpak PRESENT: per-user pin (mask) failed",
							"user", s.Username, "app_id", params.GetAppId().GetValue(), "error", pinErr)
						line("pin failed: " + pinErr.Error())
					} else if changed {
						anyChanged = true
						line("pinned " + params.GetAppId().GetValue())
					}
				}
				continue
			}

			if _, runErr := umgr.Install(ctx, remote, params.GetAppId().GetValue()); runErr != nil {
				if firstFailure == nil {
					firstFailure = fmt.Errorf("user %s: install failed: %w", s.Username, runErr)
				}
				e.logger.Warn("flatpak PRESENT: per-user install failed",
					"user", s.Username, "app_id", params.GetAppId().GetValue(), "error", runErr)
				line(runErr.Error())
				continue
			}
			anyChanged = true
			line(fmt.Sprintf("installed %s", params.GetAppId().GetValue()))

			if params.Pin {
				if _, pinErr := ensureFlatpakPinned(ctx, umgr, params.GetAppId().GetValue()); pinErr != nil {

					if firstFailure == nil {
						firstFailure = fmt.Errorf("user %s: install succeeded but %w", s.Username, pinErr)
					}
					e.logger.Warn("flatpak PRESENT: per-user pin (mask) failed (install succeeded)",
						"user", s.Username, "app_id", params.GetAppId().GetValue(), "error", pinErr)
					line("pin failed: " + pinErr.Error())
				}
			}
		}

		return &pb.CommandOutput{Stdout: perUserOut.String()}, anyChanged, firstFailure

	case pb.DesiredState_DESIRED_STATE_ABSENT:
		users, err := e.deps.desktop.UsersWithFlatpakInstall(ctx, params.GetAppId().GetValue())
		if err != nil {
			return nil, false, fmt.Errorf("enumerate per-user flatpak installs: %w", err)
		}
		if len(users) == 0 {
			return &pb.CommandOutput{
				ExitCode: 0,
				Stdout:   fmt.Sprintf("flatpak %s is already not installed for any user", params.GetAppId().GetValue()),
			}, false, nil
		}

		if out, err := e.requireWritableFS(ctx); err != nil {
			return out, false, err
		}

		var (
			anyChanged   bool
			perUserOut   = &strings.Builder{}
			firstFailure error
		)
		for _, u := range users {
			perUserOut.WriteString("user=")
			perUserOut.WriteString(u.Username)
			perUserOut.WriteString(": ")

			umgr, mkErr := e.newPerUserFlatpak(u)
			if mkErr != nil {
				if firstFailure == nil {
					firstFailure = fmt.Errorf("user %s: %w", u.Username, mkErr)
				}
				e.logger.Warn("flatpak ABSENT: per-user manager setup failed",
					"user", u.Username, "app_id", params.GetAppId().GetValue(), "error", mkErr)
				perUserOut.WriteString("setup failed: " + mkErr.Error() + "\n")
				continue
			}

			if _, err := umgr.Unpin(ctx, params.GetAppId().GetValue()); err != nil {
				e.logger.Debug("flatpak ABSENT: per-user unmask before uninstall failed (often expected if not pinned)",
					"user", u.Username, "app_id", params.GetAppId().GetValue(), "error", err)
			}

			if _, runErr := umgr.Remove(ctx, packageSDK.RemoveOptions{}, params.GetAppId().GetValue()); runErr != nil {
				if firstFailure == nil {
					firstFailure = fmt.Errorf("user %s: uninstall failed: %w", u.Username, runErr)
				}
				e.logger.Warn("flatpak ABSENT: per-user uninstall failed",
					"user", u.Username, "app_id", params.GetAppId().GetValue(), "error", runErr)
				perUserOut.WriteString(runErr.Error() + "\n")
				continue
			}
			anyChanged = true
			perUserOut.WriteString("uninstalled\n")
		}

		return &pb.CommandOutput{Stdout: perUserOut.String()}, anyChanged, firstFailure
	}
	return nil, false, fmt.Errorf("unknown desired state: %v", state)
}

func ensureFlatpakPinned(ctx context.Context, mgr *packageSDK.FlatpakManager, appID string) (bool, error) {
	pinned, err := mgr.IsPinned(ctx, appID)
	if err != nil {
		return false, fmt.Errorf("check pin %s: %w", appID, err)
	}
	if pinned {
		return false, nil
	}
	if _, err := mgr.Pin(ctx, appID); err != nil {
		return false, fmt.Errorf("pin (mask) %s: %w", appID, err)
	}
	return true, nil
}
