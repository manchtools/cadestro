package executor

import (
	"context"
	"fmt"
	"slices"
	"strings"

	pb "github.com/manchtools/cadestro/contract/gen/go/cadestro/v1"
	sysuser "github.com/manchtools/cadestro/sdk/sys/user"
)

func sanitizeSudoGroupName(actionID string) string {
	return shortGroupName("cadestro-sudo-", actionID)
}

func sudoersFilePath(actionID string) string {
	return fmt.Sprintf("/etc/sudoers.d/cadestro-sudo-%s", strings.ToLower(actionID))
}

func (e *Executor) executeSudo(ctx context.Context, params *pb.AdminPolicyParams, state pb.DesiredState, actionID string) (*pb.CommandOutput, bool, error) {
	e.ensureDeps()
	if params == nil {
		return nil, false, fmt.Errorf("sudo params required")
	}
	if err := validateActionIDForFilesystem(actionID); err != nil {
		return nil, false, err
	}

	groupName := sanitizeSudoGroupName(actionID)
	filePath := sudoersFilePath(actionID)

	switch state {
	case pb.DesiredState_DESIRED_STATE_ABSENT:
		return e.removeSudoPolicy(ctx, groupName, filePath, params.Users)
	default:
		return e.setupSudoPolicy(ctx, params, groupName, filePath)
	}
}

func (e *Executor) setupSudoPolicy(ctx context.Context, params *pb.AdminPolicyParams, groupName, sudoersPath string) (*pb.CommandOutput, bool, error) {
	var output strings.Builder
	changed := false

	for _, u := range params.Users {
		if !sysuser.IsValidName(u) {
			return nil, false, fmt.Errorf("invalid username: %q", u)
		}
	}

	content, err := sudoConfigForParams(params, groupName)
	if err != nil {
		return nil, false, err
	}

	fileMatches := e.configMatchesDesired(ctx, sudoersPath, content)
	membersMatch := e.sudoGroupMembersMatch(ctx, groupName, params.Users)
	if fileMatches && membersMatch {
		output.WriteString(fmt.Sprintf("sudo policy already up to date: %s\n", sudoersPath))
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
		if out, err := e.writeAndValidateConfig(ctx, sudoersPath, content, "0440", "root", "root", "visudo", "-c", "-f", sudoersPath); err != nil {
			return out, false, err
		}
		output.WriteString(fmt.Sprintf("wrote sudoers file: %s\n", sudoersPath))
		changed = true
	}

	if memberChanged, err := e.syncGroupMembers(ctx, groupName, params.Users, &output); err != nil {
		return &pb.CommandOutput{ExitCode: 1, Stdout: output.String(), Stderr: err.Error()}, memberChanged, err
	} else if memberChanged {
		changed = true
	}

	return &pb.CommandOutput{
		ExitCode: 0,
		Stdout:   output.String(),
	}, changed, nil
}

func (e *Executor) removeSudoPolicy(ctx context.Context, groupName, sudoersPath string, _ []string) (*pb.CommandOutput, bool, error) {
	var output strings.Builder

	changed, err := e.removeGroupWithConfig(ctx, groupName, sudoersPath, &output)
	if err != nil {
		if !changed {

			return nil, false, err
		}

		output.WriteString(fmt.Sprintf("warning: %v\n", err))
	}

	if !changed {
		output.WriteString("sudo policy does not exist, nothing to remove\n")
	}

	return &pb.CommandOutput{
		ExitCode: 0,
		Stdout:   output.String(),
	}, changed, nil
}

