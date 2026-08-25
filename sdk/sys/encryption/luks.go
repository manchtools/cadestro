package encryption

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"sync"
	"syscall"

	"github.com/manchtools/cadestro/sdk/sys/exec"
)

type luks struct {
	r exec.Runner
}

// LUKS2 (and LUKS1) support eight keyslots, 0..7. Rejecting out-of-range slots
// at the SDK boundary surfaces a clear reason instead of cryptsetup's opaque one.
const (
	LuksMinKeySlot = 0
	LuksMaxKeySlot = 7
)

// ErrInvalidKeySlot is returned for a slot index outside [LuksMinKeySlot, LuksMaxKeySlot].
var ErrInvalidKeySlot = errors.New("invalid LUKS keyslot")

// ErrEmptyKeyMaterial is returned by a mutating LUKS/TPM operation given an
// empty passphrase/key. It is refused before any cryptsetup/cryptenroll exec:
// an empty NEW key would enroll a slot that unlocks with no passphrase, and an
// empty AUTHENTICATING key for a mutating op is never a legitimate request.
// (VerifyPassphrase is intentionally exempt — probing an empty passphrase is a
// legitimate read-only query.)
var ErrEmptyKeyMaterial = errors.New("encryption: empty key material not permitted")

func validateKeySlot(slot int) error {
	if slot < LuksMinKeySlot || slot > LuksMaxKeySlot {
		return fmt.Errorf("%w: slot %d outside valid range %d..%d", ErrInvalidKeySlot, slot, LuksMinKeySlot, LuksMaxKeySlot)
	}
	return nil
}

// IsEncrypted reports whether dev is a LUKS volume.
func (l *luks) IsEncrypted(ctx context.Context, dev string) (bool, error) {
	if err := validateDevicePath(dev); err != nil {
		return false, err
	}
	res, err := l.r.Run(ctx, exec.Command{Name: "cryptsetup", Args: []string{"isLuks", dev}, Escalate: true})
	if err != nil {
		return false, fmt.Errorf("cryptsetup isLuks: %w", err)
	}
	switch res.ExitCode {
	case 0:
		return true, nil
	case 1:
		return false, nil
	default:
		return false, cryptsetupError("isLuks", res)
	}
}

// AddKey adds newKey to a LUKS volume, authenticating with existing. With
// opts.Slot nil cryptsetup auto-assigns a free slot; otherwise the given slot
// (0..7) is targeted.
func (l *luks) AddKey(ctx context.Context, dev string, existing, newKey exec.Secret, opts AddKeyOptions) error {
	if err := validateDevicePath(dev); err != nil {
		return err
	}
	if opts.Slot != nil {
		if err := validateKeySlot(*opts.Slot); err != nil {
			return err
		}
	}

	if newKey.IsZero() {
		return fmt.Errorf("%w: refusing to add an empty new key (would create an empty-passphrase unlock slot)", ErrEmptyKeyMaterial)
	}
	if existing.IsZero() {
		return fmt.Errorf("%w: empty authenticating passphrase", ErrEmptyKeyMaterial)
	}
	existingFile, err := writeKeyFile(existing)
	if err != nil {
		return err
	}
	defer cleanupKeyFile(existingFile.path)
	newFile, err := writeKeyFile(newKey)
	if err != nil {
		return err
	}
	defer cleanupKeyFile(newFile.path)

	args := []string{"luksAddKey", dev, newFile.path, "--key-file", existingFile.path}
	op := "luksAddKey"
	if opts.Slot != nil {
		args = append(args, "--key-slot", strconv.Itoa(*opts.Slot))
		op = fmt.Sprintf("luksAddKey (slot %d)", *opts.Slot)
	}
	args = append(args, "--batch-mode")
	return l.runCryptsetup(ctx, op, args, existingFile, newFile)
}

