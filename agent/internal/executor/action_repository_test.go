package executor

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"

	pb "github.com/manchtools/cadestro/contract/gen/go/cadestro/v1"
	"github.com/manchtools/cadestro/sdk/pkg"
	sysexec "github.com/manchtools/cadestro/sdk/sys/exec"
)

func TestExecuteRepository_RejectsBeforePrivilegedRemount(t *testing.T) {
	runner, err := sysexec.NewRunner(sysexec.Direct)
	if err != nil {
		t.Fatal(err)
	}
	var remountCalls int
	e := &Executor{logger: slog.Default(), now: time.Now, pkgBackend: pkg.Dnf, runner: runner, repairFS: func(context.Context) bool {
		remountCalls++
		return true
	}}
	dnf := func(baseurl string) *pb.DnfRepository {
		return &pb.DnfRepository{Baseurl: baseurl, Gpgcheck: true}
	}
	bad := []*pb.RepositoryParams{
		{Name: "r", Dnf: dnf("http://evil")},
		{Name: "../etc", Dnf: dnf("https://mirror.example/repo")},
		{Name: strings.Repeat("a", 200), Dnf: dnf("https://mirror.example/repo")},
	}
	for i, p := range bad {
		out, changed, err := e.executeRepository(context.Background(), p, pb.DesiredState_DESIRED_STATE_PRESENT)
		if err == nil {
			t.Errorf("case %d: malformed repo action accepted: out=%v changed=%v", i, out, changed)
			continue
		}
		if strings.Contains(err.Error(), "no supported package manager") {
			t.Errorf("case %d: rejected by backend dispatch, not validation: %v", i, err)
		}
	}
	if remountCalls != 0 {
		t.Errorf("privileged remount ran %d times for rejected actions; want 0", remountCalls)
	}
}

func TestRepositoryFields_Dnf5UsesDnfConfig(t *testing.T) {
	e := &Executor{pkgBackend: pkg.Dnf5}
	r, err := e.repositoryFields(&pb.RepositoryParams{
		Name: "corp",
		Dnf:  &pb.DnfRepository{Baseurl: "https://mirror.example/repo", Description: "Fedora", Enabled: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	if r.Dnf == nil || r.Dnf.BaseURL != "https://mirror.example/repo" || r.Dnf.Description != "Fedora" || !r.Dnf.Enabled {
		t.Fatalf("Dnf5 repository fields = %+v", r.Dnf)
	}
}

func TestRepositoryFields_ZypperTypeMapping(t *testing.T) {
	e := &Executor{pkgBackend: pkg.Zypper}
	cases := []struct {
		name string
		kind pb.ZypperRepositoryType
		want string
	}{
		{name: "unspecified", kind: pb.ZypperRepositoryType_ZYPPER_REPOSITORY_TYPE_UNSPECIFIED},
		{name: "rpm-md", kind: pb.ZypperRepositoryType_ZYPPER_REPOSITORY_TYPE_RPM_MD, want: "rpm-md"},
		{name: "yast2", kind: pb.ZypperRepositoryType_ZYPPER_REPOSITORY_TYPE_YAST2, want: "yast2"},
		{name: "plaindir", kind: pb.ZypperRepositoryType_ZYPPER_REPOSITORY_TYPE_PLAINDIR, want: "plaindir"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r, err := e.repositoryFields(&pb.RepositoryParams{Zypper: &pb.ZypperRepository{Type: tc.kind}})
			if err != nil {
				t.Fatal(err)
			}
			if r.Zypper == nil || r.Zypper.Type != tc.want {
				t.Fatalf("mapped type = %q, want %q", r.Zypper.Type, tc.want)
			}
		})
	}
	_, err := e.repositoryFields(&pb.RepositoryParams{Zypper: &pb.ZypperRepository{Type: pb.ZypperRepositoryType(99)}})
	if !errors.Is(err, errUnsupportedZypperRepositoryType) {
		t.Fatalf("unknown type error = %v, want errUnsupportedZypperRepositoryType", err)
	}
}

func TestDownloadAptKey_RejectsNonHTTPS(t *testing.T) {
	e := &Executor{}

	for _, u := range []string{"http://m/key.asc", "ftp://m/key", "file:///etc/x", "//m/key", "https:m/k"} {
		if _, err := e.downloadAptKey(context.Background(), u); err == nil {
			t.Errorf("non-https GPG key URL accepted: %q", u)
		}
	}
}
