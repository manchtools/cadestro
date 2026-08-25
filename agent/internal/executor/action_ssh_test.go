package executor

import (
	"context"
	"strings"
	"testing"

	pb "github.com/manchtools/cadestro/contract/gen/go/cadestro/v1"
)

func TestExecuteSsh_RejectsNilParams(t *testing.T) {
	e := NewExecutor(nil)
	_, changed, err := e.executeSsh(context.Background(), nil, pb.DesiredState_DESIRED_STATE_PRESENT, "test1234")
	if err == nil {
		t.Fatal("expected error for nil params, got nil")
	}
	if changed {
		t.Error("changed must be false when params are nil")
	}
	if !strings.Contains(err.Error(), "required") {
		t.Errorf("error should mention 'required', got %q", err)
	}
}

func TestExecuteSsh_RejectsEmptyUsers(t *testing.T) {
	e := NewExecutor(nil)
	params := &pb.SshParams{}
	_, changed, err := e.executeSsh(context.Background(), params, pb.DesiredState_DESIRED_STATE_PRESENT, "test1234")
	if err == nil {
		t.Fatal("expected error for empty users, got nil")
	}
	if changed {
		t.Error("changed must be false when users are empty")
	}
}

func TestExecuteSsh_RejectsInvalidUsername(t *testing.T) {
	e := NewExecutor(nil)
	invalidUsers := []string{
		"",
		"user name with spaces",
		"-startswithdash",
		"../../../etc/passwd",
	}
	for _, user := range invalidUsers {
		t.Run("rejects "+user, func(t *testing.T) {
			params := &pb.SshParams{Users: []string{user}}
			_, _, err := e.executeSsh(context.Background(), params, pb.DesiredState_DESIRED_STATE_PRESENT, "test1234")
			if err == nil {
				t.Fatalf("expected error for invalid username %q, got nil", user)
			}
		})
	}
}

func TestExecuteSsh_RejectsEmptyActionID(t *testing.T) {
	e := NewExecutor(nil)
	params := &pb.SshParams{Users: []string{"alice"}}
	_, changed, err := e.executeSsh(context.Background(), params, pb.DesiredState_DESIRED_STATE_PRESENT, "")
	if err == nil {
		t.Fatal("expected error for empty action ID, got nil")
	}
	if changed {
		t.Error("changed must be false when action ID is empty")
	}
}

func TestExecuteSsh_RejectsTooLongActionID(t *testing.T) {
	e := NewExecutor(nil)
	params := &pb.SshParams{Users: []string{"alice"}}
	longID := strings.Repeat("a", maxActionIDForFilesystem+1)
	_, changed, err := e.executeSsh(context.Background(), params, pb.DesiredState_DESIRED_STATE_PRESENT, longID)
	if err == nil {
		t.Fatal("expected error for too-long action ID, got nil")
	}
	if changed {
		t.Error("changed must be false when action ID is too long")
	}
}

func TestExecuteSsh_RejectsUnsafeCharsInActionID(t *testing.T) {
	e := NewExecutor(nil)
	params := &pb.SshParams{Users: []string{"alice"}}
	unsafeIDs := []string{
		"../../etc",
		"test/1234",
		"test 1234",
		"test\x00null",
	}
	for _, id := range unsafeIDs {
		t.Run("rejects "+id, func(t *testing.T) {
			_, _, err := e.executeSsh(context.Background(), params, pb.DesiredState_DESIRED_STATE_PRESENT, id)
			if err == nil {
				t.Fatalf("expected error for unsafe action ID %q, got nil", id)
			}
		})
	}
}

func TestShortGroupName_FitsIn32Chars(t *testing.T) {
	tests := []struct {
		name     string
		prefix   string
		actionID string
	}{
		{"short id fits", "cadestro-ssh-", "01J123456789"},
		{"exact fit", "cadestro-ssh-", strings.Repeat("a", 19)},
		{"overflow with hash", "cadestro-ssh-", strings.Repeat("a", 50)},
		{"long prefix with long id", "cadestro-sudo-verylongprefix-", "01J1234567890123456789"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := shortGroupName(tt.prefix, tt.actionID)
			if len(got) > 32 {
				t.Errorf("shortGroupName(%q, ...) = %q (len=%d), must be ≤ 32 chars",
					tt.prefix, got, len(got))
			}
		})
	}
}

func TestShortGroupName_Deterministic(t *testing.T) {
	prefix := "cadestro-ssh-"
	id := "01J1234567890123456789"
	first := shortGroupName(prefix, id)
	for i := 0; i < 100; i++ {
		if got := shortGroupName(prefix, id); got != first {
			t.Fatalf("shortGroupName is non-deterministic: first=%q, iteration %d=%q", first, i, got)
		}
	}
}

func TestShortGroupName_DifferentIDsProduceDifferentNames(t *testing.T) {
	prefix := "cadestro-ssh-"

	id1 := "01JARXABCDEFGHIJKLMNOP1234"
	id2 := "01JARXABCDEFGHIJKLMNOP5678"
	n1 := shortGroupName(prefix, id1)
	n2 := shortGroupName(prefix, id2)
	if n1 == n2 {
		t.Errorf("shortGroupName collision: %q and %q both map to %q", id1, id2, n1)
	}
}

func TestGenerateSshGroupConfig_ContainsMatchGroup(t *testing.T) {
	got := generateSshGroupConfig("cadestro-ssh-test1234", &pb.SshParams{
		AllowPubkey:   true,
		AllowPassword: false,
	})
	if !strings.Contains(got, "Match Group cadestro-ssh-test1234") {
		t.Errorf("generated config missing Match Group directive:\n%s", got)
	}
	if !strings.Contains(got, "PubkeyAuthentication yes") {
		t.Errorf("generated config missing PubkeyAuthentication yes:\n%s", got)
	}
	if !strings.Contains(got, "PasswordAuthentication no") {
		t.Errorf("generated config missing PasswordAuthentication no:\n%s", got)
	}
	if strings.Contains(got, "PasswordAuthentication yes") {
		t.Errorf("generated config should NOT contain PasswordAuthentication yes:\n%s", got)
	}
}

func TestGenerateSshGroupConfig_BothAllowed(t *testing.T) {
	got := generateSshGroupConfig("cadestro-ssh-test1234", &pb.SshParams{
		AllowPubkey:   true,
		AllowPassword: true,
	})
	if !strings.Contains(got, "PubkeyAuthentication yes") {
		t.Errorf("missing PubkeyAuthentication yes")
	}
	if !strings.Contains(got, "PasswordAuthentication yes") {
		t.Errorf("missing PasswordAuthentication yes")
	}
}

func TestValidateActionIDForFilesystem_RejectsEmpty(t *testing.T) {
	err := validateActionIDForFilesystem("")
	if err == nil {
		t.Fatal("expected error for empty action ID")
	}
}

func TestValidateActionIDForFilesystem_RejectsUnsafeChars(t *testing.T) {
	unsafe := []string{
		"a/b",
		"a..b",
		"a b",
		"a\tb",
		"../../passwd",
	}
	for _, id := range unsafe {
		t.Run("rejects "+id, func(t *testing.T) {
			err := validateActionIDForFilesystem(id)
			if err == nil {
				t.Errorf("expected error for unsafe action ID %q", id)
			}
		})
	}
}
