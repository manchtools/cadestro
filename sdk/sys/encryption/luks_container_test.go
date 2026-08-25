//go:build container

package encryption

import (
	"context"
	"os"
	osexec "os/exec"
	"testing"
	"time"

	"github.com/manchtools/cadestro/sdk/sys/exec"
)

const containerCtxTimeout = 60 * time.Second

func requireCryptsetup(t *testing.T) {
	t.Helper()
	if _, err := osexec.LookPath("cryptsetup"); err != nil {
		t.Skip("cryptsetup not on PATH")
	}
}

func directMgr(t *testing.T) Manager {
	t.Helper()
	r, err := exec.NewRunner(exec.Direct)
	if err != nil {
		t.Fatalf("NewRunner(Direct): %v", err)
	}
	return mgr(t, r)
}

func newDevFile(t *testing.T, size int64) string {
	t.Helper()
	f, err := os.CreateTemp("/dev/shm", "cadestro-test-luks-*.img")
	if err != nil {
		t.Fatalf("create /dev/shm device file (need --shm-size headroom?): %v", err)
	}
	name := f.Name()
	if err := f.Truncate(size); err != nil {
		f.Close()
		os.Remove(name)
		t.Fatalf("truncate device file: %v", err)
	}
	f.Close()
	t.Cleanup(func() { os.Remove(name) })
	return name
}

func formatLUKS(t *testing.T, passphrase string) string {
	t.Helper()
	dev := newDevFile(t, 64<<20)
	kf, err := os.CreateTemp(t.TempDir(), "fmt.key")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := kf.WriteString(passphrase); err != nil {
		t.Fatal(err)
	}
	kf.Close()
	cmd := osexec.Command("cryptsetup", "luksFormat", dev, "--key-file", kf.Name(), "--batch-mode")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("setup: cryptsetup luksFormat failed: %v\n%s", err, out)
	}
	return dev
}

func TestIsEncrypted_Container(t *testing.T) {
	requireCryptsetup(t)
	m := directMgr(t)
	ctx, cancel := context.WithTimeout(context.Background(), containerCtxTimeout)
	defer cancel()

	if enc, err := m.IsEncrypted(ctx, formatLUKS(t, "fmt-pass")); err != nil || !enc {
		t.Errorf("IsEncrypted(real LUKS) = (%v, %v); want (true, nil)", enc, err)
	}
	if enc, err := m.IsEncrypted(ctx, newDevFile(t, 16<<20)); err != nil || enc {
		t.Errorf("IsEncrypted(zeros) = (%v, %v); want (false, nil)", enc, err)
	}
	if enc, err := m.IsEncrypted(ctx, "/dev/cadestro-nonexistent-xyz"); err == nil || enc {
		t.Errorf("IsEncrypted(nonexistent) = (%v, %v); want (false, error) — must not fail-open to plaintext", enc, err)
	}
}

func TestVerifyPassphrase_Container(t *testing.T) {
	requireCryptsetup(t)
	m := directMgr(t)
	ctx, cancel := context.WithTimeout(context.Background(), containerCtxTimeout)
	defer cancel()
	dev := formatLUKS(t, "right-pass")

	if ok, err := m.VerifyPassphrase(ctx, dev, mustSecret(t, "right-pass")); err != nil || !ok {
		t.Errorf("VerifyPassphrase(correct) = (%v, %v); want (true, nil)", ok, err)
	}
	if ok, err := m.VerifyPassphrase(ctx, dev, mustSecret(t, "wrong-pass")); err != nil || ok {
		t.Errorf("VerifyPassphrase(wrong) = (%v, %v); want (false, nil) — a wrong passphrase is not an error", ok, err)
	}
	if ok, err := m.VerifyPassphrase(ctx, "/dev/cadestro-nonexistent-xyz", mustSecret(t, "x")); err == nil || ok {
		t.Errorf("VerifyPassphrase(nonexistent) = (%v, %v); want (false, error)", ok, err)
	}
}

func TestKeySlotLifecycle_Container(t *testing.T) {
	requireCryptsetup(t)
	m := directMgr(t)
	ctx, cancel := context.WithTimeout(context.Background(), containerCtxTimeout)
	defer cancel()
	dev := formatLUKS(t, "key-one")
	k1, k2 := mustSecret(t, "key-one"), mustSecret(t, "key-two")

	if err := m.AddKey(ctx, dev, k1, k2, AddKeyOptions{}); err != nil {
		t.Fatalf("AddKey(k2 authed by k1): %v", err)
	}
	if ok, err := m.VerifyPassphrase(ctx, dev, k2); err != nil || !ok {
		t.Fatalf("after AddKey, VerifyPassphrase(k2) = (%v, %v); want true", ok, err)
	}
	if err := m.RemoveKey(ctx, dev, k2); err != nil {
		t.Fatalf("RemoveKey(k2): %v", err)
	}
	if ok, err := m.VerifyPassphrase(ctx, dev, k2); err != nil || ok {
		t.Errorf("after RemoveKey, VerifyPassphrase(k2) = (%v, %v); want false", ok, err)
	}
	if ok, err := m.VerifyPassphrase(ctx, dev, k1); err != nil || !ok {
		t.Errorf("VerifyPassphrase(k1, untouched) = (%v, %v); want true", ok, err)
	}
}

func TestKillSlot_Container(t *testing.T) {
	requireCryptsetup(t)
	m := directMgr(t)
	ctx, cancel := context.WithTimeout(context.Background(), containerCtxTimeout)
	defer cancel()
	dev := formatLUKS(t, "slot-zero")
	k0, k1 := mustSecret(t, "slot-zero"), mustSecret(t, "slot-one")

	if err := m.AddKey(ctx, dev, k0, k1, AddKeyOptions{Slot: ptr(1)}); err != nil {
		t.Fatalf("AddKey(slot 1): %v", err)
	}
	if err := m.KillSlot(ctx, dev, 1, k0); err != nil {
		t.Fatalf("KillSlot(1, authed by k0): %v", err)
	}
	if ok, err := m.VerifyPassphrase(ctx, dev, k1); err != nil || ok {
		t.Errorf("after KillSlot(1), VerifyPassphrase(k1) = (%v, %v); want false", ok, err)
	}
	if ok, err := m.VerifyPassphrase(ctx, dev, k0); err != nil || !ok {
		t.Errorf("VerifyPassphrase(k0, untouched) = (%v, %v); want true", ok, err)
	}
}
