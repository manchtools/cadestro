package executor

import (
	"context"
	"fmt"
	"io"
	"net/http"

	sdk "github.com/manchtools/cadestro/contract"
	pb "github.com/manchtools/cadestro/contract/gen/go/cadestro/v1"
	"github.com/manchtools/cadestro/sdk/pkg"
	"github.com/manchtools/cadestro/sdk/sys/repo"
)

func (e *Executor) executeRepository(ctx context.Context, params *pb.RepositoryParams, state pb.DesiredState) (*pb.CommandOutput, bool, error) {
	e.ensureDeps()
	if params == nil {
		return nil, false, fmt.Errorf("repository params required")
	}

	if params.Name == "" {
		return nil, false, fmt.Errorf("repository name required")
	}

	switch e.pkgBackend {
	case pkg.Apt:
		if params.Apt == nil || params.Apt.Disabled {
			return nil, false, notApplicable("no APT repository configuration provided")
		}
	case pkg.Dnf, pkg.Dnf5:
		if params.Dnf == nil || params.Dnf.Disabled {
			return nil, false, notApplicable("no DNF repository configuration provided")
		}
	case pkg.Pacman:
		if params.Pacman == nil || params.Pacman.Disabled {
			return nil, false, notApplicable("no Pacman repository configuration provided")
		}
	case pkg.Zypper:
		if params.Zypper == nil || params.Zypper.Disabled {
			return nil, false, notApplicable("no Zypper repository configuration provided")
		}
	default:
		return nil, false, fmt.Errorf("no supported package manager found for repository configuration")
	}

	mgr, err := repo.New(e.pkgBackend, e.runner)
	if err != nil {
		return nil, false, fmt.Errorf("no supported package manager found for repository configuration")
	}

	if err := mgr.Validate(e.repositoryFields(params)); err != nil {
		return nil, false, err
	}

	if out, err := e.requireWritableFS(ctx); err != nil {
		return out, false, err
	}

	switch state {
	case pb.DesiredState_DESIRED_STATE_ABSENT:
		outcome, rerr := mgr.Remove(ctx, params.Name)
		return repoOutcome(outcome, rerr)
	case pb.DesiredState_DESIRED_STATE_PRESENT:
		r, berr := e.repositoryConfig(ctx, params)
		if berr != nil {
			return nil, false, berr
		}
		outcome, rerr := mgr.Apply(ctx, r)
		return repoOutcome(outcome, rerr)
	default:
		return nil, false, fmt.Errorf("unknown desired state: %v", state)
	}
}

func (e *Executor) repositoryFields(params *pb.RepositoryParams) repo.Repository {
	r := repo.Repository{Name: params.Name}
	switch e.pkgBackend {
	case pkg.Apt:
		a := params.Apt
		r.Apt = &repo.AptConfig{
			URL:          a.Url,
			Distribution: a.Distribution,
			Components:   a.Components,
			Arch:         a.Arch,
			Trusted:      a.Trusted,
		}
	case pkg.Dnf, pkg.Dnf5:
		d := params.Dnf
		r.Dnf = &repo.DnfConfig{
			BaseURL:        d.Baseurl,
			Description:    d.Description,
			Enabled:        d.Enabled,
			GPGCheck:       d.Gpgcheck,
			GPGKey:         d.Gpgkey,
			ModuleHotfixes: d.ModuleHotfixes,
		}
	case pkg.Pacman:
		p := params.Pacman
		r.Pacman = &repo.PacmanConfig{
			Server:   p.Server,
			SigLevel: p.SigLevel,
		}
	case pkg.Zypper:
		z := params.Zypper
		r.Zypper = &repo.ZypperConfig{
			URL:         z.Url,
			Description: z.Description,
			Enabled:     z.Enabled,
			Autorefresh: z.Autorefresh,
			GPGCheck:    z.Gpgcheck,
			GPGKey:      z.Gpgkey,
			Type:        z.Type,
		}
	}
	return r
}

func (e *Executor) repositoryConfig(ctx context.Context, params *pb.RepositoryParams) (repo.Repository, error) {
	r := e.repositoryFields(params)
	if e.pkgBackend == pkg.Apt && r.Apt != nil {
		switch a := params.Apt; {
		case a.GpgKeyUrl != "":
			key, err := e.downloadAptKey(ctx, a.GpgKeyUrl)
			if err != nil {
				return repo.Repository{}, err
			}
			r.Apt.GPGKey = key
		case a.GpgKey != "":
			r.Apt.GPGKey = []byte(a.GpgKey)
		}
	}
	return r, nil
}

func (e *Executor) downloadAptKey(ctx context.Context, keyURL string) ([]byte, error) {
	if err := sdk.ValidateHTTPSURL(keyURL); err != nil {
		return nil, fmt.Errorf("GPG key URL rejected: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, "GET", keyURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create GPG key request: %w", err)
	}
	resp, err := e.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to download GPG key: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GPG key download failed: HTTP %d", resp.StatusCode)
	}

	const maxGPGKeySize = 10 << 20
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxGPGKeySize+1))
	if err != nil {
		return nil, fmt.Errorf("failed to read GPG key response: %w", err)
	}
	if len(raw) > maxGPGKeySize {
		return nil, fmt.Errorf("GPG key exceeds the %d-byte limit", maxGPGKeySize)
	}
	return raw, nil
}

func repoOutcome(o repo.Outcome, err error) (*pb.CommandOutput, bool, error) {
	out := &pb.CommandOutput{
		ExitCode: int32(o.Result.ExitCode),
		Stdout:   o.Result.Stdout,
		Stderr:   o.Result.Stderr,
	}
	if err != nil {
		return out, false, err
	}
	return out, o.Changed, nil
}
