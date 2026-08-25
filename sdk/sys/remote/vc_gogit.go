package remote

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/storage/memory"
)

type goGitBackend struct{}

func init() {
	RegisterVersionControlBackend("go-git", goGitBackend{})
}

// CloneOrSync brings the repo at cfg.URL to dest, checked out at the
// configured ref. On a fresh dest it clones from scratch; on a
// re-existing dest it fetches the latest refs and checks out the
// target.
//
// cfg.Ref can be a branch, tag, or full commit SHA. The clone path
// deliberately does NOT pre-pin a ReferenceName: that would lock the
// clone to refs/heads/<ref>, which fails for tags and SHAs. Instead
// the clone fetches every ref (including tags), then resolveTargetHash
// converts cfg.Ref to a plumbing.Hash post-clone — the only path that
// handles all three ref shapes uniformly.
//
// Result.Revision is the commit SHA dest points at after the operation;
// Result.Changed is true on the first clone and on any sync that
// advanced HEAD, false when the previously checked-out commit already
// matches upstream.
func (goGitBackend) CloneOrSync(ctx context.Context, cfg GitConfig, dest string) (Result, error) {
	repo, fresh, err := openOrClone(ctx, cfg, dest)
	if err != nil {
		return Result{}, err
	}

	if !fresh {
		if err := goGitFetch(ctx, repo); err != nil && !errors.Is(err, gogit.NoErrAlreadyUpToDate) {
			return Result{}, fmt.Errorf("fetch %s: %w", cfg.URL, err)
		}
	}

	target, err := resolveTargetHash(repo, cfg.Ref)
	if err != nil {

		if fresh {
			_ = os.RemoveAll(dest)
		}
		return Result{}, err
	}

	prevHead, headErr := repo.Head()
	if !fresh && headErr == nil && prevHead.Hash() == target {

		if cfg.Prune {
			wt, werr := repo.Worktree()
			if werr == nil {
				_ = wt.Clean(&gogit.CleanOptions{Dir: true})
			}
		}
		return Result{Changed: false, Revision: target.String()}, nil
	}

	wt, err := repo.Worktree()
	if err != nil {
		return Result{}, fmt.Errorf("worktree: %w", err)
	}

	var snapshot []untrackedFile
	if !fresh && !cfg.Prune {
		snapshot, _ = snapshotUntracked(dest, wt)
	}

	if err := wt.Checkout(&gogit.CheckoutOptions{Hash: target, Force: true}); err != nil {
		if fresh {
			_ = os.RemoveAll(dest)
		}
		return Result{}, fmt.Errorf("checkout %s: %w", target, err)
	}

	if len(snapshot) > 0 {
		if err := restoreUntracked(dest, snapshot); err != nil {
			return Result{}, fmt.Errorf("restore untracked: %w", err)
		}
	}

	if cfg.Prune {
		if err := wt.Clean(&gogit.CleanOptions{Dir: true}); err != nil {
			return Result{}, fmt.Errorf("clean: %w", err)
		}
	}

	files, total, _ := countTreeFiles(dest)

	return Result{
		Changed:      true,
		BytesWritten: total,
		FilesTouched: files,
		Revision:     target.String(),
	}, nil
}

// Resolve returns the upstream SHA the configured ref points at, without
// touching dest. Uses an in-memory storer so the call has no on-disk
// side effects — same pattern as `git ls-remote`.
func (goGitBackend) Resolve(ctx context.Context, cfg GitConfig) (string, error) {
	rem := gogit.NewRemote(memory.NewStorage(), &config.RemoteConfig{
		Name: "origin",
		URLs: []string{cfg.URL},
	})
	refs, err := rem.ListContext(ctx, &gogit.ListOptions{})
	if err != nil {
		return "", fmt.Errorf("ls-remote %s: %w", cfg.URL, err)
	}
	hash, ok := matchRef(refs, cfg.Ref)
	if !ok {
		return "", fmt.Errorf("%w: ref %q not found at %s", ErrInvalidConfig, cfg.Ref, cfg.URL)
	}
	return hash.String(), nil
}

