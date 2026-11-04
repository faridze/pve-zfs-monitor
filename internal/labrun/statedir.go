package labrun

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

const (
	// LabMarkerFileName must exist in the state directory before lab writes.
	LabMarkerFileName = ".pve-zfs-monitor-lab"
	// LabMarkerContent is the required marker payload (includes trailing newline).
	LabMarkerContent = "pve-zfs-monitor-lab-v1\n"
)

// ValidateLabStateDir ensures an existing private lab state directory (path and mode only).
func ValidateLabStateDir(path string) error {
	if path == "" {
		return errors.New("state directory required")
	}
	if !filepath.IsAbs(path) {
		return errors.New("state directory must be absolute")
	}
	clean := filepath.Clean(path)
	if clean != path || clean == "/" {
		return errors.New("state directory must be a clean absolute path")
	}
	st, err := os.Lstat(clean)
	if err != nil {
		if os.IsNotExist(err) {
			return errors.New("state directory must exist and be pre-created for lab use")
		}
		return err
	}
	if st.Mode()&os.ModeSymlink != 0 {
		return errors.New("state directory must not be a symlink")
	}
	if !st.IsDir() {
		return errors.New("state directory must be a directory")
	}
	if st.Mode()&0o077 != 0 {
		return fmt.Errorf("state directory must be mode 0700 (got %o)", st.Mode()&0o777)
	}
	return nil
}

// ValidateLabAuthorization verifies explicit lab marker authorization before durable writes.
func ValidateLabAuthorization(dir string) error {
	if err := ValidateLabStateDir(dir); err != nil {
		return err
	}
	markerPath := filepath.Join(dir, LabMarkerFileName)
	st, err := os.Lstat(markerPath)
	if err != nil {
		if os.IsNotExist(err) {
			return errors.New("state directory missing lab authorization marker")
		}
		return err
	}
	if st.Mode()&os.ModeSymlink != 0 {
		return errors.New("lab marker must not be a symlink")
	}
	if !st.Mode().IsRegular() {
		return errors.New("lab marker must be a regular file")
	}
	if st.Mode()&0o077 != 0 {
		return fmt.Errorf("lab marker must be mode 0600 (got %o)", st.Mode()&0o777)
	}
	data, err := os.ReadFile(markerPath)
	if err != nil {
		return err
	}
	if string(data) != LabMarkerContent {
		return errors.New("invalid lab authorization marker content")
	}
	return nil
}

// WriteLabMarker creates or replaces the lab authorization marker in an existing private directory.
func WriteLabMarker(dir string) error {
	if err := ValidateLabStateDir(dir); err != nil {
		return err
	}
	markerPath := filepath.Join(dir, LabMarkerFileName)
	return os.WriteFile(markerPath, []byte(LabMarkerContent), 0o600)
}
