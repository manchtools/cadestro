package executor

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log/slog"
	"strings"
	"time"

	pb "github.com/manchtools/cadestro/contract/gen/go/cadestro/v1"
	sysuser "github.com/manchtools/cadestro/sdk/sys/user"

	"github.com/manchtools/cadestro/agent/internal/store"
)

type LpsPasswordStore interface {
	StorePasswords(ctx context.Context, actionID string, rotations []*pb.LpsPasswordRotation) error
}

func lpsRotationReason(reason string) pb.RotationReason {
	switch reason {
	case "initial", "user_created":
		return pb.RotationReason_ROTATION_REASON_INITIAL
	case "scheduled":
		return pb.RotationReason_ROTATION_REASON_SCHEDULED
	case "auth_grace":
		return pb.RotationReason_ROTATION_REASON_AUTH_GRACE
	default:
		return pb.RotationReason_ROTATION_REASON_UNSPECIFIED
	}
}

func (e *Executor) executeLps(ctx context.Context, params *pb.LpsParams, state pb.DesiredState, actionID string) (*pb.CommandOutput, bool, map[string]string, error) {
	e.ensureDeps()
	if params == nil {
		return nil, false, nil, fmt.Errorf("lps params required")
	}
	if actionID == "" {
		return nil, false, nil, fmt.Errorf("action ID required for LPS state tracking")
	}
	if len(params.Usernames) == 0 {
		return nil, false, nil, fmt.Errorf("at least one username is required")
	}
	for _, u := range params.Usernames {
		if !sysuser.IsValidName(u) {
			return nil, false, nil, fmt.Errorf("invalid username: %q", u)
		}
	}

	switch state {
	case pb.DesiredState_DESIRED_STATE_ABSENT:
		return e.removeLpsManagement(ctx, actionID)
	default:
		return e.setupLpsPasswords(ctx, params, actionID)
	}
}

func (e *Executor) setupLpsPasswords(ctx context.Context, params *pb.LpsParams, actionID string) (*pb.CommandOutput, bool, map[string]string, error) {
	e.ensureDeps()
	st := e.getStore()
	if st == nil {
		return nil, false, nil, fmt.Errorf("agent store not configured")
	}

	lpsStore := e.getLpsPasswordStore()
	if lpsStore == nil {
		return nil, false, nil, fmt.Errorf("LPS rotation requires a connection to the server (not connected)")
	}
	var output strings.Builder

	userStates, err := st.GetLpsState(ctx, actionID)
	if err != nil {
		e.logger.Warn("failed to load LPS state, will treat as initial rotation", "action_id", actionID, "error", err)
		userStates = make(map[string]*store.LpsUserState)
	}

	if params.Complexity == pb.LpsPasswordComplexity_LPS_PASSWORD_COMPLEXITY_UNSPECIFIED {
		e.logger.Warn("LPS policy has no complexity set, defaulting to alphanumeric",
			"action_id", actionID)
	}
	complexity := sysuser.ComplexityAlphanumeric
	if params.Complexity == pb.LpsPasswordComplexity_LPS_PASSWORD_COMPLEXITY_COMPLEX {
		complexity = sysuser.ComplexityComplex
	}

	reported := 0
	var rotatedUsers []string
	var anyError error

	for _, username := range params.Usernames {

		uExists, err := e.userExists(ctx, username)
		if err != nil {
			anyError = fmt.Errorf("check user %s: %w", username, err)
			output.WriteString(fmt.Sprintf("LPS: %s — failed to verify user: %v\n", username, err))
			continue
		}
		if !uExists {
			output.WriteString(fmt.Sprintf("LPS: user %q does not exist, skipping\n", username))
			e.logger.Warn("LPS user does not exist, skipping", "username", username)
			continue
		}

		storedState := userStates[username]

		rotate, reason := e.shouldRotateLps(ctx, storedState, params, username, e.now().UTC())
		if !rotate {
			output.WriteString(fmt.Sprintf("LPS: %s — password up to date\n", username))
			continue
		}

		requested := int(params.PasswordLength)
		length := requested
		if length < sysuser.MinPasswordLength {
			length = sysuser.MinPasswordLength
		}
		if length > sysuser.MaxPasswordLength {
			length = sysuser.MaxPasswordLength
		}
		if length != requested {
			e.logger.Warn("LPS password length clamped to SDK bounds",
				"action_id", actionID, "username", username,
				"requested", requested, "effective", length,
				"min", sysuser.MinPasswordLength, "max", sysuser.MaxPasswordLength)
		}
		password, err := sysuser.GeneratePassword(length, complexity)
		if err != nil {
			anyError = fmt.Errorf("generate password for %s: %w", username, err)
			output.WriteString(fmt.Sprintf("LPS: %s — failed to generate password: %v\n", username, err))
			continue
		}

		plaintext := password.Reveal()
		rotatedAt := e.now().UTC()
		passwordBytes, err := copySecret([]byte(plaintext))
		if err != nil {
			anyError = fmt.Errorf("prepare password for %s: %w", username, err)
			output.WriteString(fmt.Sprintf("LPS: %s — failed to prepare password for server, not rotating\n", username))
			continue
		}
		if err := lpsStore.StorePasswords(ctx, actionID, []*pb.LpsPasswordRotation{{
			Username:  username,
			Password:  passwordBytes,
			RotatedAt: rotatedAt.Format(time.RFC3339),
			Reason:    lpsRotationReason(reason),
		}}); err != nil {
			anyError = fmt.Errorf("report password for %s: %w", username, err)
			output.WriteString(fmt.Sprintf("LPS: %s — failed to report password to server, not rotating: %v\n", username, err))
			continue
		}

		if err := e.deps.user.SetPassword(ctx, username, password); err != nil {
			anyError = fmt.Errorf("set password for %s: %w", username, err)
			output.WriteString(fmt.Sprintf("LPS: %s — failed to set password: %v\n", username, err))
			continue
		}

		rotatedUsers = append(rotatedUsers, username)

		now := rotatedAt
		output.WriteString(fmt.Sprintf("LPS: %s — rotated password (reason: %s)\n", username, reason))

		hash := sha256.Sum256([]byte(plaintext))
		hashStr := hex.EncodeToString(hash[:])
		if err := st.SetLpsUserState(context.WithoutCancel(ctx), actionID, username, now, hashStr); err != nil {

			e.logger.Error("failed to persist LPS rotation state; next cycle will re-rotate",
				"action_id", actionID, "username", username, "error", err)
			anyError = fmt.Errorf("rotated password for %s but failed to persist rotation state (will re-rotate next cycle): %w", username, err)
		}

		reported++
	}

	if len(rotatedUsers) > 0 {
		e.notifyUsers(ctx, rotatedUsers, "Session Termination",
			"Your password has been changed by Cadestro. All sessions will be terminated in 60 seconds. Please save your work.")
		output.WriteString(fmt.Sprintf("LPS: notified %d user(s), waiting 60 seconds before session termination\n", len(rotatedUsers)))

		select {
		case <-time.After(60 * time.Second):
		case <-ctx.Done():
			output.WriteString("LPS: grace period interrupted\n")
		}

		for _, username := range rotatedUsers {
			e.killUserSessions(ctx, username)
		}
		output.WriteString(fmt.Sprintf("LPS: terminated sessions for %d user(s)\n", len(rotatedUsers)))
	}

	if reported == 0 {
		if anyError != nil {
			return &pb.CommandOutput{
				ExitCode: 1,
				Stdout:   output.String(),
			}, false, nil, anyError
		}
		return &pb.CommandOutput{
			ExitCode: 0,
			Stdout:   output.String(),
		}, false, nil, nil
	}

	return &pb.CommandOutput{
		ExitCode: 0,
		Stdout:   output.String(),
	}, true, nil, anyError
}

