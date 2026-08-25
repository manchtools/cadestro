package executor

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"strings"

	pb "github.com/manchtools/cadestro/contract/gen/go/cadestro/v1"
)

var errReadOnlyFS = errors.New("filesystem is read-only")

func (e *Executor) requireWritableFS(ctx context.Context) (*pb.CommandOutput, error) {
	repair := e.repairFilesystem
	if e.repairFS != nil {
		repair = e.repairFS
	}
	if repair(ctx) {
		return nil, nil
	}
	return &pb.CommandOutput{
		ExitCode: 1,
		Stderr:   "filesystem is read-only and could not be remounted",
	}, errReadOnlyFS
}

var validActionIDRegex = regexp.MustCompile(`^[A-Za-z0-9]+$`)

func envActionID(env *pb.Action) string {
	if env == nil || env.GetId() == nil {
		return ""
	}
	id := env.GetId().GetValue()
	if id == "" || len(id) > 64 || !validActionIDRegex.MatchString(id) {
		return ""
	}
	return id
}

func (e *Executor) syncGroupMembers(ctx context.Context, groupName string, desiredUsers []string, output *strings.Builder) (bool, error) {
	changed := false
	var errs []string

	for _, username := range desiredUsers {
		uExists, err := e.userExists(ctx, username)
		if err != nil {
			return changed, fmt.Errorf("check user %s: %w", username, err)
		}
		if !uExists {
			output.WriteString(fmt.Sprintf("warning: user %q does not exist, skipping group membership\n", username))
			continue
		}
		if !e.userInGroup(ctx, username, groupName) {
			if err := e.addUserToGroup(ctx, username, groupName); err != nil {
				msg := fmt.Sprintf("failed to add user %s to group %s: %v", username, groupName, err)
				output.WriteString(fmt.Sprintf("warning: %s\n", msg))
				errs = append(errs, msg)
			} else {
				output.WriteString(fmt.Sprintf("added user %s to group %s\n", username, groupName))
				changed = true
			}
		}
	}

	currentMembers := e.getGroupMembers(ctx, groupName)
	desiredSet := make(map[string]bool, len(desiredUsers))
	for _, u := range desiredUsers {
		desiredSet[u] = true
	}
	for _, member := range currentMembers {
		if !desiredSet[member] {
			if err := e.removeUserFromGroup(ctx, member, groupName); err != nil {
				msg := fmt.Sprintf("failed to remove user %s from group %s: %v", member, groupName, err)
				output.WriteString(fmt.Sprintf("warning: %s\n", msg))
				errs = append(errs, msg)
			} else {
				output.WriteString(fmt.Sprintf("removed user %s from group %s\n", member, groupName))
				changed = true
			}
		}
	}

	if len(errs) > 0 {
		return changed, fmt.Errorf("group membership errors: %s", strings.Join(errs, "; "))
	}
	return changed, nil
}

func (e *Executor) writeAndValidateConfig(ctx context.Context, path, content, mode, owner, group string, validateCmd string, validateArgs ...string) (*pb.CommandOutput, error) {
	if err := e.atomicWriteFile(ctx, path, content, mode, owner, group); err != nil {
		return nil, fmt.Errorf("write config file: %w", err)
	}

	validateOut, validateErr := e.runSudo(ctx, validateCmd, validateArgs...)
	if validateErr != nil {

		if rmErr := e.removeFileStrict(ctx, path); rmErr != nil {
			slog.Warn("failed to remove invalid config after validation failure", "path", path, "error", rmErr)
		}
		errMsg := "config validation failed"
		if validateOut != nil && validateOut.Stderr != "" {
			errMsg = strings.TrimSpace(validateOut.Stderr)
		}
		return &pb.CommandOutput{
			ExitCode: 1,
			Stderr:   errMsg,
		}, fmt.Errorf("%s validation failed: %s", validateCmd, errMsg)
	}

	return nil, nil
}

func (e *Executor) removeGroupWithConfig(ctx context.Context, groupName, configPath string, output *strings.Builder) (bool, error) {
	changed := false

	if configPath != "" && e.fileExistsWithSudo(ctx, configPath) {
		if _, err := e.requireWritableFS(ctx); err != nil {
			return false, fmt.Errorf("writable fs: %w", err)
		}
		if err := e.removeFileStrict(ctx, configPath); err != nil {
			return false, fmt.Errorf("remove config file %s: %w", configPath, err)
		}
		output.WriteString(fmt.Sprintf("removed config file: %s\n", configPath))
		changed = true
	}

	gExists, err := e.groupExists(ctx, groupName)
	if err != nil {
		return changed, fmt.Errorf("check group %s: %w", groupName, err)
	}
	if gExists {
		if !changed {

			if _, err := e.requireWritableFS(ctx); err != nil {
				return false, fmt.Errorf("writable fs: %w", err)
			}
		}
		members := e.getGroupMembers(ctx, groupName)
		for _, member := range members {
			if err := e.removeUserFromGroup(ctx, member, groupName); err != nil {
				output.WriteString(fmt.Sprintf("warning: failed to remove user %s from group %s: %v\n", member, groupName, err))
			} else {
				output.WriteString(fmt.Sprintf("removed user %s from group %s\n", member, groupName))
				changed = true
			}
		}
		if err := e.deps.user.GroupDelete(ctx, groupName); err != nil {
			return changed, fmt.Errorf("delete group %s: %w", groupName, err)
		}
		output.WriteString(fmt.Sprintf("deleted group: %s\n", groupName))
		changed = true
	}

	return changed, nil
}
