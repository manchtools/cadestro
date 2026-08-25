package executor

import (
	"reflect"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	pb "github.com/manchtools/cadestro/contract/gen/go/cadestro/v1"
	"github.com/manchtools/cadestro/sdk/pkg"
	sysexec "github.com/manchtools/cadestro/sdk/sys/exec"
	"github.com/manchtools/cadestro/sdk/sys/repo"
)

func testRunnerForValidation(t *testing.T) sysexec.Runner {
	t.Helper()
	r, err := sysexec.NewRunner(sysexec.Direct)
	require.NoError(t, err)
	return r
}

func validateRepoViaSDK(t *testing.T, p *pb.RepositoryParams) error {
	t.Helper()
	var backend pkg.Backend
	switch {
	case p.Apt != nil:
		backend = pkg.Apt
	case p.Dnf != nil:
		backend = pkg.Dnf
	case p.Pacman != nil:
		backend = pkg.Pacman
	case p.Zypper != nil:
		backend = pkg.Zypper
	default:
		t.Fatal("test params configure no backend")
	}
	mgr, err := repo.New(backend, testRunnerForValidation(t))
	require.NoError(t, err)
	e := &Executor{pkgBackend: backend}
	r, err := e.repositoryFields(p)
	if err != nil {
		return err
	}
	return mgr.Validate(r)
}

func TestRepository_AcceptsRealistic(t *testing.T) {
	cases := map[string]*pb.RepositoryParams{
		"apt": {Name: "r", Apt: &pb.AptRepository{
			Url: "https://apt.example.com/debian", Distribution: "bookworm",
			Components: []string{"main", "contrib", "non-free-firmware"}, Arch: "amd64,arm64",
		}},
		"dnf": {Name: "r", Dnf: &pb.DnfRepository{
			Baseurl: "https://dnf.example.com/fedora/$releasever", Description: "Example DNF repo",
			Gpgkey: "https://dnf.example.com/key.asc", Gpgcheck: true,
		}},
		"pacman": {Name: "r", Pacman: &pb.PacmanRepository{
			Server: "https://arch.example.com/os/$arch", SigLevel: "Optional TrustAll",
		}},
		"zypper": {Name: "r", Zypper: &pb.ZypperRepository{
			Url: "https://zypper.example.com/15.5", Description: "Example Zypper repo",
			Gpgkey: "https://zypper.example.com/key.asc", Type: pb.ZypperRepositoryType_ZYPPER_REPOSITORY_TYPE_RPM_MD,
		}},
	}
	for name, p := range cases {
		t.Run(name, func(t *testing.T) {
			if err := validateRepoViaSDK(t, p); err != nil {
				t.Fatalf("legitimate config rejected: %v", err)
			}
		})
	}
}

func TestRepository_RejectsBadBaseURLAndGpgKey(t *testing.T) {
	reject := []*pb.RepositoryParams{
		{Name: "r", Dnf: &pb.DnfRepository{Baseurl: "http://m/r", Gpgcheck: true}},
		{Name: "r", Zypper: &pb.ZypperRepository{Url: "http://m/r", Gpgcheck: true}},
		{Name: "r", Pacman: &pb.PacmanRepository{Server: "http://m/r"}},
		{Name: "r", Dnf: &pb.DnfRepository{Baseurl: "ftp://x", Gpgcheck: true}},
		{Name: "r", Dnf: &pb.DnfRepository{Baseurl: "https://m/r", Gpgcheck: true, Gpgkey: "http://evil/key"}},
		{Name: "r", Dnf: &pb.DnfRepository{Baseurl: "https://m/r", Gpgcheck: true, Gpgkey: "ext::sh -c id"}},
		{Name: "r", Dnf: &pb.DnfRepository{Baseurl: "https://m/r", Gpgcheck: true, Gpgkey: "--import=/etc/shadow"}},
	}
	for i, p := range reject {
		if err := validateRepoViaSDK(t, p); err == nil {
			t.Errorf("reject case %d accepted a non-https base URL or unsafe gpg key ref", i)
		}
	}
}

