package release

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

func isProductionConfigDir() bool {
	dir := ConfigDir()
	return dir == defaultConfigDir || strings.HasPrefix(filepath.Clean(dir), "/etc/")
}

func assertSecureConfigPath(path string) error {
	path = filepath.Clean(path)
	if err := rejectSymlink(path); err != nil {
		return err
	}
	fi, err := os.Stat(path)
	if err != nil {
		return err
	}
	if !fi.Mode().IsRegular() {
		return fmt.Errorf("%s must be a regular file", path)
	}
	if fi.Mode().Perm()&0o022 != 0 {
		return fmt.Errorf("%s must not be group/world writable", path)
	}
	if err := assertSecureParentDir(filepath.Dir(path)); err != nil {
		return err
	}
	if isProductionConfigDir() {
		if err := assertFileOwnedByRoot(fi); err != nil {
			return err
		}
	}
	return nil
}

func assertSecureParentDir(dir string) error {
	dir = filepath.Clean(dir)
	if err := rejectSymlink(dir); err != nil {
		return err
	}
	fi, err := os.Stat(dir)
	if err != nil {
		return err
	}
	if !fi.IsDir() {
		return fmt.Errorf("%s must be a directory", dir)
	}
	if fi.Mode().Perm()&0o022 != 0 {
		return fmt.Errorf("%s must not be group/world writable", dir)
	}
	if isProductionConfigDir() {
		if err := assertFileOwnedByRoot(fi); err != nil {
			return err
		}
	}
	return nil
}

func rejectSymlink(path string) error {
	fi, err := os.Lstat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if fi.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("refusing symlink at %q", path)
	}
	return nil
}

func assertFileOwnedByRoot(fi os.FileInfo) error {
	sys, ok := fi.Sys().(*syscall.Stat_t)
	if !ok {
		return fmt.Errorf("cannot determine file ownership")
	}
	if sys.Uid != 0 {
		return fmt.Errorf("must be owned by root (uid 0)")
	}
	return nil
}

func finalizeConfigFileMode(path string) error {
	if err := os.Chmod(path, 0o644); err != nil {
		return err
	}
	if os.Getuid() == 0 {
		if err := os.Chown(path, 0, 0); err != nil {
			return err
		}
	}
	return assertSecureConfigPath(path)
}
