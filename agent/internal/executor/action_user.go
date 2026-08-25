package executor

import (
	"context"
	"fmt"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"

	pb "github.com/manchtools/cadestro/contract/gen/go/cadestro/v1"
	sysfs "github.com/manchtools/cadestro/sdk/sys/fs"
	sysuser "github.com/manchtools/cadestro/sdk/sys/user"
)

func (e *Executor) executeUser(ctx context.Context, params *pb.UserParams, state pb.DesiredState, actionID string) (*pb.CommandOutput, bool, map[string]string, error) {
	e.ensureDeps()
	if params == nil {
		return nil, false, nil, fmt.Errorf("user params required")
	}

	if params.Username == "" {
		return nil, false, nil, fmt.Errorf("username is required")
	}

	if !sysuser.IsValidName(params.Username) {
		return nil, false, nil, fmt.Errorf("invalid username: must be 1-32 alphanumeric characters, starting with a letter")
	}

	if params.HomeDir != "" {
		if !filepath.IsAbs(params.HomeDir) {
			return nil, false, nil, fmt.Errorf("home directory must be an absolute path")
		}
		if isProtectedPath(params.HomeDir) {
			return nil, false, nil, fmt.Errorf("home directory %q is a protected system path", params.HomeDir)
		}
	}

	if out, err := e.requireWritableFS(ctx); err != nil {
		return out, false, nil, err
	}

	switch state {
	case pb.DesiredState_DESIRED_STATE_PRESENT:
		return e.createOrUpdateUser(ctx, params, actionID)
	case pb.DesiredState_DESIRED_STATE_ABSENT:
		output, changed, err := e.removeUser(ctx, params.Username)
		return output, changed, nil, err
	default:
		return nil, false, nil, fmt.Errorf("unknown desired state: %v", state)
	}
}

func homeGroupFor(params *pb.UserParams) string {
	if params.Gid > 0 {
		return fmt.Sprintf("%d", params.Gid)
	}
	if params.PrimaryGroup != "" {
		return params.PrimaryGroup
	}
	return params.Username
}

func (e *Executor) ensureHomeIfMissing(ctx context.Context, params *pb.UserParams, currentHome string, output *strings.Builder) bool {
	if !params.CreateHome {
		return false
	}
	homeDir := params.HomeDir
	if homeDir == "" {
		homeDir = currentHome
	}
	if homeDir == "" {
		homeDir = "/home/" + params.Username
	}
	ok, err := e.deps.fs.Exists(ctx, homeDir)
	if err != nil {
		output.WriteString(fmt.Sprintf("warning: could not check home directory %s: %v\n", homeDir, err))
		return false
	}
	if ok {
		return false
	}

	if hErr := e.deps.user.EnsureHome(ctx, params.Username, sysuser.EnsureHomeOptions{Group: homeGroupFor(params), Mode: 0o700}); hErr != nil {
		output.WriteString(fmt.Sprintf("warning: failed to create home directory: %v\n", hErr))
		return false
	}
	output.WriteString(fmt.Sprintf("created missing home directory: %s\n", homeDir))
	return true
}

func (e *Executor) createOrUpdateUser(ctx context.Context, params *pb.UserParams, actionID string) (*pb.CommandOutput, bool, map[string]string, error) {
	var output strings.Builder
	exists, err := e.userExists(ctx, params.Username)
	if err != nil {
		return nil, false, nil, fmt.Errorf("check user %s: %w", params.Username, err)
	}

	if exists {

		cmdOutput, changed, err := e.updateUser(ctx, params, &output)
		return cmdOutput, changed, nil, err
	}

	cmdOutput, metadata, err := e.createUser(ctx, params, actionID, &output)
	return cmdOutput, true, metadata, err
}