// RemoveKey removes a passphrase from a LUKS volume.
func (l *luks) RemoveKey(ctx context.Context, dev string, key exec.Secret) error {
	if err := validateDevicePath(dev); err != nil {
		return err
	}
	if key.IsZero() {
		return fmt.Errorf("%w: empty passphrase", ErrEmptyKeyMaterial)
	}
	keyFile, err := writeKeyFile(key)
	if err != nil {
		return err
	}
	defer cleanupKeyFile(keyFile.path)
	return l.runCryptsetup(ctx, "luksRemoveKey",
		[]string{"luksRemoveKey", dev, "--key-file", keyFile.path, "--batch-mode"}, keyFile)
}

// KillSlot removes a specific keyslot, authenticating with existing.
func (l *luks) KillSlot(ctx context.Context, dev string, slot int, existing exec.Secret) error {
	if err := validateDevicePath(dev); err != nil {
		return err
	}
	if err := validateKeySlot(slot); err != nil {
		return err
	}
	if existing.IsZero() {
		return fmt.Errorf("%w: empty authenticating passphrase", ErrEmptyKeyMaterial)
	}
	keyFile, err := writeKeyFile(existing)
	if err != nil {
		return err
	}
	defer cleanupKeyFile(keyFile.path)
	return l.runCryptsetup(ctx, fmt.Sprintf("luksKillSlot %d", slot),
		[]string{"luksKillSlot", dev, strconv.Itoa(slot), "--key-file", keyFile.path, "--batch-mode"}, keyFile)
}

// VerifyPassphrase reports whether p unlocks dev, without unlocking it.
func (l *luks) VerifyPassphrase(ctx context.Context, dev string, p exec.Secret) (bool, error) {
	if err := validateDevicePath(dev); err != nil {
		return false, err
	}
	keyFile, err := writeKeyFile(p)
	if err != nil {
		return false, err
	}
	defer cleanupKeyFile(keyFile.path)
	if err := verifyStaged(keyFile); err != nil {
		return false, err
	}

	res, err := l.r.Run(ctx, exec.Command{
		Name:     "cryptsetup",
		Args:     []string{"open", "--test-passphrase", dev, "--key-file", keyFile.path, "--batch-mode"},
		Escalate: true,
	})
	if err != nil {
		return false, fmt.Errorf("cryptsetup test-passphrase: %w", err)
	}
	switch res.ExitCode {
	case 0:
		return true, nil
	case 2:
		return false, nil
	default:
		return false, cryptsetupError("test-passphrase", res)
	}
}

func (l *luks) TPM() (TPMEnroller, bool) {
	return &tpmEnroller{r: l.r}, true
}

func (l *luks) runCryptsetup(ctx context.Context, op string, args []string, staged ...stagedKeyFile) error {
	if err := verifyStaged(staged...); err != nil {
		return fmt.Errorf("cryptsetup %s: %w", op, err)
	}
	res, err := l.r.Run(ctx, exec.Command{Name: "cryptsetup", Args: args, Escalate: true})
	if err != nil {
		return fmt.Errorf("cryptsetup %s: %w", op, err)
	}
	if res.ExitCode != 0 {
		return cryptsetupError(op, res)
	}
	return nil
}

func cryptsetupError(op string, res exec.Result) error {
	detail := exitCodeDetail(res.ExitCode)
	if s := strings.TrimSpace(res.Stderr); s != "" {
		detail = s
	}
	slog.Warn("cryptsetup command failed", "command", op, "exit_code", res.ExitCode, "detail", detail)
	return fmt.Errorf("cryptsetup %s failed: %s (exit code %d)", op, detail, res.ExitCode)
}

func exitCodeDetail(code int) string {
	switch code {
	case 1:
		return "wrong parameters"
	case 2:
		return "no key available with this passphrase"
	case 3:
		return "out of memory"
	case 4:
		return "wrong device specified or device does not exist"
	case 5:
		return "device already exists or device is busy"
	default:
		return fmt.Sprintf("unexpected error (exit code %d)", code)
	}
}

var keyFileDir = "/dev/shm"