func TestRepository_AllowsOperatorChoiceGpgcheck(t *testing.T) {
	accept := []*pb.RepositoryParams{
		{Name: "r", Dnf: &pb.DnfRepository{Baseurl: "https://m/r", Gpgcheck: false}},
		{Name: "r", Dnf: &pb.DnfRepository{Baseurl: "https://m/r", Gpgcheck: false, Gpgkey: "https://m/k"}},
		{Name: "r", Zypper: &pb.ZypperRepository{Url: "https://m/r", Gpgcheck: false}},
	}
	for i, p := range accept {
		if err := validateRepoViaSDK(t, p); err != nil {
			t.Errorf("accept case %d: legitimate operator-choice config rejected: %v", i, err)
		}
	}
}

func protoFieldName(f reflect.StructField) string {
	for _, part := range strings.Split(f.Tag.Get("protobuf"), ",") {
		if strings.HasPrefix(part, "name=") {
			return strings.TrimPrefix(part, "name=")
		}
	}
	return ""
}

func TestRepository_SelfDiscoversEveryStringField(t *testing.T) {

	excluded := map[string]bool{
		"apt.gpg_key":     true,
		"apt.gpg_key_url": true,
	}

	managers := []struct {
		prefix string
		base   func() proto.Message
		wrap   func(proto.Message) *pb.RepositoryParams
	}{
		{"apt",
			func() proto.Message {
				return &pb.AptRepository{Url: "https://m/d", Distribution: "stable", Components: []string{"main"}, Arch: "amd64"}
			},
			func(m proto.Message) *pb.RepositoryParams {
				return &pb.RepositoryParams{Name: "r", Apt: m.(*pb.AptRepository)}
			}},
		{"dnf",
			func() proto.Message { return &pb.DnfRepository{Baseurl: "https://m/r"} },
			func(m proto.Message) *pb.RepositoryParams {
				return &pb.RepositoryParams{Name: "r", Dnf: m.(*pb.DnfRepository)}
			}},
		{"pacman",
			func() proto.Message { return &pb.PacmanRepository{Server: "https://m/x"} },
			func(m proto.Message) *pb.RepositoryParams {
				return &pb.RepositoryParams{Name: "r", Pacman: m.(*pb.PacmanRepository)}
			}},
		{"zypper",
			func() proto.Message { return &pb.ZypperRepository{Url: "https://m/r"} },
			func(m proto.Message) *pb.RepositoryParams {
				return &pb.RepositoryParams{Name: "r", Zypper: m.(*pb.ZypperRepository)}
			}},
	}

	const payload = "x\nEvil: 1"
	covered, urlish := 0, 0
	for _, mgr := range managers {
		rt := reflect.TypeOf(mgr.base()).Elem()
		for i := 0; i < rt.NumField(); i++ {
			f := rt.Field(i)
			snake := protoFieldName(f)
			if snake == "" {
				continue
			}
			isString := f.Type.Kind() == reflect.String
			isStringSlice := f.Type.Kind() == reflect.Slice && f.Type.Elem().Kind() == reflect.String
			if !isString && !isStringSlice {
				continue
			}
			key := mgr.prefix + "." + snake
			if excluded[key] {
				continue
			}

			fresh := mgr.base()
			fv := reflect.ValueOf(fresh).Elem().Field(i)
			if isString {
				fv.SetString(payload)
			} else {
				fv.Set(reflect.ValueOf([]string{payload}))
			}

			if err := validateRepoViaSDK(t, mgr.wrap(fresh)); err == nil {
				t.Errorf("%s: control-char value accepted (field unmapped or unguarded)", key)
				continue
			}
			covered++
			if strings.Contains(snake, "url") || strings.Contains(snake, "server") ||
				strings.Contains(snake, "gpgkey") || strings.Contains(snake, "baseurl") {
				urlish++
			}
		}
	}
	if covered == 0 {
		t.Fatal("self-discovering walk covered 0 fields — reflection is broken")
	}
	if urlish == 0 {
		t.Fatal("no URL-ish field (url/server/baseurl/gpgkey) was exercised — mapping/walk mismatch")
	}
}
