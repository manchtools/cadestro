package user

import (
	"context"
	"io"
	"math/big"
	"strings"
	"testing"

	"github.com/manchtools/cadestro/sdk/sys/exec"
	"github.com/manchtools/cadestro/sdk/sys/exec/exectest"
)

func TestGet_NonNumericUIDFailsClosed(t *testing.T) {
	f := exectest.New(exec.Direct)
	f.Push(exec.Result{Stdout: "deploy:x:NOTANUMBER:1000:Deploy:/home/deploy:/bin/bash\n"}, nil)
	if _, err := mgr(t, f).Get(context.Background(), "deploy"); err == nil {
		t.Error("Get accepted a non-numeric UID; want a fail-closed error")
	}
}

func TestGet_NonNumericGIDFailsClosed(t *testing.T) {
	f := exectest.New(exec.Direct)
	f.Push(exec.Result{Stdout: "deploy:x:1000:NOTANUMBER:Deploy:/home/deploy:/bin/bash\n"}, nil)
	if _, err := mgr(t, f).Get(context.Background(), "deploy"); err == nil {
		t.Error("Get accepted a non-numeric GID; want a fail-closed error")
	}
}

func TestGet_StarPasswordIsNotLocked(t *testing.T) {
	f := exectest.New(exec.Direct)
	f.Push(exec.Result{Stdout: "deploy:x:1000:1000::/home/deploy:/bin/bash\n"}, nil)
	f.Push(exec.Result{Stdout: "deploy:x:1000:\n"}, nil)
	f.Push(exec.Result{Stdout: "deploy\n"}, nil)
	f.Push(exec.Result{Stdout: "deploy:*:19000:0:99999:7:::\n"}, nil)
	info, err := mgr(t, f).Get(context.Background(), "deploy")
	if err != nil {
		t.Fatal(err)
	}
	if info.Locked {
		t.Error("'*'-prefixed shadow entry wrongly detected as locked (only '!' means locked)")
	}
	if !info.LockedKnown {
		t.Error("LockedKnown = false after a successful shadow read; Locked is authoritative here")
	}
}

func TestGet_TrimsWhitespaceAndCR(t *testing.T) {
	f := exectest.New(exec.Direct)
	f.Push(exec.Result{Stdout: "  deploy:x:1000:1000:Deploy:/home/deploy:/bin/bash\r\n"}, nil)
	f.Push(exec.Result{Stdout: "deploy:x:1000:\n"}, nil)
	f.Push(exec.Result{Stdout: "deploy\n"}, nil)
	f.Push(exec.Result{Stdout: "deploy:$6$x:::::::\n"}, nil)
	info, err := mgr(t, f).Get(context.Background(), "deploy")
	if err != nil {
		t.Fatalf("Get with padded output: %v", err)
	}
	if info.UID != 1000 || info.Shell != "/bin/bash" {
		t.Errorf("Info = %+v, want UID 1000 / /bin/bash despite padding", info)
	}
}

func TestGroupMembers_FiltersEmptyEntries(t *testing.T) {
	f := exectest.New(exec.Direct)
	f.Push(exec.Result{Stdout: "docker:x:999:deploy,,ops,\n"}, nil)
	members, err := mgr(t, f).GroupMembers(context.Background(), "docker")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(members, ",") != "deploy,ops" {
		t.Errorf("members = %v, want [deploy ops] with empties filtered", members)
	}
}

func TestGroupMembers_AllEmptyEntriesIsNil(t *testing.T) {
	f := exectest.New(exec.Direct)
	f.Push(exec.Result{Stdout: "docker:x:999:,,\n"}, nil)
	members, err := mgr(t, f).GroupMembers(context.Background(), "docker")
	if err != nil || members != nil {
		t.Errorf("GroupMembers = (%v,%v), want (nil,nil) for an all-separators field", members, err)
	}
}

func TestSetPassword_ColonInPasswordIsPreserved(t *testing.T) {
	f := exectest.New(exec.Direct)
	secret, err := exec.NewSecret("p@ss:w0rd:with:colons")
	if err != nil {
		t.Fatal(err)
	}
	if err := mgr(t, f).SetPassword(context.Background(), "deploy", secret); err != nil {
		t.Fatal(err)
	}
	got, _ := io.ReadAll(f.Calls()[0].Stdin)
	if string(got) != "deploy:p@ss:w0rd:with:colons" {
		t.Errorf("chpasswd stdin = %q, want the colon-bearing password intact", got)
	}
}

func TestGeneratePassword_ExactBounds(t *testing.T) {
	for _, n := range []int{MinPasswordLength, MaxPasswordLength} {
		s, err := GeneratePassword(n, ComplexityComplex)
		if err != nil {
			t.Fatalf("GeneratePassword(%d): %v", n, err)
		}
		if len(s.Reveal()) != n {
			t.Errorf("length = %d, want %d", len(s.Reveal()), n)
		}
	}
}

func TestGeneratePassword_RNGFailure(t *testing.T) {
	restore := randInt
	randInt = func(io.Reader, *big.Int) (*big.Int, error) {
		return nil, io.ErrUnexpectedEOF
	}
	defer func() { randInt = restore }()

	if _, err := GeneratePassword(16, ComplexityAlphanumeric); err == nil {
		t.Error("GeneratePassword returned nil error when the RNG failed")
	}
}