func (e *Executor) createUser(ctx context.Context, params *pb.UserParams, actionID string, output *strings.Builder) (*pb.CommandOutput, map[string]string, error) {

	shell := params.Shell
	if shell == "" {
		if params.Disabled || params.SystemUser {
			shell = "/usr/sbin/nologin"
		} else {
			shell = "/bin/bash"
		}
	}

	opts := sysuser.CreateOptions{
		Shell:      shell,
		HomeDir:    params.HomeDir,
		Comment:    params.Comment,
		System:     params.SystemUser,
		CreateHome: params.CreateHome,
	}
	if params.Uid > 0 {
		opts.UID = int(params.Uid)
	}

	if params.Gid > 0 {
		opts.PrimaryGroup = fmt.Sprintf("%d", params.Gid)
	} else if params.PrimaryGroup != "" {
		if err := e.deps.user.GroupEnsure(ctx, params.PrimaryGroup); err != nil {
			e.logger.Warn("failed to ensure primary group exists", "group", params.PrimaryGroup, "error", err)
		}
		opts.PrimaryGroup = params.PrimaryGroup
	}

	if err := e.deps.user.Create(ctx, params.Username, opts); err != nil {
		output.WriteString(err.Error())
		return &pb.CommandOutput{ExitCode: 1, Stderr: output.String()}, nil, fmt.Errorf("failed to create user: %w", err)
	}
	output.WriteString(fmt.Sprintf("created user: %s\n", params.Username))

	var metadata map[string]string
	if createUserSetsPassword(params) {
		tempPassword, err := sysuser.GeneratePassword(16, sysuser.ComplexityAlphanumeric)
		if err != nil {
			output.WriteString(fmt.Sprintf("warning: failed to generate temporary password: %v\n", err))
		} else {

			if chpasswdErr := e.deps.user.SetPassword(ctx, params.Username, tempPassword); chpasswdErr != nil {
				output.WriteString(fmt.Sprintf("warning: failed to set temporary password: %v\n", chpasswdErr))
			} else {

				if chageErr := e.deps.user.ExpirePassword(ctx, params.Username); chageErr != nil {
					output.WriteString(fmt.Sprintf("warning: failed to expire password: %v\n", chageErr))
				}
				output.WriteString(fmt.Sprintf("temporary password set for %s (must be changed on first login)\n", params.Username))

				e.reportUserCreatePassword(ctx, params.Username, actionID, tempPassword.Reveal(), output)
			}
		}
	}

	if len(params.SshAuthorizedKeys) > 0 {
		if _, err := e.setupSSHKeys(ctx, params, output); err != nil {
			return nil, nil, fmt.Errorf("setup SSH keys: %w", err)
		}
	}

	if desiredAccountLocked(params) {
		if lockErr := e.deps.user.Lock(ctx, params.Username); lockErr != nil {
			output.WriteString(fmt.Sprintf("warning: failed to lock user account: %v\n", lockErr))
		} else {
			output.WriteString("account locked (disabled)\n")
		}
	} else if unlockErr := e.deps.user.Unlock(ctx, params.Username); unlockErr != nil {
		output.WriteString(fmt.Sprintf("warning: failed to unlock user account: %v\n", unlockErr))
	}

	if params.Hidden {
		e.setUserHidden(ctx, params.Username, true, output)
	}

	return &pb.CommandOutput{ExitCode: 0, Stdout: output.String()}, metadata, nil
}

func createUserSetsPassword(params *pb.UserParams) bool {
	return !params.NoPassword && !params.SystemUser && !params.Disabled
}

func desiredAccountLocked(params *pb.UserParams) bool {
	return params.Disabled
}