func sudoConfigForParams(params *pb.AdminPolicyParams, groupName string) (string, error) {
	switch params.AccessLevel {
	case pb.AdminAccessLevel_ADMIN_ACCESS_LEVEL_FULL:
		return generateFullSudoConfig(groupName), nil
	case pb.AdminAccessLevel_ADMIN_ACCESS_LEVEL_LIMITED:
		return generateLimitedSudoConfig(groupName), nil
	case pb.AdminAccessLevel_ADMIN_ACCESS_LEVEL_CUSTOM:
		if params.CustomConfig == "" {
			return "", fmt.Errorf("custom_config is required when access_level is CUSTOM")
		}
		return generateCustomSudoConfig(groupName, params.CustomConfig), nil
	case pb.AdminAccessLevel_ADMIN_ACCESS_LEVEL_TERMINAL_ADMIN_LIMITED:
		return generateTerminalAdminLimitedSudoConfig(groupName), nil
	case pb.AdminAccessLevel_ADMIN_ACCESS_LEVEL_TERMINAL_ADMIN_FULL:
		return generateTerminalAdminFullSudoConfig(groupName), nil
	default:
		return "", fmt.Errorf("unsupported access level: %v", params.AccessLevel)
	}
}

func terminalAdminDefaultsBlock(groupName string) string {
	g := "%" + groupName
	return strings.Join([]string{
		"Defaults:" + g + " requiretty",
		"Defaults:" + g + " env_reset",
		"Defaults:" + g + " !lecture",
		"Defaults:" + g + " timestamp_timeout=0",
	}, "\n")
}

func terminalAdminLimitedDenyBlocks(groupName string) []string {
	return []string{
		"",
		"# Deny editor escapes (vim :!bash, less !sh, etc.) — ADR T2",
		fmt.Sprintf("%%%s ALL=(ALL) !/usr/bin/vim, !/usr/bin/vi, !/usr/bin/vimdiff, !/usr/bin/view, !/usr/bin/nvim", groupName),
		fmt.Sprintf("%%%s ALL=(ALL) !/usr/bin/emacs, !/usr/bin/emacsclient", groupName),
		fmt.Sprintf("%%%s ALL=(ALL) !/usr/bin/nano, !/bin/nano", groupName),
		fmt.Sprintf("%%%s ALL=(ALL) !/usr/bin/less, !/usr/bin/more, !/usr/bin/most", groupName),
		fmt.Sprintf("%%%s ALL=(ALL) !/usr/bin/ed, !/usr/bin/ex", groupName),
		fmt.Sprintf("%%%s ALL=(ALL) !/usr/bin/mc, !/usr/bin/joe, !/usr/bin/jed", groupName),
		"",
		"# Deny shell spawns — ADR T3 (path-based friction, not a boundary:",
		"# /usr/local/bin covered too so a locally-installed shell doesn't",
		"# trivially bypass the block — #174)",
		fmt.Sprintf("%%%s ALL=(ALL) !/bin/sh, !/bin/bash, !/bin/dash, !/bin/zsh, !/bin/ksh, !/bin/csh, !/bin/tcsh, !/bin/fish", groupName),
		fmt.Sprintf("%%%s ALL=(ALL) !/usr/bin/sh, !/usr/bin/bash, !/usr/bin/dash, !/usr/bin/zsh, !/usr/bin/ksh, !/usr/bin/csh, !/usr/bin/tcsh, !/usr/bin/fish", groupName),
		fmt.Sprintf("%%%s ALL=(ALL) !/usr/local/bin/sh, !/usr/local/bin/bash, !/usr/local/bin/dash, !/usr/local/bin/zsh, !/usr/local/bin/ksh, !/usr/local/bin/csh, !/usr/local/bin/tcsh, !/usr/local/bin/fish", groupName),
		fmt.Sprintf("%%%s ALL=(ALL) !/usr/bin/env, !/usr/local/bin/env", groupName),
		"",
		"# Deny persistence vectors — ADR T5",
		fmt.Sprintf("%%%s ALL=(ALL) !/usr/bin/at, !/usr/bin/atq, !/usr/bin/atrm, !/usr/bin/batch", groupName),
		fmt.Sprintf("%%%s ALL=(ALL) !/usr/bin/crontab", groupName),
		fmt.Sprintf("%%%s ALL=(ALL) !/usr/sbin/dpkg-divert, !/usr/bin/dpkg-divert", groupName),
		fmt.Sprintf("%%%s ALL=(ALL) !/usr/bin/update-alternatives, !/usr/sbin/update-alternatives", groupName),
	}
}