func (e *Executor) removeLpsManagement(ctx context.Context, actionID string) (*pb.CommandOutput, bool, map[string]string, error) {
	st := e.getStore()
	if st == nil {
		return nil, false, nil, fmt.Errorf("agent store not configured")
	}
	userStates, err := st.GetLpsState(ctx, actionID)
	if err != nil {

		e.logger.Error("removeLpsManagement: failed to read local state",
			"action_id", actionID, "error", err)
		return nil, false, nil, fmt.Errorf("get lps state: %w", err)
	}

	if len(userStates) > 0 {
		if err := st.DeleteLpsState(ctx, actionID); err != nil {

			e.logger.Error("failed to delete LPS state", "action_id", actionID, "error", err)
			return nil, false, nil, fmt.Errorf("delete lps state: %w", err)
		}
		return &pb.CommandOutput{
			ExitCode: 0,
			Stdout:   "LPS: password management stopped, state removed\n",
		}, true, nil, nil
	}

	return &pb.CommandOutput{
		ExitCode: 0,
		Stdout:   "LPS: password management not active, nothing to remove\n",
	}, false, nil, nil
}

func (e *Executor) shouldRotateLps(ctx context.Context, state *store.LpsUserState, params *pb.LpsParams, username string, now time.Time) (bool, string) {

	if state == nil {
		return true, "initial"
	}

	intervalDuration := time.Duration(params.RotationIntervalDays) * 24 * time.Hour
	if now.Sub(state.LastRotatedAt) >= intervalDuration {
		return true, "scheduled"
	}

	if params.GracePeriodHours > 0 {
		lastAuth, err := e.deps.user.LastLogin(ctx, username)
		if err == nil && !lastAuth.IsZero() && lastAuth.After(state.LastRotatedAt) {
			graceDuration := time.Duration(params.GracePeriodHours) * time.Hour
			if now.Sub(lastAuth) >= graceDuration {
				return true, "auth_grace"
			}
		}
	}

	return false, ""
}

func (e *Executor) killUserSessions(ctx context.Context, username string) {

	if err := e.deps.user.KillSessions(ctx, username); err != nil {
		slog.Warn("killUserSessions: SDK KillSessions failed (may be benign — no active sessions/processes)",
			"username", username, "error", err)
	}

	time.Sleep(500 * time.Millisecond)
}

func (e *Executor) reportUserCreatePassword(ctx context.Context, username, actionID, plaintext string, output *strings.Builder) {
	ps := e.getLpsPasswordStore()
	if ps == nil {
		e.logger.Warn("user create: no server connection; temp password not reported", "username", username)
		output.WriteString("warning: temporary password not reported (not connected; reset out of band)\n")
		return
	}
	passwordBytes, err := copySecret([]byte(plaintext))
	if err != nil {
		e.logger.Warn("user create: failed to prepare temp password", "username", username, "error", err)
		output.WriteString("warning: temporary password not reported (preparation failed; reset out of band)\n")
		return
	}
	if err := ps.StorePasswords(ctx, actionID, []*pb.LpsPasswordRotation{{
		Username:  username,
		Password:  passwordBytes,
		RotatedAt: e.now().UTC().Format(time.RFC3339),
		Reason:    pb.RotationReason_ROTATION_REASON_INITIAL,
	}}); err != nil {
		e.logger.Warn("user create: failed to report temp password", "username", username, "error", err)
		output.WriteString("warning: temporary password not reported (server rejected it; reset out of band)\n")
	}
}
