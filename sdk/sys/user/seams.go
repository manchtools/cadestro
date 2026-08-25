package user

import (
	"context"
	"os"

	"github.com/manchtools/cadestro/sdk/sys/exec"
	"github.com/manchtools/cadestro/sdk/sys/fs"
)

type fsManager interface {
	WriteFile(ctx context.Context, path string, data []byte, opts fs.WriteOptions) error
	Remove(ctx context.Context, path string) error
	SetOwnershipRecursive(ctx context.Context, path, owner, group string) error
	Exists(ctx context.Context, path string) (bool, error)
	Mkdir(ctx context.Context, path string, opts fs.MkdirOptions) error
	CopyTree(ctx context.Context, src, dst string, opts fs.WriteOptions) error
	SetMode(ctx context.Context, path string, mode os.FileMode) error
}

var newFS = func(r exec.Runner) (fsManager, error) { return fs.New(r) }

var accountsServiceDir = "/var/lib/AccountsService/users"
