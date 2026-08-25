package firewall

import (
	"context"
	"os"

	"github.com/manchtools/cadestro/sdk/sys/exec"
	"github.com/manchtools/cadestro/sdk/sys/fs"
)

type fsManager interface {
	WriteFile(ctx context.Context, path string, data []byte, opts fs.WriteOptions) error
	Remove(ctx context.Context, path string) error
	// WriteFileExclusive writes only if the path is absent, reporting fs.ErrExists
	// otherwise. ApplyRule uses it to learn — atomically, as part of the write
	// itself — whether THIS call created the service XML, because only a file we
	// created may be deleted when a later step fails. An Exists probe followed by
	// a WriteFile cannot answer that: a foreign definition landing in the gap
	// would be overwritten and then deleted.
	WriteFileExclusive(ctx context.Context, path string, data []byte, opts fs.WriteOptions) error
}

var newFS = func(r exec.Runner) (fsManager, error) { return fs.New(r) }

var readFile = os.ReadFile
