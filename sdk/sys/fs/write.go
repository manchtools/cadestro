package fs

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// WriteFile writes data to path atomically, applying opts (mode/ownership, and
// an optional backup of the prior contents).
//
// When the Runner's backend is Direct — the deployed root agent — the write
// takes the TOCTOU-safe, fd-anchored path: a random-suffix same-directory temp
// opened O_NOFOLLOW, fchmod'd, fsync'd, renamed into place, then chowned through
// an O_NOFOLLOW fd. This closes the root arbitrary-file-write privesc class
// (WS6 #2): a predictable, symlink-followable temp could otherwise let a local
// attacker redirect the root agent's write to an arbitrary file.
//
// Under Sudo/Doas (a non-root caller, e.g. CI or a dev tool) the escalated path
// is used: it cannot openat as root, so it is also made symlink-safe by refusing
// any target whose parent directory a non-root user could write to (the only
// place a symlink could be planted) and then writing atomically in a single root
// shell — mktemp + write + `mv -T` over the target (a rename replaces a symlinked
// target, never follows it). See writeEscalated.
func (m *manager) WriteFile(ctx context.Context, path string, data []byte, opts WriteOptions) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := ValidatePath(path); err != nil {
		return err
	}
	if err := validateMode(opts.Mode); err != nil {
		return err
	}
	if opts.Backup != "" {
		if err := ValidatePath(opts.Backup); err != nil {
			return err
		}

		if filepath.Clean(opts.Backup) == filepath.Clean(path) {
			return fmt.Errorf("%w: backup path must differ from the target path", ErrInvalidPath)
		}
	}
	if m.direct() {
		return writeDirect(path, data, opts)
	}
	return m.writeEscalated(ctx, path, data, opts)
}

// WriteFileExclusive writes data to path only when path does not already exist,
// returning ErrExists otherwise. See the Manager interface for why this exists
// as a primitive rather than as Exists-then-WriteFile.
//
// Backup is rejected: a backup only makes sense when replacing existing content,
// and this call by construction never replaces anything.
func (m *manager) WriteFileExclusive(ctx context.Context, path string, data []byte, opts WriteOptions) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := ValidatePath(path); err != nil {
		return err
	}
	if err := validateMode(opts.Mode); err != nil {
		return err
	}
	if opts.Backup != "" {
		return fmt.Errorf("%w: WriteFileExclusive never replaces existing content, so a backup path is meaningless", ErrInvalidPath)
	}
	if m.direct() {
		return writeExclusiveDirect(path, data, opts)
	}
	return m.writeExclusiveEscalated(ctx, path, data, opts)
}

func writeExclusiveDirect(path string, data []byte, opts WriteOptions) error {
	perm := opts.Mode
	if perm == 0 {
		perm = 0o644
	}
	if err := safeReplaceFile(path, data, perm, false); err != nil {
		if errors.Is(err, ErrExists) {
			return fmt.Errorf("write file %s: %w", path, ErrExists)
		}
		return fmt.Errorf("write file %s: %w", path, err)
	}
	if opts.Owner != "" || opts.Group != "" {
		uid, gid, err := ResolveOwnership(opts.Owner, opts.Group)
		if err != nil {
			return err
		}
		if err := FchownNoFollow(path, uid, gid); err != nil {
			return fmt.Errorf("set ownership on %s: %w", path, err)
		}
	}
	return nil
}

func (m *manager) writeExclusiveEscalated(ctx context.Context, path string, data []byte, opts WriteOptions) error {
	perm := opts.Mode
	if perm == 0 {
		perm = 0o644
	}
	if err := escalatedParentSafe(filepath.Dir(path)); err != nil {
		return err
	}
	res, err := m.runPrivStdin(ctx, string(data), "sh", "-c", escalatedWriteExclusiveScript,
		"sh", path, modeArg(perm), Ownership(opts.Owner, opts.Group))
	if err != nil {
		return fmt.Errorf("write file %s: %w", path, err)
	}
	if res.ExitCode == exclusiveExistsExit {
		return fmt.Errorf("write file %s: %w", path, ErrExists)
	}
	if cerr := cmdError("write file", res); cerr != nil {
		return fmt.Errorf("write file %s: %w", path, cerr)
	}
	return nil
}

