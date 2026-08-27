//go:build !unix

package fs

import (
	"context"
	"errors"
)

func removeDirSecure(_ context.Context, _ string) error {
	return errors.New("fs: secure directory removal is not supported on this platform")
}
