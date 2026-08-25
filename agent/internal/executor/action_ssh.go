package executor

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"

	pb "github.com/manchtools/cadestro/contract/gen/go/cadestro/v1"
	sysuser "github.com/manchtools/cadestro/sdk/sys/user"
)

func shortGroupName(prefix, actionID string) string {
	const linuxGroupMax = 32
	lower := strings.ToLower(actionID)
	full := prefix + lower
	if len(full) <= linuxGroupMax {
		return full
	}
	sum := sha256.Sum256([]byte(lower))
	hashHex := hex.EncodeToString(sum[:])

	const hashLen = 8
	const sepLen = 1
	keep := linuxGroupMax - len(prefix) - sepLen - hashLen
	if keep < 1 {

		return prefix[:linuxGroupMax-sepLen-hashLen] + "-" + hashHex[:hashLen]
	}
	return prefix + lower[:keep] + "-" + hashHex[:hashLen]
}

const maxActionIDForFilesystem = 64

func validateActionIDForFilesystem(actionID string) error {
	if actionID == "" {
		return fmt.Errorf("action ID required for group/file naming")
	}
	if len(actionID) > maxActionIDForFilesystem {
		return fmt.Errorf("action ID %q exceeds %d-character limit for filesystem use", actionID, maxActionIDForFilesystem)
	}
	if !validActionIDRegex.MatchString(actionID) {
		return fmt.Errorf("action ID %q contains characters that are unsafe for filesystem paths", actionID)
	}
	return nil
}

func sshGroupName(actionID string) string {
	return shortGroupName("cadestro-ssh-", actionID)
}

func sshConfigPath(actionID string) string {
	return fmt.Sprintf("/etc/ssh/sshd_config.d/cadestro-ssh-%s.conf", strings.ToLower(actionID))
}

func sshEffectiveUsers(params *pb.SshParams) []string {
	return params.Users
}

func (e *Executor) executeSsh(ctx context.Context, params *pb.SshParams, state pb.DesiredState, actionID string) (*pb.CommandOutput, bool, error) {
	e.ensureDeps()
	if params == nil {
		return nil, false, fmt.Errorf("ssh params required")
	}
	if err := validateActionIDForFilesystem(actionID); err != nil {
		return nil, false, err
	}

	users := sshEffectiveUsers(params)
	if len(users) == 0 {
		return nil, false, fmt.Errorf("at least one user is required")
	}
	for _, u := range users {
		if !sysuser.IsValidName(u) {
			return nil, false, fmt.Errorf("invalid username: %s", u)
		}
	}

	groupName := sshGroupName(actionID)
	configPath := sshConfigPath(actionID)

	switch state {
	case pb.DesiredState_DESIRED_STATE_ABSENT:
		return e.removeSshAccess(ctx, groupName, configPath)
	default:
		return e.setupSshAccess(ctx, params, users, groupName, configPath)
	}
}

func generateSshGroupConfig(groupName string, params *pb.SshParams) string {
	lines := []string{
		"# Managed by Cadestro - do not edit manually",
		fmt.Sprintf("Match Group %s", groupName),
	}
	if params.AllowPubkey {
		lines = append(lines, "    PubkeyAuthentication yes")
		lines = append(lines, "    AuthorizedKeysFile .ssh/authorized_keys")
	} else {
		lines = append(lines, "    PubkeyAuthentication no")
	}
	if params.AllowPassword {
		lines = append(lines, "    PasswordAuthentication yes")
	} else {
		lines = append(lines, "    PasswordAuthentication no")
	}
	return strings.Join(lines, "\n") + "\n"
}

func (e *Executor) setupSshAccess(ctx context.Context, params *pb.SshParams, users []string, groupName, configPath string) (*pb.CommandOutput, bool, error) {
	var output strings.Builder
	changed := false

	content := generateSshGroupConfig(groupName, params)

	fileMatches := e.configMatchesDesired(ctx, configPath, content)
	membersMatch := e.sudoGroupMembersMatch(ctx, groupName, users)
	if fileMatches && membersMatch {
		output.WriteString(fmt.Sprintf("SSH config already up to date: %s\n", configPath))
		return &pb.CommandOutput{
			ExitCode: 0,
			Stdout:   output.String(),
		}, false, nil
	}

	if out, err := e.requireWritableFS(ctx); err != nil {
		return out, false, err
	}

	gExists, err := e.groupExists(ctx, groupName)
	if err != nil {
		return nil, false, fmt.Errorf("check group %s: %w", groupName, err)
	}
	if !gExists {
		if err := e.deps.user.GroupCreate(ctx, groupName, sysuser.GroupCreateOptions{}); err != nil {
			return nil, false, fmt.Errorf("create group %s: %v", groupName, err)
		}
		output.WriteString(fmt.Sprintf("created group: %s\n", groupName))
		changed = true
	}

	if !fileMatches {

		if err := e.createDirectory(ctx, "/etc/ssh/sshd_config.d", true); err != nil {
			return nil, false, fmt.Errorf("create sshd_config.d: %w", err)
		}
		if out, err := e.writeAndValidateConfig(ctx, configPath, content, "0644", "root", "root", "sshd", "-t"); err != nil {
			return out, false, err
		}
		output.WriteString(fmt.Sprintf("wrote SSH config: %s\n", configPath))
		changed = true
		e.reloadSshd(ctx, &output)
	}

	if memberChanged, err := e.syncGroupMembers(ctx, groupName, users, &output); err != nil {
		return &pb.CommandOutput{ExitCode: 1, Stdout: output.String(), Stderr: err.Error()}, changed, err
	} else if memberChanged {
		changed = true
	}

	return &pb.CommandOutput{
		ExitCode: 0,
		Stdout:   output.String(),
	}, changed, nil
}

