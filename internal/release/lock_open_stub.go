//go:build !unix

package release

import (
	"os"
	"path/filepath"
)

func openLockFile(path string) (*os.File, error) {
	if err := validateLockFilePath(path); err != nil {
		return nil, err
	}
	dir := filepath.Dir(path)
	if err := prepareLockDirectory(dir); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	return f, nil
}