func (e *Executor) updateUser(ctx context.Context, params *pb.UserParams, output *strings.Builder) (*pb.CommandOutput, bool, error) {

	currentInfo, err := e.deps.user.Get(ctx, params.Username)
	if err != nil {
		return nil, false, fmt.Errorf("failed to get current user info: %w", err)
	}

	changed := false
	var modOpts sysuser.ModifyOptions
	needModify := false

	desiredShell := params.Shell
	if desiredShell == "" {

		if params.Disabled && currentInfo.UID != 0 {
			desiredShell = "/usr/sbin/nologin"
		}

	}

	if desiredShell != "" && currentInfo.Shell != desiredShell {
		modOpts.Shell = desiredShell
		needModify = true
		output.WriteString(fmt.Sprintf("shell: %s -> %s\n", currentInfo.Shell, desiredShell))
	}

	if params.HomeDir != "" && currentInfo.HomeDir != params.HomeDir {
		modOpts.HomeDir = params.HomeDir
		needModify = true
		output.WriteString(fmt.Sprintf("home: %s -> %s\n", currentInfo.HomeDir, params.HomeDir))
	}

	if params.Comment != "" && currentInfo.Comment != params.Comment {
		modOpts.Comment = params.Comment
		needModify = true
		output.WriteString(fmt.Sprintf("comment: %s -> %s\n", currentInfo.Comment, params.Comment))
	}

	if params.Gid > 0 && currentInfo.GID != int(params.Gid) {
		modOpts.PrimaryGroup = fmt.Sprintf("%d", params.Gid)
		needModify = true
		output.WriteString(fmt.Sprintf("gid: %d -> %d\n", currentInfo.GID, params.Gid))
	} else if params.PrimaryGroup != "" {
		if err := e.deps.user.GroupEnsure(ctx, params.PrimaryGroup); err != nil {
			e.logger.Warn("failed to ensure primary group exists for usermod", "group", params.PrimaryGroup, "error", err)
		}

		if grp, err := user.LookupGroup(params.PrimaryGroup); err != nil || grp.Gid != strconv.Itoa(currentInfo.GID) {
			modOpts.PrimaryGroup = params.PrimaryGroup
			needModify = true
			output.WriteString(fmt.Sprintf("primary group -> %s\n", params.PrimaryGroup))
		}
	}

	if needModify {
		if err := e.deps.user.Modify(ctx, params.Username, modOpts); err != nil {
			output.WriteString(err.Error())
			return &pb.CommandOutput{ExitCode: 1, Stderr: output.String()}, false, fmt.Errorf("failed to update user: %w", err)
		}
		changed = true
	}

	if e.ensureHomeIfMissing(ctx, params, currentInfo.HomeDir, output) {
		changed = true
	}

	desiredLocked := desiredAccountLocked(params)
	if desiredLocked != currentInfo.Locked {
		if desiredLocked {
			if err := e.deps.user.Lock(ctx, params.Username); err != nil {
				output.WriteString(fmt.Sprintf("warning: failed to lock user: %v\n", err))
			} else {
				if currentInfo.UID == 0 {

					e.logger.Warn("locked the superuser account per USER action (password login disabled; sudo/key-SSH unaffected)", "username", params.Username)
				}
				output.WriteString("account locked (disabled)\n")
				changed = true
			}
		} else {
			if err := e.deps.user.Unlock(ctx, params.Username); err != nil {
				output.WriteString(fmt.Sprintf("warning: failed to unlock user: %v\n", err))
			} else {
				output.WriteString("account unlocked\n")
				changed = true
			}
		}
	}

	if len(params.SshAuthorizedKeys) > 0 {
		if keysChanged, err := e.setupSSHKeys(ctx, params, output); err != nil {
			return nil, changed, fmt.Errorf("setup SSH keys: %w", err)
		} else if keysChanged {
			changed = true
		}
	}

	if e.setUserHidden(ctx, params.Username, params.Hidden, output) {
		changed = true
	}

	if !changed {
		output.WriteString(fmt.Sprintf("user %s is already in desired state\n", params.Username))
	}

	return &pb.CommandOutput{ExitCode: 0, Stdout: output.String()}, changed, nil
}

func (e *Executor) removeUser(ctx context.Context, username string) (*pb.CommandOutput, bool, error) {
	uExists, err := e.userExists(ctx, username)
	if err != nil {
		return nil, false, fmt.Errorf("check user %s: %w", username, err)
	}
	if !uExists {

		return &pb.CommandOutput{
			ExitCode: 0,
			Stdout:   fmt.Sprintf("user %s does not exist, nothing to remove\n", username),
		}, false, nil
	}

	e.killUserSessions(ctx, username)

	e.removeAccountsServiceFile(ctx, username)

	err = e.deps.user.Delete(ctx, username, sysuser.DeleteOptions{RemoveHome: true})
	if err != nil {

		exists, existsErr := e.deps.user.Exists(ctx, username)
		if existsErr == nil && !exists {
			return &pb.CommandOutput{
				ExitCode: 0,
				Stdout:   fmt.Sprintf("removed user: %s (home directory may not have existed)\n", username),
			}, true, nil
		}
		return nil, false, fmt.Errorf("failed to remove user: %w", err)
	}

	return &pb.CommandOutput{
		ExitCode: 0,
		Stdout:   fmt.Sprintf("removed user: %s\n", username),
	}, true, nil
}

const accountsServiceDir = "/var/lib/AccountsService/users"

const accountsServiceHiddenContent = "[User]\nSystemAccount=true\n"

