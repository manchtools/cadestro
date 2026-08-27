package user

import (
	"strings"
	"testing"
)

func TestIsValidName(t *testing.T) {

	max32 := "a" + strings.Repeat("b", 31)
	if !IsValidName(max32) {
		t.Errorf("IsValidName(32-char) = false, want true (exact max boundary)")
	}
	if IsValidName(max32 + "c") {
		t.Errorf("IsValidName(33-char) = true, want false (one over max)")
	}

	valid := []string{"deploy", "a", "user_1", "svc-acct", max32}
	for _, n := range valid {
		if !IsValidName(n) {
			t.Errorf("IsValidName(%q) = false, want true", n)
		}
	}
	invalid := []string{
		"",
		"Deploy",
		"1user",
		"-rf",
		"_priv",
		"user name",
		"user:x",
		"user\nroot",
		"a2345678901234567890123456789012x",
	}
	for _, n := range invalid {
		if IsValidName(n) {
			t.Errorf("IsValidName(%q) = true, want false", n)
		}
	}
}

func TestDefaultShell(t *testing.T) {
	if got := DefaultShell(false); got != "/bin/bash" {
		t.Errorf("DefaultShell(false) = %q, want /bin/bash", got)
	}
	if got := DefaultShell(true); got != "/usr/sbin/nologin" {
		t.Errorf("DefaultShell(true) = %q, want /usr/sbin/nologin", got)
	}
}
