package executor

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"testing"
	"time"

	pb "github.com/manchtools/cadestro/contract/gen/go/cadestro/v1"
	"github.com/manchtools/cadestro/sdk/sys/remote"
)

func TestExecuteDeb_RejectsBeforeRemount(t *testing.T) {
	validHex := strings.Repeat("a", 64)
	cases := []struct {
		name string
		p    *pb.AppInstallParams
	}{
		{"http url", &pb.AppInstallParams{Url: "http://mirror/x.deb", ChecksumSha256: validHex}},
		{"ftp url", &pb.AppInstallParams{Url: "ftp://mirror/x.deb", ChecksumSha256: validHex}},
		{"empty checksum", &pb.AppInstallParams{Url: "https://x/x.deb", ChecksumSha256: ""}},
		{"whitespace checksum", &pb.AppInstallParams{Url: "https://x/x.deb", ChecksumSha256: "   "}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var remountCalls int
			e := &Executor{logger: slog.Default(), now: time.Now, repairFS: func(context.Context) bool {
				remountCalls++
				return true
			}}
			out, changed, err := e.executeDeb(context.Background(), tc.p, pb.DesiredState_DESIRED_STATE_PRESENT)
			if err == nil {
				t.Fatalf("expected rejection, got out=%v changed=%v", out, changed)
			}
			if remountCalls != 0 {
				t.Errorf("privileged remount ran %d times before validation; want 0", remountCalls)
			}
		})
	}
}

func TestDebAbsentPackageName_NoChecksumNeverFetches(t *testing.T) {
	fetchCalls := 0
	orig := fetchArtifact
	fetchArtifact = func(_ context.Context, _, _, _, _ string, _ remote.RedirectPolicy) error {
		fetchCalls++
		return nil
	}
	t.Cleanup(func() { fetchArtifact = orig })

	e := &Executor{logger: slog.Default(), now: time.Now}
	name, err := e.debAbsentPackageName(context.Background(), nil,
		&pb.AppInstallParams{Url: "https://mirror/pool/foo-agent_1.2.3_amd64.deb", ChecksumSha256: ""})
	if err != nil {
		t.Fatalf("URL-filename fallback must succeed: %v", err)
	}
	if name != "foo-agent" {
		t.Fatalf("name = %q, want %q (from the signed URL, not the artifact)", name, "foo-agent")
	}
	if fetchCalls != 0 {
		t.Fatalf("fetchArtifact was called %d times with an unverifiable checksum — the origin must never choose the removal target", fetchCalls)
	}
}

func TestDebAbsentPackageName_WithChecksumFetchesVerified(t *testing.T) {
	var gotChecksum string
	fetchCalls := 0
	orig := fetchArtifact
	fetchArtifact = func(_ context.Context, _, _ string, checksum, _ string, _ remote.RedirectPolicy) error {
		fetchCalls++
		gotChecksum = checksum
		return fmt.Errorf("404: artifact deleted upstream")
	}
	t.Cleanup(func() { fetchArtifact = orig })

	validHex := strings.Repeat("a", 64)
	e := &Executor{logger: slog.Default(), now: time.Now}
	name, err := e.debAbsentPackageName(context.Background(), nil,
		&pb.AppInstallParams{Url: "https://mirror/pool/foo-agent_1.2.3_amd64.deb", ChecksumSha256: validHex})
	if err != nil {
		t.Fatalf("stale-URL fallback must succeed: %v", err)
	}
	if name != "foo-agent" {
		t.Fatalf("name = %q, want %q", name, "foo-agent")
	}
	if fetchCalls != 1 {
		t.Fatalf("fetchArtifact calls = %d, want 1", fetchCalls)
	}
	if gotChecksum != validHex {
		t.Fatalf("fetch ran without the action's checksum (got %q)", gotChecksum)
	}
}
