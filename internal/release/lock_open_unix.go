//go:build unix

package release

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

func openLockFile(path string) (*os.File, error) {
	if err := validateLockFilePath(path); err != nil {
		return nil, err
	}
	dir := filepath.Dir(path)
	if err := prepareLockDirectory(dir); err != nil {
		return nil, err
	}
	fd, err := syscall.Open(path, syscall.O_RDWR|syscall.O_CREAT|syscall.O_NOFOLLOW, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open lock file: %w", err)
	}
	return os.NewFile(uintptr(fd), path), nil
}