const exclusiveExistsExit = 3

const escalatedWriteExclusiveScript = `set -eu
target=$1; mode=$2; owner=$3
dir=$(dirname -- "$target")
tmp=$(mktemp "$dir/.cadestro-XXXXXXXXXX")
trap 'rm -f -- "$tmp"' EXIT
cat > "$tmp"
chmod "$mode" -- "$tmp"
if [ -n "$owner" ]; then
	chown "$owner" -- "$tmp"
fi
if ln -- "$tmp" "$target" 2>/dev/null; then
	exit 0
fi
exit 3
`

func writeDirect(path string, data []byte, opts WriteOptions) error {
	perm := opts.Mode
	if perm == 0 {
		perm = 0o644
	}
	if opts.Backup != "" {
		if err := safeBackupAndReplace(path, opts.Backup, data, perm, true); err != nil {
			return fmt.Errorf("write file %s: %w", path, err)
		}
	} else if err := safeReplaceFile(path, data, perm, true); err != nil {
		return fmt.Errorf("write file %s: %w", path, err)
	}
	if opts.Owner != "" || opts.Group != "" {
		uid, gid, err := ResolveOwnership(opts.Owner, opts.Group)
		if err != nil {
			return err
		}
		if err := FchownNoFollow(path, uid, gid); err != nil {
			return fmt.Errorf("set ownership on %s: %w", path, err)
		}
	}
	return nil
}

func (m *manager) writeEscalated(ctx context.Context, path string, data []byte, opts WriteOptions) error {
	perm := opts.Mode
	if perm == 0 {
		perm = 0o644
	}
	if err := escalatedParentSafe(filepath.Dir(path)); err != nil {
		return err
	}
	res, err := m.runPrivStdin(ctx, string(data), "sh", "-c", escalatedWriteScript,
		"sh", path, modeArg(perm), Ownership(opts.Owner, opts.Group), opts.Backup)
	if err != nil {
		return fmt.Errorf("write file %s: %w", path, err)
	}
	if cerr := cmdError("write file", res); cerr != nil {
		return fmt.Errorf("write file %s: %w", path, cerr)
	}
	return nil
}

const escalatedWriteScript = `set -eu
target=$1; mode=$2; owner=$3; backup=$4
dir=$(dirname -- "$target")
if [ -n "$backup" ] && [ -e "$target" ]; then
	cp -f -- "$target" "$backup"
fi
tmp=$(mktemp "$dir/.cadestro-XXXXXXXXXX")
trap 'rm -f -- "$tmp"' EXIT
cat > "$tmp"
chmod "$mode" -- "$tmp"
if [ -n "$owner" ]; then
	chown "$owner" -- "$tmp"
fi
mv -T -- "$tmp" "$target"
trap - EXIT
`

func (m *manager) runChecked(ctx context.Context, name string, args ...string) error {
	res, err := m.runPriv(ctx, name, args...)
	if err != nil {
		return err
	}
	return cmdError(name, res)
}

// Copy copies src to dst (plain cp, no -p) and applies opts to dst. opts.Mode of
// 0 leaves cp's default destination mode (the source mode with the process umask
// applied) in place; set opts.Mode to fix the mode explicitly. This differs from
// WriteFile, which defaults a zero mode to 0644.
func (m *manager) Copy(ctx context.Context, src, dst string, opts WriteOptions) error {
	if err := ValidatePath(src); err != nil {
		return err
	}
	if err := ValidatePath(dst); err != nil {
		return err
	}
	if err := validateMode(opts.Mode); err != nil {
		return err
	}
	if err := m.runChecked(ctx, "cp", "--", src, dst); err != nil {
		return fmt.Errorf("copy file: %w", err)
	}
	if opts.Mode != 0 {
		if err := m.SetMode(ctx, dst, opts.Mode); err != nil {
			return err
		}
	}
	if opts.Owner != "" || opts.Group != "" {
		if err := m.SetOwnership(ctx, dst, opts.Owner, opts.Group); err != nil {
			return err
		}
	}
	return nil
}

