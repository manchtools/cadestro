//go:build container

package antivirus_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/manchtools/cadestro/sdk/sys/antivirus"
	sysexec "github.com/manchtools/cadestro/sdk/sys/exec"
)

const marker = "CADESTRO-CLAMAV-TEST-MARKER"

func realAV(t *testing.T) antivirus.Manager {
	t.Helper()
	if !hasClamscan(t) {
		t.Skip("clamscan not installed here; ClamAV backend not exercisable")
	}
	r, err := sysexec.NewRunner(sysexec.Direct)
	if err != nil {
		t.Fatalf("NewRunner(Direct): %v", err)
	}
	m, err := antivirus.New(antivirus.ClamAV, r)
	if err != nil {
		t.Fatalf("New(ClamAV): %v", err)
	}
	return m
}

func hasClamscan(t *testing.T) bool {
	t.Helper()
	return len(antivirus.Detect(context.Background())) > 0
}

func avCtx(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	t.Cleanup(cancel)
	return ctx
}

func TestDetect_Container(t *testing.T) {
	if !hasClamscan(t) {
		t.Skip("clamscan not installed here")
	}
	got := antivirus.Detect(context.Background())
	if len(got) != 1 || got[0] != antivirus.ClamAV {
		t.Errorf("Detect = %v, want [clamav]", got)
	}
}

func TestScan_Clean_Container(t *testing.T) {
	m := realAV(t)
	dir := t.TempDir()
	clean := filepath.Join(dir, "clean.txt")
	if err := os.WriteFile(clean, []byte("nothing to see here\n"), 0o644); err != nil {
		t.Fatalf("write clean file: %v", err)
	}
	res, err := m.Scan(avCtx(t), clean)
	if err != nil {
		t.Fatalf("Scan(clean): %v", err)
	}
	if !res.Clean() {
		t.Errorf("clean file reported infected: %+v", res.Infected)
	}
}

func eicarString() string {
	return `X5O!P%@AP[4\PZX54(P^)7CC)7}` + `$EICAR-STANDARD-ANTIVIRUS-TEST-FILE!$H+H*`
}

func TestScan_EICAR_Container(t *testing.T) {
	m := realAV(t)
	dir := t.TempDir()
	bad := filepath.Join(dir, "eicar.txt")
	if err := os.WriteFile(bad, []byte(eicarString()), 0o644); err != nil {
		t.Fatalf("write EICAR file: %v", err)
	}
	res, err := m.Scan(avCtx(t), bad)
	if err != nil {
		t.Fatalf("Scan(EICAR): %v", err)
	}
	if res.Clean() {
		t.Fatal("EICAR not detected — real clamscan + seed .hdb produced no infection")
	}
	if len(res.Infected) != 1 {
		t.Fatalf("want exactly 1 infection, got %d: %+v", len(res.Infected), res.Infected)
	}
	inf := res.Infected[0]
	if inf.File != bad {
		t.Errorf("Infection.File = %q, want %q", inf.File, bad)
	}
	if !strings.Contains(inf.Signature, "Cadestro.Eicar.Test") {
		t.Errorf("Infection.Signature = %q, want it to carry the seed sig name Cadestro.Eicar.Test", inf.Signature)
	}
}

func TestScan_Marker_Container(t *testing.T) {
	m := realAV(t)
	dir := t.TempDir()
	bad := filepath.Join(dir, "marker.txt")
	if err := os.WriteFile(bad, []byte("leading junk "+marker+" trailing junk\n"), 0o644); err != nil {
		t.Fatalf("write marker file: %v", err)
	}
	res, err := m.Scan(avCtx(t), bad)
	if err != nil {
		t.Fatalf("Scan(marker): %v", err)
	}
	if res.Clean() || len(res.Infected) != 1 {
		t.Fatalf("marker not detected as exactly one infection: %+v", res.Infected)
	}
	if !strings.Contains(res.Infected[0].Signature, "Cadestro.Marker.Test") {
		t.Errorf("Infection.Signature = %q, want it to carry the seed sig name Cadestro.Marker.Test", res.Infected[0].Signature)
	}
}

func TestScan_InvalidPath_Container(t *testing.T) {
	m := realAV(t)
	if _, err := m.Scan(avCtx(t), "-rf"); err == nil {
		t.Error("Scan accepted a flag-shaped path; want ErrInvalidPath")
	}
}

func TestVersion_Container(t *testing.T) {
	m := realAV(t)
	if _, err := m.Version(avCtx(t)); err == nil {
		t.Error("Version against a signature-DB-less clamscan should error per the signature-required contract; got nil")
	}
}

func TestUpdateSignatures_Container(t *testing.T) {
	m := realAV(t)
	if err := m.UpdateSignatures(avCtx(t)); err == nil {
		t.Error("UpdateSignatures with no freshclam config should surface freshclam's failure; got nil")
	}
}
