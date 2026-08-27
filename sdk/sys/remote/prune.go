package remote

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"syscall"
)

func pruneTo(dest string, keep map[string]struct{}) error {
	if err := canWipe(dest); err != nil {
		return err
	}
	if _, err := os.Stat(dest); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("stat dest %s: %w", dest, err)
	}

	var victims []string
	var dirs []string

	walkErr := filepath.WalkDir(dest, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path == dest {
			return nil
		}
		if d.IsDir() {

			dirs = append(dirs, path)
			return nil
		}
		rel, rerr := filepath.Rel(dest, path)
		if rerr != nil {
			return rerr
		}
		rel = filepath.ToSlash(rel)
		if _, ok := keep[rel]; !ok {
			victims = append(victims, path)
		}
		return nil
	})
	if walkErr != nil {
		return fmt.Errorf("walk %s: %w", dest, walkErr)
	}

	for _, v := range victims {
		if err := os.Remove(v); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("remove %s: %w", v, err)
		}
	}

	sort.Slice(dirs, func(i, j int) bool { return len(dirs[i]) > len(dirs[j]) })
	for _, d := range dirs {

		if err := os.Remove(d); err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}

			if !isNotEmptyErr(err) {
				return fmt.Errorf("rmdir %s: %w", d, err)
			}
		}
	}
	return nil
}

func isNotEmptyErr(err error) bool {
	return errors.Is(err, syscall.ENOTEMPTY)
}
