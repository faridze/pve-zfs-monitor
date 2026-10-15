package release

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const (
	DefaultInstallPath = "/usr/local/bin/pve-zfs-monitor"
	RollbackSuffix     = ".previous"
	DefaultLockPath    = "/run/lock/pve-zfs-monitor.lock"
)

// InstallOptions controls atomic installation behavior.
type InstallOptions struct {
	TargetPath   string
	RollbackPath string
	ExpectedName string
}

// DefaultInstallOptions returns standard /usr/local/bin layout.
func DefaultInstallOptions() InstallOptions {
	target := DefaultInstallPath
	return InstallOptions{
		TargetPath:   target,
		RollbackPath: target + RollbackSuffix,
		ExpectedName: "pve-zfs-monitor",
	}
}

func installLockPath() string {
	if p := strings.TrimSpace(os.Getenv("PVE_ZFS_MONITOR_LOCK_PATH")); p != "" {
		return p
	}
	return DefaultLockPath
}

// ValidateInstallTarget ensures we do not clobber package paths or symlinks.
func ValidateInstallTarget(target string) error {
	target = filepath.Clean(target)
	if os.Getenv("PVE_ZFS_MONITOR_TEST_INSTALL") != "1" && target != DefaultInstallPath {
		return fmt.Errorf("refusing non-default install target %q (expected %s)", target, DefaultInstallPath)
	}
	if strings.HasPrefix(target, "/usr/bin/") || strings.HasPrefix(target, "/bin/") {
		return fmt.Errorf("refusing package-managed path %q", target)
	}
	fi, err := os.Lstat(target)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if fi.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("refusing to replace symlink at %q", target)
	}
	if !fi.Mode().IsRegular() {
		return fmt.Errorf("refusing non-regular file at %q", target)
	}
	return nil
}

// AtomicInstall activates a verified staged binary without removing the live executable first.
func AtomicInstall(stagedBinary string, opts InstallOptions) error {
	lock, err := AcquireUpdateLock(installLockPath())
	if err != nil {
		return err
	}
	defer lock.Release()
	return atomicInstallUnlocked(stagedBinary, opts)
}

func atomicInstallUnlocked(stagedBinary string, opts InstallOptions) error {
	if err := ValidateInstallTarget(opts.TargetPath); err != nil {
		return err
	}
	if err := os.Chmod(stagedBinary, 0o755); err != nil {
		return err
	}
	dir := filepath.Dir(opts.TargetPath)
	staging := filepath.Join(dir, "."+opts.ExpectedName+".staging")
	if err := copyFileAtomic(stagedBinary, staging); err != nil {
		return err
	}
	if err := syncFile(staging); err != nil {
		_ = os.Remove(staging)
		return err
	}
	if _, err := os.Stat(opts.TargetPath); err == nil {
		rollbackNew := opts.RollbackPath + ".new"
		if err := copyFileAtomic(opts.TargetPath, rollbackNew); err != nil {
			_ = os.Remove(staging)
			return fmt.Errorf("preserve rollback copy failed: %w", err)
		}
		if err := os.Rename(rollbackNew, opts.RollbackPath); err != nil {
			_ = os.Remove(staging)
			_ = os.Remove(rollbackNew)
			return err
		}
	}
	if err := os.Rename(staging, opts.TargetPath); err != nil {
		_ = os.Remove(staging)
		return err
	}
	return syncFile(opts.TargetPath)
}

// Rollback restores the previous binary if present.
func Rollback(opts InstallOptions) error {
	lock, err := AcquireUpdateLock(installLockPath())
	if err != nil {
		return err
	}
	defer lock.Release()

	if _, err := os.Stat(opts.RollbackPath); err != nil {
		return fmt.Errorf("no rollback binary at %s", opts.RollbackPath)
	}
	if err := ValidateInstallTarget(opts.TargetPath); err != nil && !os.IsNotExist(err) {
		if _, statErr := os.Stat(opts.TargetPath); statErr == nil {
			return err
		}
	}
	staging := opts.TargetPath + ".rollback-staging"
	if err := copyFileAtomic(opts.RollbackPath, staging); err != nil {
		return err
	}
	if err := os.Rename(staging, opts.TargetPath); err != nil {
		_ = os.Remove(staging)
		return err
	}
	return nil
}

func copyFileAtomic(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	tmp := dst + ".part"
	out, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		_ = os.Remove(tmp)
		return err
	}
	if err := out.Close(); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, dst)
}

func syncFile(path string) error {
	f, err := os.OpenFile(path, os.O_RDONLY, 0)
	if err != nil {
		return nil
	}
	defer f.Close()
	return f.Sync()
}

func copyFile(src, dst string) error {
	return copyFileAtomic(src, dst)
}
