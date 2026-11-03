package labrun

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// ValidateLabStateDir ensures an existing private lab state directory.
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
	st, err := os.Stat(clean)
	if err != nil {
		if os.IsNotExist(err) {
			return errors.New("state directory must exist and be pre-created for lab use")
		}
		return err
	}
	if !st.IsDir() {
		return errors.New("state directory must be a directory")
	}
	if st.Mode()&0o077 != 0 {
		return fmt.Errorf("state directory must be mode 0700 (got %o)", st.Mode()&0o777)
	}
	return nil
}