// ErrKeyFileStaging is returned when the private directory key files are
// staged in cannot be established, or fails its owner/mode check. Owning that
// directory is enough to replace a key file regardless of the file's own
// ownership, so anything unexpected about it is a hard failure.
var ErrKeyFileStaging = errors.New("encryption: unsafe LUKS key file staging directory")

// ErrKeyFileTampered is returned when a staged key file no longer names the
// file this process wrote. cryptsetup re-resolves the path it is handed, so a
// swapped entry would enrol a passphrase the attacker chose.
var ErrKeyFileTampered = errors.New("encryption: LUKS key file changed between staging and use")

var (
	stagingMu   sync.Mutex
	stagingRoot string
	stagingDir  string
)

func ownerUID(info os.FileInfo) (int, bool) {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, false
	}
	return int(stat.Uid), true
}

func verifyStagingParent(dir string) error {
	info, err := lstatFile(dir)
	if err != nil {
		return fmt.Errorf("%w: %s: %v", ErrKeyFileStaging, dir, err)
	}
	if !info.Mode().IsDir() {
		return fmt.Errorf("%w: %s is not a directory (mode %s)", ErrKeyFileStaging, dir, info.Mode())
	}
	uid, ok := ownerUID(info)
	if !ok {
		return fmt.Errorf("%w: %s exposes no owning uid on this platform", ErrKeyFileStaging, dir)
	}
	if uid != 0 && uid != geteuid() {
		return fmt.Errorf("%w: %s is owned by uid %d, neither root nor %d", ErrKeyFileStaging, dir, uid, geteuid())
	}
	if info.Mode().Perm()&0o022 != 0 && info.Mode()&os.ModeSticky == 0 {
		return fmt.Errorf("%w: %s is writable beyond its owner without the sticky bit (mode %s)", ErrKeyFileStaging, dir, info.Mode())
	}
	return nil
}

func verifyStagingDir(dir string) error {
	info, err := lstatFile(dir)
	if err != nil {
		return fmt.Errorf("%w: %s: %v", ErrKeyFileStaging, dir, err)
	}
	if !info.Mode().IsDir() {
		return fmt.Errorf("%w: %s is not a directory (mode %s)", ErrKeyFileStaging, dir, info.Mode())
	}
	if perm := info.Mode().Perm(); perm != 0o700 {
		return fmt.Errorf("%w: %s has mode %o, want exactly 0700", ErrKeyFileStaging, dir, perm)
	}
	uid, ok := ownerUID(info)
	if !ok {
		return fmt.Errorf("%w: %s exposes no owning uid on this platform", ErrKeyFileStaging, dir)
	}
	if uid != geteuid() {
		return fmt.Errorf("%w: %s is owned by uid %d, not %d", ErrKeyFileStaging, dir, uid, geteuid())
	}
	return nil
}

func keyFileStagingDir() (string, error) {
	stagingMu.Lock()
	defer stagingMu.Unlock()
	if stagingDir != "" && stagingRoot == keyFileDir {
		if err := verifyStagingDir(stagingDir); err != nil {
			return "", err
		}
		return stagingDir, nil
	}
	if err := verifyStagingParent(keyFileDir); err != nil {
		return "", err
	}
	dir, err := mkdirTemp(keyFileDir, "cadestro-luks-")
	if err != nil {
		return "", fmt.Errorf("%w: create under %s: %v", ErrKeyFileStaging, keyFileDir, err)
	}
	if err := verifyStagingDir(dir); err != nil {
		return "", err
	}
	stagingDir, stagingRoot = dir, keyFileDir
	return dir, nil
}

type stagedKeyFile struct {
	path string
	id   os.FileInfo
}

