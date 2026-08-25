package exec

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

func TestNewSecret_RejectsNewlineAndCR(t *testing.T) {
	for _, bad := range []string{"a\nb", "a\rb", "trailing\n", "\r", "x\r\ny"} {
		if _, err := NewSecret(bad); !errors.Is(err, ErrSecretContainsNewline) {
			t.Errorf("NewSecret(%q) err = %v, want ErrSecretContainsNewline", bad, err)
		}
	}
}

func TestNewSecret_EmptyIsValidAndZero(t *testing.T) {
	s, err := NewSecret("")
	if err != nil {
		t.Fatalf("NewSecret(\"\") err = %v, want nil", err)
	}
	if !s.IsZero() {
		t.Errorf("empty secret IsZero() = false, want true")
	}
}

func TestNewMultilineSecret_AllowsNewlinesAndStillRedacts(t *testing.T) {

	const pem = "-----BEGIN PRIVATE KEY-----\nMIIBVgIBADAN\nQ==\n-----END PRIVATE KEY-----\n"
	s := NewMultilineSecret(pem)
	if s.IsZero() {
		t.Error("non-empty multiline secret IsZero() = true, want false")
	}
	if got := s.Reveal(); got != pem {
		t.Errorf("Reveal() round-trip = %q, want the verbatim PEM", got)
	}
	var logged any = s
	for verb, out := range map[string]string{
		"String()": s.String(),
		"%v":       fmt.Sprintf("%v", logged),
		"%s":       fmt.Sprintf("%s", logged),
	} {
		if strings.Contains(out, "PRIVATE KEY") || strings.Contains(out, "MIIBVgIBADAN") {
			t.Errorf("%s leaked PEM material: %q", verb, out)
		}
		if !strings.Contains(out, "[REDACTED]") {
			t.Errorf("%s = %q, want [REDACTED]", verb, out)
		}
	}

	if !NewMultilineSecret("").IsZero() {
		t.Error("empty multiline secret IsZero() = false, want true")
	}
}

func TestSecret_RedactsEverywhereButReveal(t *testing.T) {
	const plaintext = "hunter2-s3cr3t"
	s, err := NewSecret(plaintext)
	if err != nil {
		t.Fatalf("NewSecret err = %v", err)
	}
	if s.IsZero() {
		t.Errorf("non-empty secret IsZero() = true, want false")
	}
	if got := s.Reveal(); got != plaintext {
		t.Errorf("Reveal() = %q, want %q", got, plaintext)
	}

	var logged any = s
	renders := map[string]string{
		"String()": s.String(),
		"%v":       fmt.Sprintf("%v", logged),
		"%s":       fmt.Sprintf("%s", logged),
		"%#v":      fmt.Sprintf("%#v", logged),
		"%+v":      fmt.Sprintf("%+v", logged),
	}
	for verb, out := range renders {
		if strings.Contains(out, plaintext) {
			t.Errorf("%s leaked the plaintext: %q", verb, out)
		}
		if !strings.Contains(out, "[REDACTED]") {
			t.Errorf("%s = %q, want it to contain [REDACTED]", verb, out)
		}
	}
}

func TestSecret_RedactsWhenNestedInStruct(t *testing.T) {
	const plaintext = "nested-passphrase"
	s, _ := NewSecret(plaintext)
	type creds struct {
		User string
		Pass Secret
	}
	out := fmt.Sprintf("%v", creds{User: "deploy", Pass: s})
	if strings.Contains(out, plaintext) {
		t.Fatalf("nested Secret leaked plaintext: %q", out)
	}
	if !strings.Contains(out, "[REDACTED]") {
		t.Fatalf("nested Secret not redacted: %q", out)
	}
}

func TestSecret_HasNewline(t *testing.T) {
	clean, err := NewSecret("Hunter2-no-newlines")
	if err != nil {
		t.Fatal(err)
	}
	if clean.HasNewline() {
		t.Error("a clean secret reported a newline")
	}
	if NewMultilineSecret("a\nb").HasNewline() != true {
		t.Error("a \\n multiline secret must report HasNewline")
	}
	if NewMultilineSecret("a\rb").HasNewline() != true {
		t.Error("a \\r multiline secret must report HasNewline")
	}
	if (Secret{}).HasNewline() {
		t.Error("the zero secret has no newline")
	}
}
