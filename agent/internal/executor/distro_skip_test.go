package executor

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	pb "github.com/manchtools/cadestro/contract/gen/go/cadestro/v1"
	packageSDK "github.com/manchtools/cadestro/sdk/pkg"
)

func debCapable() bool { return slices.Contains(packageSDK.Detect(), packageSDK.Apt) }
func rpmCapable() bool {
	d := packageSDK.Detect()
	return slices.Contains(d, packageSDK.Dnf) || slices.Contains(d, packageSDK.Dnf5) || slices.Contains(d, packageSDK.Zypper)
}
func flatpakCapable() bool { return packageSDK.FlatpakAvailable() }

func requireNotApplicable(t *testing.T, changed bool, err error, wantReason string) {
	t.Helper()
	if !errors.Is(err, errNotApplicable) {
		t.Fatalf("expected errNotApplicable, got: %v", err)
	}
	if !strings.Contains(err.Error(), wantReason) {
		t.Errorf("reason %q missing from error: %v", wantReason, err)
	}
	if changed {
		t.Error("expected changed=false for a not-applicable action")
	}
}

func TestExecuteDeb_NotApplicableWhenDpkgMissing(t *testing.T) {
	if debCapable() {
		t.Skip("apt (deb backend) detected — test requires a non-deb host")
	}

	e := NewExecutor(nil)

	_, changed, err := e.executeDeb(context.Background(), &pb.AppInstallParams{
		Url:            "https://example.com/test.deb",
		ChecksumSha256: strings.Repeat("a", 64),
	}, pb.DesiredState_DESIRED_STATE_PRESENT)

	requireNotApplicable(t, changed, err, "no supported .deb package manager")
}

func TestExecuteRpm_NotApplicableWhenRpmMissing(t *testing.T) {
	if rpmCapable() {
		t.Skip("dnf/zypper (rpm backend) detected — test requires a non-rpm host")
	}

	e := NewExecutor(nil)

	_, changed, err := e.executeRpm(context.Background(), &pb.AppInstallParams{
		Url:            "https://example.com/test.rpm",
		ChecksumSha256: strings.Repeat("a", 64),
	}, pb.DesiredState_DESIRED_STATE_PRESENT)

	requireNotApplicable(t, changed, err, "no supported .rpm package manager")
}

func TestExecuteFlatpak_NotApplicableWhenFlatpakMissing(t *testing.T) {
	if flatpakCapable() {
		t.Skip("flatpak detected — test requires a host without flatpak")
	}

	e := NewExecutor(nil)
	_, changed, err := e.executeFlatpak(context.Background(), &pb.FlatpakParams{
		AppId: &pb.FlatpakAppId{Value: "org.example.Test"},
	}, pb.DesiredState_DESIRED_STATE_PRESENT)

	requireNotApplicable(t, changed, err, "flatpak not available")
}

func TestExecuteDeb_DoesNotSkipWhenDpkgPresent(t *testing.T) {
	if !debCapable() {
		t.Skip("apt (deb backend) not detected on this host")
	}

	e := NewExecutor(nil)

	output, _, err := e.executeDeb(context.Background(), &pb.AppInstallParams{
		Url:            "https://invalid.example.com/nonexistent.deb",
		ChecksumSha256: strings.Repeat("a", 64),
	}, pb.DesiredState_DESIRED_STATE_PRESENT)

	if output != nil && strings.Contains(output.Stdout, "skipped") {
		t.Error("DEB executor should not skip when dpkg is available")
	}
	if errors.Is(err, errNotApplicable) {
		t.Errorf("DEB executor wrongly reported not-applicable on a deb-capable host: %v", err)
	}
	if err == nil {
		t.Error("expected error from download/install of invalid URL, got nil")
	}

	if err != nil && strings.Contains(err.Error(), "artifact rejected") {
		t.Errorf("valid action wrongly rejected at the artifact guard: %v", err)
	}
}

func TestExecuteRpm_DoesNotSkipWhenRpmPresent(t *testing.T) {
	if !rpmCapable() {
		t.Skip("dnf/zypper (rpm backend) not detected on this host")
	}

	e := NewExecutor(nil)

	output, _, err := e.executeRpm(context.Background(), &pb.AppInstallParams{
		Url:            "https://invalid.example.com/nonexistent.rpm",
		ChecksumSha256: strings.Repeat("a", 64),
	}, pb.DesiredState_DESIRED_STATE_PRESENT)

	if output != nil && strings.Contains(output.Stdout, "skipped") {
		t.Error("RPM executor should not skip when rpm is available")
	}
	if errors.Is(err, errNotApplicable) {
		t.Errorf("RPM executor wrongly reported not-applicable on an rpm-capable host: %v", err)
	}
	if err == nil {
		t.Error("expected error from download/install of invalid URL, got nil")
	}

	if err != nil && strings.Contains(err.Error(), "artifact rejected") {
		t.Errorf("valid action wrongly rejected at the artifact guard: %v", err)
	}
}

func TestExecuteFlatpak_DoesNotSkipWhenFlatpakPresent(t *testing.T) {
	if !flatpakCapable() {
		t.Skip("flatpak not detected on this host")
	}

	e := NewExecutor(nil)
	output, _, err := e.executeFlatpak(context.Background(), &pb.FlatpakParams{
		AppId: &pb.FlatpakAppId{Value: "org.nonexistent.surely_does_not_exist_12345"},
	}, pb.DesiredState_DESIRED_STATE_PRESENT)

	if output != nil && strings.Contains(output.Stdout, "skipped") {
		t.Error("Flatpak executor should not skip when flatpak is available")
	}
	if errors.Is(err, errNotApplicable) {
		t.Errorf("Flatpak executor wrongly reported not-applicable on a flatpak-capable host: %v", err)
	}
	if err == nil {
		t.Error("expected error from install of nonexistent app, got nil")
	}
}