func (e *Executor) setUserHidden(ctx context.Context, username string, hidden bool, output *strings.Builder) bool {
	if !e.fileExistsWithSudo(ctx, accountsServiceDir) {
		return false
	}

	existing, _ := e.readFileWithSudo(ctx, accountsServiceDir+"/"+username)
	if (existing == accountsServiceHiddenContent) == hidden {
		return false
	}

	if err := e.deps.user.SetHiddenOnLoginScreen(ctx, username, hidden); err != nil {
		verb := "hide"
		if !hidden {
			verb = "unhide"
		}
		output.WriteString(fmt.Sprintf("warning: failed to %s user on login screen: %v\n", verb, err))
		return false
	}
	if hidden {
		output.WriteString("hidden from login screen (AccountsService)\n")
	} else {
		output.WriteString("visible on login screen (AccountsService removed)\n")
	}
	return true
}

func (e *Executor) removeAccountsServiceFile(ctx context.Context, username string) {
	_ = e.deps.user.SetHiddenOnLoginScreen(ctx, username, false)
}

func (e *Executor) setupSSHKeys(ctx context.Context, params *pb.UserParams, output *strings.Builder) (bool, error) {

	homeDir := params.HomeDir
	if homeDir == "" {
		if params.SystemUser {
			homeDir = "/"
		} else {
			homeDir = filepath.Join("/home", params.Username)
		}
	}

	sshDir := filepath.Join(homeDir, ".ssh")
	authKeysFile := filepath.Join(sshDir, "authorized_keys")

	var keysContent strings.Builder
	validKeyCount := 0
	for i, key := range params.SshAuthorizedKeys {
		trimmedKey := strings.TrimSpace(key)
		if trimmedKey == "" {
			continue
		}

		if strings.ContainsAny(trimmedKey, "\n\r") {
			return false, fmt.Errorf("authorized_keys entry contains embedded newline (input index %d for user %s); refusing to splice into file", i, params.Username)
		}
		if !strings.HasPrefix(trimmedKey, "ssh-") && !strings.HasPrefix(trimmedKey, "ecdsa-") {
			output.WriteString(fmt.Sprintf("warning: skipping invalid SSH key (doesn't start with ssh- or ecdsa-): %s...\n", trimmedKey[:min(30, len(trimmedKey))]))
			continue
		}
		keysContent.WriteString(trimmedKey)
		keysContent.WriteString("\n")
		validKeyCount++
	}
	desiredContent := keysContent.String()

	existing, _ := e.readFileWithSudo(ctx, authKeysFile)
	if existing == desiredContent {
		return false, nil
	}

	if err := e.deps.fs.Mkdir(ctx, sshDir, sysfs.MkdirOptions{Recursive: true}); err != nil {
		return false, fmt.Errorf("failed to create .ssh directory: %w", err)
	}

	sshFd, err := sysfs.OpenRealDir(sshDir)
	if err != nil {
		return false, fmt.Errorf("refusing to configure SSH keys: %w", err)
	}
	defer sshFd.Close()

	uid, gid, err := sysfs.ResolveOwnership(params.Username, homeGroupFor(params))
	if err != nil {
		return false, fmt.Errorf("resolve .ssh ownership: %w", err)
	}
	if err := sshFd.Chown(uid, gid); err != nil {
		return false, fmt.Errorf("failed to set .ssh ownership: %w", err)
	}
	if err := sshFd.Chmod(0o700); err != nil {
		return false, fmt.Errorf("failed to set .ssh permissions: %w", err)
	}

	if err := e.deps.fs.WriteFile(ctx, authKeysFile, []byte(desiredContent), sysfs.WriteOptions{Mode: 0o600}); err != nil {
		return false, fmt.Errorf("failed to write authorized_keys: %w", err)
	}

	if err := sysfs.FchownNoFollow(authKeysFile, uid, gid); err != nil {
		return false, fmt.Errorf("failed to set authorized_keys ownership: %w", err)
	}

	output.WriteString(fmt.Sprintf("configured %d SSH authorized key(s)\n", validKeyCount))
	return true, nil
}

func (e *Executor) reloadSshd(ctx context.Context, output *strings.Builder) {

	err := e.deps.service.Reload(ctx, "sshd")
	if err != nil {
		err = e.deps.service.Reload(ctx, "ssh")
	}
	if err != nil {
		output.WriteString("warning: failed to reload sshd\n")
		output.WriteString(strings.TrimSpace(err.Error()) + "\n")
	} else {
		output.WriteString("reloaded sshd\n")
	}
}
