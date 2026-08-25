package fs

import (
	"errors"
	"strings"
	"testing"
)

func TestValidatePath_AcceptsCanonicalShapes(t *testing.T) {
	for _, p := range []string{
		"/etc/sudoers.d/cadestro-power",
		"/var/lib/cadestro/wifi/abc/ca.pem",
		"/tmp/cadestro-test-keyfile",
		"/home/alice/file with spaces.txt",
		"relative/looking/path",
	} {
		t.Run(p, func(t *testing.T) {
			if err := ValidatePath(p); err != nil {
				t.Fatalf("ValidatePath(%q) = %v; want nil", p, err)
			}
		})
	}
}

func TestValidatePath_RejectsEmpty(t *testing.T) {
	err := ValidatePath("")
	if !errors.Is(err, ErrInvalidPath) {
		t.Fatalf("err = %v; want ErrInvalidPath", err)
	}
	if !strings.Contains(err.Error(), "empty") {
		t.Errorf("error %q should mention empty", err)
	}
}

func TestValidatePath_RejectsNULByte(t *testing.T) {
	err := ValidatePath("/etc/sudoers.d/cadestro-power\x00.evil")
	if !errors.Is(err, ErrInvalidPath) {
		t.Fatalf("err = %v; want ErrInvalidPath", err)
	}
	if !strings.Contains(err.Error(), "NUL") {
		t.Errorf("error %q should mention NUL", err)
	}
}

func TestValidatePath_RejectsLeadingDash(t *testing.T) {

	for _, p := range []string{
		"-no-preserve-root",
		"--force",
		"-rf",
		"-",
	} {
		t.Run(p, func(t *testing.T) {
			err := ValidatePath(p)
			if !errors.Is(err, ErrInvalidPath) {
				t.Fatalf("ValidatePath(%q) = %v; want ErrInvalidPath", p, err)
			}
		})
	}
}
