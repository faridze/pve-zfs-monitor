package release

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	DefaultLockDir  = "/run/lock/pve-zfs-monitor"
	DefaultLockPath = DefaultLockDir + "/install.lock"
)

func installLockPath() string {
	if HermeticTestEnabled() {
		if p := strings.TrimSpace(os.Getenv("PVE_ZFS_MONITOR_LOCK_PATH")); p != "" {
			return filepath.Clean(p)
		}
	}
	return DefaultLockPath
}

func validateLockFilePath(path string) error {
	path = filepath.Clean(path)
	if !HermeticTestEnabled() {
		if path != DefaultLockPath {
			return fmt.Errorf("refusing non-default lock path %q", path)
		}
	}
	if err := rejectSymlink(path); err != nil {
		return err
	}
	dir := filepath.Clean(filepath.Dir(path))
	if err := rejectSymlink(dir); err != nil {
		return err
	}
	if !HermeticTestEnabled() && dir != DefaultLockDir {
		return fmt.Errorf("refusing non-default lock directory %q", dir)
	}
	return nil
}

func prepareLockDirectory(dir string) error {
	dir = filepath.Clean(dir)
	if err := rejectSymlink(dir); err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	if !HermeticTestEnabled() {
		if err := assertSecureParentDir(dir); err != nil {
			return err
		}
	}
	return nil
}
