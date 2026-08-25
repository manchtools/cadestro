package network

import (
	"os"
	"path/filepath"
)

var (
	mkdirAll   = os.MkdirAll
	writeFile  = os.WriteFile
	readFile   = os.ReadFile
	renameFile = os.Rename
	statFile   = os.Stat
	removeAll  = os.RemoveAll
	removeFile = os.Remove
	createTemp = func(dir, pattern string) (keyfileHandle, error) { return os.CreateTemp(dir, pattern) }
)

var (
	absPath      = filepath.Abs
	evalSymlinks = filepath.EvalSymlinks
	statResolve  = os.Stat
)

type keyfileHandle interface {
	Name() string
	Write([]byte) (int, error)
	Chmod(os.FileMode) error
	Close() error
}
