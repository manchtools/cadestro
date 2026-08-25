package executor

import (
	"context"
	"log/slog"
	"strings"
	"testing"
	"time"

	pb "github.com/manchtools/cadestro/contract/gen/go/cadestro/v1"
)

func TestRequireVerifiedArtifact(t *testing.T) {
	validHex := strings.Repeat("a", 64)
	if err := requireVerifiedArtifact("https://x/x.rpm", validHex); err != nil {
		t.Errorf("valid https+checksum rejected: %v", err)
	}

	if err := requireVerifiedArtifact("https://x/x.rpm", "  "+strings.Repeat("A", 64)+"  "); err != nil {
		t.Errorf("valid uppercase checksum rejected: %v", err)
	}
	bad := []struct{ url, sum string }{
		{"http://x/x.rpm", validHex},
		{"ftp://x/x.rpm", validHex},
		{"https://x/x.rpm", ""},
		{"https://x/x.rpm", "   "},
		{"https://x/x.rpm", "abc123"},
		{"https://x/x.rpm", strings.Repeat("a", 63)},
		{"https://x/x.rpm", strings.Repeat("z", 64)},
	}
	for _, tc := range bad {
		if err := requireVerifiedArtifact(tc.url, tc.sum); err == nil {
			t.Errorf("requireVerifiedArtifact(%q,%q) = nil; want error", tc.url, tc.sum)
		}
	}
}

func TestExecuteRpm_RejectsBeforeRemount(t *testing.T) {
	validHex := strings.Repeat("a", 64)
	cases := []struct {
		name string
		p    *pb.AppInstallParams
	}{
		{"http url", &pb.AppInstallParams{Url: "http://mirror/x.rpm", ChecksumSha256: validHex}},
		{"empty checksum", &pb.AppInstallParams{Url: "https://x/x.rpm", ChecksumSha256: ""}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var remountCalls int
			e := &Executor{logger: slog.Default(), now: time.Now, repairFS: func(context.Context) bool {
				remountCalls++
				return true
			}}
			out, changed, err := e.executeRpm(context.Background(), tc.p, pb.DesiredState_DESIRED_STATE_PRESENT)
			if err == nil {
				t.Fatalf("expected rejection, got out=%v changed=%v", out, changed)
			}
			if remountCalls != 0 {
				t.Errorf("privileged remount ran %d times before validation; want 0", remountCalls)
			}
		})
	}
}