func generateTerminalAdminLimitedSudoConfig(groupName string) string {
	lines := []string{
		"# Managed by Cadestro — do not edit manually",
		fmt.Sprintf("# Passwordless LIMITED sudo for group %s (TerminalAdmin, server #70)", groupName),
		"",
		terminalAdminDefaultsBlock(groupName),
		"",
		"# Package management",
		fmt.Sprintf("%%%s ALL=(ALL) NOPASSWD: /usr/bin/apt, /usr/bin/apt-get, /usr/bin/apt-cache, /usr/bin/dpkg", groupName),
		fmt.Sprintf("%%%s ALL=(ALL) NOPASSWD: /usr/bin/dnf, /usr/bin/yum, /usr/bin/rpm", groupName),
		fmt.Sprintf("%%%s ALL=(ALL) NOPASSWD: /usr/bin/pacman", groupName),
		fmt.Sprintf("%%%s ALL=(ALL) NOPASSWD: /usr/bin/zypper", groupName),
		fmt.Sprintf("%%%s ALL=(ALL) NOPASSWD: /usr/bin/flatpak, /usr/bin/snap", groupName),
		fmt.Sprintf("%%%s ALL=(ALL) NOPASSWD: /usr/bin/nix, /usr/bin/nix-env, /usr/bin/nix-store, /usr/bin/nix-channel", groupName),
		fmt.Sprintf("%%%s ALL=(ALL) NOPASSWD: /sbin/apk", groupName),
		"",
		"# Service and system management",
		fmt.Sprintf("%%%s ALL=(ALL) NOPASSWD: /usr/bin/systemctl, /usr/bin/journalctl", groupName),
		fmt.Sprintf("%%%s ALL=(ALL) NOPASSWD: /usr/sbin/reboot, /usr/sbin/shutdown", groupName),
		fmt.Sprintf("%%%s ALL=(ALL) NOPASSWD: /usr/bin/timedatectl, /usr/bin/hostnamectl", groupName),
		"",
		"# Network management",
		fmt.Sprintf("%%%s ALL=(ALL) NOPASSWD: /usr/bin/ip, /usr/bin/nmcli, /usr/bin/networkctl", groupName),
		fmt.Sprintf("%%%s ALL=(ALL) NOPASSWD: /usr/sbin/ufw, /usr/bin/firewall-cmd", groupName),
		"",
		"# Disk and storage",
		fmt.Sprintf("%%%s ALL=(ALL) NOPASSWD: /usr/bin/mount, /usr/bin/umount, /usr/sbin/blkid", groupName),
		"",
		"# Containers",
		fmt.Sprintf("%%%s ALL=(ALL) NOPASSWD: /usr/bin/docker, /usr/bin/podman", groupName),
		"",
		"# Diagnostics",
		fmt.Sprintf("%%%s ALL=(ALL) NOPASSWD: /usr/bin/dmesg", groupName),
		"",
		"# Deny modifications to cadestrod and sudoers",
		fmt.Sprintf("%%%s ALL=(ALL) !/usr/bin/systemctl * cadestrod*", groupName),
		fmt.Sprintf("%%%s ALL=(ALL) !/usr/bin/visudo, !/usr/sbin/visudo", groupName),
	}
	lines = append(lines, terminalAdminLimitedDenyBlocks(groupName)...)
	return strings.Join(lines, "\n") + "\n"
}

func generateTerminalAdminFullSudoConfig(groupName string) string {
	lines := []string{
		"# Managed by Cadestro — do not edit manually",
		fmt.Sprintf("# Passwordless FULL sudo for group %s (TerminalAdmin, server #70)", groupName),
		"",
		terminalAdminDefaultsBlock(groupName),
		"",
		fmt.Sprintf("%%%s ALL=(ALL:ALL) NOPASSWD: ALL", groupName),
	}
	return strings.Join(lines, "\n") + "\n"
}

func generateFullSudoConfig(groupName string) string {
	lines := []string{
		"# Managed by Cadestro - do not edit manually",
		fmt.Sprintf("# Full sudo access for group %s (password required)", groupName),
		fmt.Sprintf("%%%s ALL=(ALL:ALL) ALL", groupName),
	}
	return strings.Join(lines, "\n") + "\n"
}