func (e *Executor) removeSshAccess(ctx context.Context, groupName, configPath string) (*pb.CommandOutput, bool, error) {
	var output strings.Builder

	changed, err := e.removeGroupWithConfig(ctx, groupName, configPath, &output)
	if err != nil {
		if !changed {
			return nil, false, err
		}
		output.WriteString(fmt.Sprintf("warning: %v\n", err))
	}

	if changed {
		e.reloadSshd(ctx, &output)
	}

	if !changed {
		output.WriteString("SSH access does not exist, nothing to remove\n")
	}

	return &pb.CommandOutput{
		ExitCode: 0,
		Stdout:   output.String(),
	}, changed, nil
}

func (e *Executor) configMatchesDesired(ctx context.Context, path, desiredContent string) bool {
	if !e.fileExistsWithSudo(ctx, path) {
		return false
	}
	existing, err := e.readFileWithSudo(ctx, path)
	if err != nil {
		return false
	}
	return existing == desiredContent
}

func (e *Executor) executeSshd(ctx context.Context, params *pb.SshdParams, state pb.DesiredState, actionID string) (*pb.CommandOutput, bool, error) {
	e.ensureDeps()
	if params == nil {
		return nil, false, fmt.Errorf("sshd params required")
	}
	if len(params.Directives) == 0 && state != pb.DesiredState_DESIRED_STATE_ABSENT {
		return nil, false, fmt.Errorf("at least one directive is required")
	}
	if err := validateActionIDForFilesystem(actionID); err != nil {
		return nil, false, err
	}

	configPath := fmt.Sprintf("/etc/ssh/sshd_config.d/%04d-cadestro-%s.conf", params.Priority, strings.ToLower(actionID))

	switch state {
	case pb.DesiredState_DESIRED_STATE_ABSENT:
		return e.removeSshdConfig(ctx, configPath)
	default:
		return e.setupSshdConfig(ctx, params, configPath)
	}
}

func generateSshdGlobalConfig(params *pb.SshdParams) (string, error) {
	var lines []string
	lines = append(lines, "# Managed by Cadestro - do not edit manually")
	for _, d := range params.Directives {
		if strings.ContainsAny(d.Key, "\n\r\x00") {
			return "", fmt.Errorf("sshd directive key contains forbidden control character (CR, LF, or NUL)")
		}
		if strings.ContainsAny(d.Value, "\n\r\x00") {
			return "", fmt.Errorf("sshd directive %q value contains forbidden control character (CR, LF, or NUL)", d.Key)
		}
		lines = append(lines, fmt.Sprintf("%s %s", d.Key, d.Value))
	}
	return strings.Join(lines, "\n") + "\n", nil
}

func (e *Executor) setupSshdConfig(ctx context.Context, params *pb.SshdParams, configPath string) (*pb.CommandOutput, bool, error) {
	var output strings.Builder

	content, err := generateSshdGlobalConfig(params)
	if err != nil {
		return nil, false, err
	}

	if e.configMatchesDesired(ctx, configPath, content) {
		output.WriteString(fmt.Sprintf("SSHD config already up to date: %s\n", configPath))
		return &pb.CommandOutput{
			ExitCode: 0,
			Stdout:   output.String(),
		}, false, nil
	}

	if out, err := e.requireWritableFS(ctx); err != nil {
		return out, false, err
	}

	if err := e.createDirectory(ctx, "/etc/ssh/sshd_config.d", true); err != nil {
		return nil, false, fmt.Errorf("create sshd_config.d: %w", err)
	}

	if out, err := e.writeAndValidateConfig(ctx, configPath, content, "0644", "root", "root", "sshd", "-t"); err != nil {
		return out, false, err
	}
	output.WriteString(fmt.Sprintf("created SSHD config: %s\n", configPath))

	e.reloadSshd(ctx, &output)

	return &pb.CommandOutput{
		ExitCode: 0,
		Stdout:   output.String(),
	}, true, nil
}

func (e *Executor) removeSshdConfig(ctx context.Context, configPath string) (*pb.CommandOutput, bool, error) {
	var output strings.Builder

	if !e.fileExistsWithSudo(ctx, configPath) {
		output.WriteString(fmt.Sprintf("SSHD config does not exist: %s\n", configPath))
		return &pb.CommandOutput{
			ExitCode: 0,
			Stdout:   output.String(),
		}, false, nil
	}

	if out, err := e.requireWritableFS(ctx); err != nil {
		return out, false, err
	}

	if err := e.removeFileStrict(ctx, configPath); err != nil {
		return nil, false, fmt.Errorf("remove sshd config: %w", err)
	}
	output.WriteString(fmt.Sprintf("removed SSHD config: %s\n", configPath))
	e.reloadSshd(ctx, &output)

	return &pb.CommandOutput{
		ExitCode: 0,
		Stdout:   output.String(),
	}, true, nil
}