func (k stagedKeyFile) verify() error {
	if k.path == "" || k.id == nil {
		return fmt.Errorf("%w: %q was never staged", ErrKeyFileTampered, k.path)
	}
	now, err := lstatFile(k.path)
	if err != nil {
		return fmt.Errorf("%w: %s: %v", ErrKeyFileTampered, k.path, err)
	}
	if !now.Mode().IsRegular() || now.Mode().Perm() != 0o600 || !os.SameFile(k.id, now) {
		return fmt.Errorf("%w: %s (mode %s)", ErrKeyFileTampered, k.path, now.Mode())
	}
	return nil
}

func verifyStaged(files ...stagedKeyFile) error {
	for _, f := range files {
		if err := f.verify(); err != nil {
			return err
		}
	}
	return nil
}

type keyFileHandle interface {
	Name() string
	Chmod(os.FileMode) error
	WriteString(string) (int, error)
	Close() error
}

type scrubFile interface {
	Stat() (os.FileInfo, error)
	WriteAt([]byte, int64) (int, error)
	Close() error
}

var (
	mkdirTemp     = os.MkdirTemp
	lstatFile     = os.Lstat
	geteuid       = os.Geteuid
	createKeyFile = func(dir string) (keyFileHandle, error) { return os.CreateTemp(dir, "key-*") }
	removeFile    = os.Remove
	openKeyFile   = func(path string) (scrubFile, error) {

		return os.OpenFile(path, os.O_WRONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	}
)

func cleanupKeyFileAfter(stage, name string, cause error) error {
	if rmErr := removeFile(name); rmErr != nil && !os.IsNotExist(rmErr) {
		return fmt.Errorf("%s: %w (key file cleanup failed, plaintext key may remain at %s: %v)", stage, cause, name, rmErr)
	}
	return fmt.Errorf("%s: %w", stage, cause)
}

func writeKeyFile(key exec.Secret) (stagedKeyFile, error) {
	dir, err := keyFileStagingDir()
	if err != nil {
		return stagedKeyFile{}, err
	}
	f, err := createKeyFile(dir)
	if err != nil {
		return stagedKeyFile{}, fmt.Errorf("create key file: %w", err)
	}
	if err := f.Chmod(0o600); err != nil {
		_ = f.Close()
		return stagedKeyFile{}, cleanupKeyFileAfter("set key file permissions", f.Name(), err)
	}
	if _, err := f.WriteString(key.Reveal()); err != nil {
		_ = f.Close()
		return stagedKeyFile{}, cleanupKeyFileAfter("write key file", f.Name(), err)
	}
	if err := f.Close(); err != nil {
		return stagedKeyFile{}, cleanupKeyFileAfter("close key file", f.Name(), err)
	}

	id, err := lstatFile(f.Name())
	if err != nil {
		return stagedKeyFile{}, cleanupKeyFileAfter("identify key file", f.Name(), err)
	}
	return stagedKeyFile{path: f.Name(), id: id}, nil
}

func cleanupKeyFile(path string) {
	if path == "" {
		return
	}
	f, err := openKeyFile(path)
	if err != nil {
		if rmErr := removeFile(path); rmErr != nil && !os.IsNotExist(rmErr) {
			slog.Warn("luks: removing unscrubbed key file failed", "path", path, "error", rmErr)
		}
		return
	}
	if info, err := f.Stat(); err == nil && !info.Mode().IsRegular() {

		slog.Warn("luks: refusing to scrub a non-regular key file (possible TOCTOU swap)", "path", path, "mode", info.Mode().String())
	} else if err == nil && info.Size() > 0 {
		zeros := make([]byte, info.Size())
		if _, werr := f.WriteAt(zeros, 0); werr != nil {
			slog.Warn("luks: scrubbing key file before unlink failed; passphrase bytes may persist", "path", path, "error", werr)
		}
	}
	if cerr := f.Close(); cerr != nil {
		slog.Warn("luks: closing key file failed", "path", path, "error", cerr)
	}
	if rmErr := removeFile(path); rmErr != nil && !os.IsNotExist(rmErr) {
		slog.Warn("luks: removing key file failed", "path", path, "error", rmErr)
	}
}