func generateLimitedSudoConfig(groupName string) string {
	lines := []string{
		"# Managed by Cadestro - do not edit manually",
		fmt.Sprintf("# Limited sudo access for group %s (password required)", groupName),
		"",
		"# Package management",
		fmt.Sprintf("%%%s ALL=(ALL) /usr/bin/apt, /usr/bin/apt-get, /usr/bin/apt-cache, /usr/bin/dpkg", groupName),
		fmt.Sprintf("%%%s ALL=(ALL) /usr/bin/dnf, /usr/bin/yum, /usr/bin/rpm", groupName),
		fmt.Sprintf("%%%s ALL=(ALL) /usr/bin/pacman", groupName),
		fmt.Sprintf("%%%s ALL=(ALL) /usr/bin/zypper", groupName),
		fmt.Sprintf("%%%s ALL=(ALL) /usr/bin/flatpak, /usr/bin/snap", groupName),
		fmt.Sprintf("%%%s ALL=(ALL) /usr/bin/nix, /usr/bin/nix-env, /usr/bin/nix-store, /usr/bin/nix-channel", groupName),
		fmt.Sprintf("%%%s ALL=(ALL) /sbin/apk", groupName),
		"",
		"# Service and system management",
		fmt.Sprintf("%%%s ALL=(ALL) /usr/bin/systemctl, /usr/bin/journalctl", groupName),
		fmt.Sprintf("%%%s ALL=(ALL) /usr/sbin/reboot, /usr/sbin/shutdown", groupName),
		fmt.Sprintf("%%%s ALL=(ALL) /usr/bin/timedatectl, /usr/bin/hostnamectl", groupName),
		"",
		"# Network management",
		fmt.Sprintf("%%%s ALL=(ALL) /usr/bin/ip, /usr/bin/nmcli, /usr/bin/networkctl", groupName),
		fmt.Sprintf("%%%s ALL=(ALL) /usr/sbin/ufw, /usr/bin/firewall-cmd", groupName),
		"",
		"# Disk and storage",
		fmt.Sprintf("%%%s ALL=(ALL) /usr/bin/mount, /usr/bin/umount, /usr/sbin/blkid", groupName),
		"",
		"# Containers",
		fmt.Sprintf("%%%s ALL=(ALL) /usr/bin/docker, /usr/bin/podman", groupName),
		"",
		"# Diagnostics",
		fmt.Sprintf("%%%s ALL=(ALL) /usr/bin/dmesg", groupName),
		"",
		"# Deny modifications to cadestrod and sudoers",
		fmt.Sprintf("%%%s ALL=(ALL) !/usr/bin/systemctl * cadestrod*", groupName),
		fmt.Sprintf("%%%s ALL=(ALL) !/usr/bin/visudo, !/usr/sbin/visudo", groupName),
	}
	return strings.Join(lines, "\n") + "\n"
}

func generateCustomSudoConfig(groupName, customConfig string) string {

	resolved := strings.ReplaceAll(customConfig, "{group}", groupName)
	lines := []string{
		"# Managed by Cadestro - do not edit manually",
		fmt.Sprintf("# Custom sudo access for group %s", groupName),
		"",
		resolved,
	}
	return strings.Join(lines, "\n") + "\n"
}

func (e *Executor) addUserToGroup(ctx context.Context, username, groupName string) error {
	return e.deps.user.AddToGroup(ctx, username, groupName)
}

func (e *Executor) removeUserFromGroup(ctx context.Context, username, groupName string) error {
	return e.deps.user.RemoveFromGroup(ctx, username, groupName)
}

func (e *Executor) getGroupMembers(ctx context.Context, groupName string) []string {
	members, _ := e.deps.user.GroupMembers(ctx, groupName)
	return members
}

func (e *Executor) userInGroup(ctx context.Context, username, groupName string) bool {
	members, _ := e.deps.user.GroupMembers(ctx, groupName)
	return slices.Contains(members, username)
}

func (e *Executor) sudoGroupMembersMatch(ctx context.Context, groupName string, desiredUsers []string) bool {
	members, _ := e.deps.user.GroupMembers(ctx, groupName)
	return sysuser.MembersMatch(members, desiredUsers)
}