func openOrClone(ctx context.Context, cfg GitConfig, dest string) (*gogit.Repository, bool, error) {
	if _, err := os.Stat(filepath.Join(dest, ".git")); err == nil {
		repo, oerr := gogit.PlainOpen(dest)
		if oerr != nil {
			return nil, false, fmt.Errorf("open existing %s: %w", dest, oerr)
		}
		return repo, false, nil
	}

	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return nil, false, fmt.Errorf("mkdir parent of %s: %w", dest, err)
	}

	opts := &gogit.CloneOptions{
		URL:               cfg.URL,
		Tags:              gogit.AllTags,
		RecurseSubmodules: gogit.NoRecurseSubmodules,
	}
	if cfg.Submodules {
		opts.RecurseSubmodules = gogit.DefaultSubmoduleRecursionDepth
	}
	repo, err := gogit.PlainCloneContext(ctx, dest, false, opts)
	if err != nil {

		_ = os.RemoveAll(dest)
		return nil, false, fmt.Errorf("clone %s: %w", cfg.URL, err)
	}
	return repo, true, nil
}

func goGitFetch(ctx context.Context, repo *gogit.Repository) error {
	return repo.FetchContext(ctx, &gogit.FetchOptions{
		RemoteName: "origin",
		RefSpecs: []config.RefSpec{
			"+refs/heads/*:refs/remotes/origin/*",
			"+refs/tags/*:refs/tags/*",
		},
		Tags:  gogit.AllTags,
		Force: true,
	})
}

func resolveTargetHash(repo *gogit.Repository, ref string) (plumbing.Hash, error) {

	for _, candidate := range []string{
		"refs/remotes/origin/" + ref,
		"refs/heads/" + ref,
		"refs/tags/" + ref,
	} {
		if r, err := repo.Reference(plumbing.ReferenceName(candidate), true); err == nil {
			return r.Hash(), nil
		}
	}

	if h := plumbing.NewHash(ref); !h.IsZero() {
		if _, err := repo.CommitObject(h); err == nil {
			return h, nil
		}
	}
	return plumbing.ZeroHash, fmt.Errorf("%w: ref %q not found", ErrInvalidConfig, ref)
}

func matchRef(refs []*plumbing.Reference, ref string) (plumbing.Hash, bool) {
	for _, r := range refs {
		switch r.Name().String() {
		case "refs/heads/" + ref, "refs/tags/" + ref:
			return r.Hash(), true
		}
	}

	for _, r := range refs {
		if r.Name().String() == "refs/tags/"+ref+"^{}" {
			return r.Hash(), true
		}
	}
	return plumbing.ZeroHash, false
}

func countTreeFiles(dest string) (int, int64, error) {
	var files int
	var total int64
	err := filepath.WalkDir(dest, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && d.Name() == ".git" && path != dest {
			return filepath.SkipDir
		}
		if d.Type().IsRegular() {
			files++
			info, ierr := d.Info()
			if ierr == nil {
				total += info.Size()
			}
		}
		return nil
	})
	return files, total, err
}

type untrackedFile struct {
	relPath string
	body    []byte
	mode    os.FileMode
}

func snapshotUntracked(dest string, wt *gogit.Worktree) ([]untrackedFile, error) {
	st, err := wt.Status()
	if err != nil {
		return nil, fmt.Errorf("status: %w", err)
	}
	var out []untrackedFile
	for relPath, entry := range st {
		if entry.Worktree != gogit.Untracked {
			continue
		}
		full := filepath.Join(dest, relPath)
		info, ierr := os.Lstat(full)
		if ierr != nil {
			continue
		}
		if !info.Mode().IsRegular() {
			continue
		}
		body, rerr := os.ReadFile(full)
		if rerr != nil {
			continue
		}
		out = append(out, untrackedFile{
			relPath: relPath,
			body:    body,
			mode:    info.Mode().Perm(),
		})
	}
	return out, nil
}

func restoreUntracked(dest string, snap []untrackedFile) error {
	for _, f := range snap {
		full := filepath.Join(dest, f.relPath)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			return fmt.Errorf("mkdir %s: %w", filepath.Dir(full), err)
		}
		if err := os.WriteFile(full, f.body, f.mode); err != nil {
			return fmt.Errorf("write %s: %w", full, err)
		}
	}
	return nil
}
