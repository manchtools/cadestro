package executor

import (
	"context"
	"fmt"
	"strings"

	pb "github.com/manchtools/cadestro/contract/gen/go/cadestro/v1"
	sysuser "github.com/manchtools/cadestro/sdk/sys/user"
)

func (e *Executor) executeGroup(ctx context.Context, params *pb.GroupParams, state pb.DesiredState) (*pb.CommandOutput, bool, error) {
	e.ensureDeps()
	if params == nil {
		return nil, false, fmt.Errorf("group params required")
	}
	if !sysuser.IsValidName(params.Name) {
		return nil, false, fmt.Errorf("invalid group name: %q", params.Name)
	}

	for _, m := range params.Members {
		if !sysuser.IsValidName(m) {
			return nil, false, fmt.Errorf("invalid member username: %q", m)
		}
	}

	switch state {
	case pb.DesiredState_DESIRED_STATE_ABSENT:
		return e.removeGroup(ctx, params.Name)
	default:
		return e.setupGroup(ctx, params)
	}
}

func (e *Executor) setupGroup(ctx context.Context, params *pb.GroupParams) (*pb.CommandOutput, bool, error) {
	var output strings.Builder
	changed := false

	exists, err := e.groupExists(ctx, params.Name)
	if err != nil {
		return nil, false, fmt.Errorf("check group %s: %w", params.Name, err)
	}

	if exists && e.sudoGroupMembersMatch(ctx, params.Name, params.Members) {
		output.WriteString(fmt.Sprintf("group %s already up to date\n", params.Name))
		return &pb.CommandOutput{
			ExitCode: 0,
			Stdout:   output.String(),
		}, false, nil
	}

	if out, err := e.requireWritableFS(ctx); err != nil {
		return out, false, err
	}

	if !exists {
		opts := sysuser.GroupCreateOptions{System: params.SystemGroup}
		if params.Gid > 0 {
			opts.GID = int(params.Gid)
		}
		if err := e.deps.user.GroupCreate(ctx, params.Name, opts); err != nil {
			return nil, false, fmt.Errorf("create group %s: %v", params.Name, err)
		}
		output.WriteString(fmt.Sprintf("created group: %s\n", params.Name))
		changed = true
	}

	if memberChanged, err := e.syncGroupMembers(ctx, params.Name, params.Members, &output); err != nil {
		return &pb.CommandOutput{ExitCode: 1, Stdout: output.String(), Stderr: err.Error()}, memberChanged, err
	} else if memberChanged {
		changed = true
	}

	return &pb.CommandOutput{
		ExitCode: 0,
		Stdout:   output.String(),
	}, changed, nil
}

func (e *Executor) removeGroup(ctx context.Context, groupName string) (*pb.CommandOutput, bool, error) {
	var output strings.Builder

	exists, err := e.groupExists(ctx, groupName)
	if err != nil {
		return nil, false, fmt.Errorf("check group %s: %w", groupName, err)
	}
	if !exists {
		output.WriteString(fmt.Sprintf("group %s does not exist, nothing to remove\n", groupName))
		return &pb.CommandOutput{
			ExitCode: 0,
			Stdout:   output.String(),
		}, false, nil
	}

	changed, err := e.removeGroupWithConfig(ctx, groupName, "", &output)
	if err != nil {
		return nil, false, err
	}

	return &pb.CommandOutput{
		ExitCode: 0,
		Stdout:   output.String(),
	}, changed, nil
}