// CopyTree recursively copies the tree at src to dst, preserving mode, ownership,
// and timestamps (cp -a), and merges into dst rather than nesting under it: it
// runs `cp -a -T -- src dst`, where -T (--no-target-directory) makes cp treat dst
// as the literal destination. So `CopyTree(ctx, "/etc/skel", "/home/alice", …)`
// makes /home/alice a copy of skel's contents whether or not /home/alice already
// exists — never /home/alice/skel. cp -a OVERWRITES files that already exist at
// dst (it does not delete dst-only files); a caller that must not clobber existing
// content (e.g. a user's customised dotfiles) checks Exists first.
//
// opts applies to dst AFTER the copy: a non-zero Mode chmods the destination ROOT
// only (not recursively — the per-file modes from the archive copy are kept),
// while Owner/Group, if set, are applied RECURSIVELY (the common intent when
// re-homing a tree copied as root — e.g. skel → a user's home). Both are skipped
// when unset, leaving the archive-preserved metadata.
func (m *manager) CopyTree(ctx context.Context, src, dst string, opts WriteOptions) error {
	if err := ValidatePath(src); err != nil {
		return err
	}
	if err := ValidatePath(dst); err != nil {
		return err
	}
	if err := validateMode(opts.Mode); err != nil {
		return err
	}
	if err := m.runChecked(ctx, "cp", "-a", "-T", "--", src, dst); err != nil {
		return fmt.Errorf("copy tree: %w", err)
	}
	if opts.Mode != 0 {
		if err := m.SetMode(ctx, dst, opts.Mode); err != nil {
			return err
		}
	}
	if opts.Owner != "" || opts.Group != "" {
		if err := m.SetOwnershipRecursive(ctx, dst, opts.Owner, opts.Group); err != nil {
			return err
		}
	}
	return nil
}

// SetMode sets the file mode (chmod). The mode is applied exactly as given
// (a zero mode means 0000); callers that want a default for a fresh file use
// WriteOptions.Mode, which defaults 0 to 0644.
func (m *manager) SetMode(ctx context.Context, path string, mode os.FileMode) error {
	if err := ValidatePath(path); err != nil {
		return err
	}
	if err := validateMode(mode); err != nil {
		return err
	}
	return m.runChecked(ctx, "chmod", modeArg(mode), "--", path)
}

// SetOwnership sets the file owner and group (chown). Both empty is a no-op.
func (m *manager) SetOwnership(ctx context.Context, path, owner, group string) error {
	ownership := Ownership(owner, group)
	if ownership == "" {
		return nil
	}
	if err := ValidatePath(path); err != nil {
		return err
	}
	return m.runChecked(ctx, "chown", "--", ownership, path)
}

// SetOwnershipRecursive changes ownership of a path and all its contents
// (chown -R). Both empty is a no-op. The `--` separator and ValidatePath both
// refuse an ownership or path value that begins with `-`.
func (m *manager) SetOwnershipRecursive(ctx context.Context, path, owner, group string) error {
	ownership := Ownership(owner, group)
	if ownership == "" {
		return nil
	}
	if err := ValidatePath(path); err != nil {
		return err
	}

	if IsProtectedPath(path) {
		return fmt.Errorf("%w: %s", ErrProtectedTarget, filepath.Clean(path))
	}
	return m.runChecked(ctx, "chown", "-R", "--", ownership, path)
}
