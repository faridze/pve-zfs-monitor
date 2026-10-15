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
)

// InstallOptions controls atomic installation behavior.
type InstallOptions struct {
	TargetPath   string
	RollbackPath string
	ExpectedName string // basename guard for symlink targets
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

// ValidateInstallTarget ensures we do not clobber package paths or unexpected symlinks.
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
		link, err := os.Readlink(target)
		if err != nil {
			return err
		}
		base := filepath.Base(link)
		if base != "pve-zfs-monitor" && !strings.Contains(link, "pve-zfs-monitor") {
			return fmt.Errorf("refusing to replace unexpected symlink %q -> %q", target, link)
		}
	}
	return nil
}

// AtomicInstall replaces target with stagedBinary, preserving previous at rollbackPath.
func AtomicInstall(stagedBinary string, opts InstallOptions) error {
	if err := ValidateInstallTarget(opts.TargetPath); err != nil {
		return err
	}
	if err := os.Chmod(stagedBinary, 0o755); err != nil {
		return err
	}
	dir := filepath.Dir(opts.TargetPath)
	staging := filepath.Join(dir, "."+opts.ExpectedName+".staging-"+filepath.Base(stagedBinary))
	if err := copyFile(stagedBinary, staging); err != nil {
		return err
	}
	if err := os.Chmod(staging, 0o755); err != nil {
		_ = os.Remove(staging)
		return err
	}
	if _, err := os.Stat(opts.TargetPath); err == nil {
		_ = os.Remove(opts.RollbackPath)
		if err := os.Rename(opts.TargetPath, opts.RollbackPath); err != nil {
			_ = os.Remove(staging)
			return fmt.Errorf("rollback preserve failed: %w", err)
		}
	}
	if err := os.Rename(staging, opts.TargetPath); err != nil {
		// attempt restore
		if _, statErr := os.Stat(opts.RollbackPath); statErr == nil {
			_ = os.Rename(opts.RollbackPath, opts.TargetPath)
		}
		_ = os.Remove(staging)
		return err
	}
	return nil
}

// Rollback restores the previous binary if present.
func Rollback(opts InstallOptions) error {
	if _, err := os.Stat(opts.RollbackPath); err != nil {
		return fmt.Errorf("no rollback binary at %s", opts.RollbackPath)
	}
	tmp := opts.TargetPath + ".rollback-tmp"
	_ = os.Remove(tmp)
	if err := os.Rename(opts.TargetPath, tmp); err != nil && !os.IsNotExist(err) {
		return err
	}
	if err := os.Rename(opts.RollbackPath, opts.TargetPath); err != nil {
		_ = os.Rename(tmp, opts.TargetPath)
		return err
	}
	_ = os.Remove(tmp)
	return nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
	if err != nil {
		return err
	}
	defer out.Close()
	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Close()
}
