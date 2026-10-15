package device

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
)

var allowedDevPathRE = regexp.MustCompile(`^/dev/(?:disk/by-id/[-a-zA-Z0-9_.]+|nvme\d+n\d+(?:p\d+)?|[a-z]{2,}[a-z0-9]*(?:\d+)?)$`)

// ValidateDevicePath ensures path is an absolute /dev node we may pass to fixed tools.
func ValidateDevicePath(path string) error {
	path = filepath.Clean(path)
	if !strings.HasPrefix(path, "/dev/") {
		return fmt.Errorf("device path outside /dev: %q", path)
	}
	if strings.Contains(path, "..") {
		return fmt.Errorf("invalid device path: %q", path)
	}
	if !allowedDevPathRE.MatchString(path) {
		return fmt.Errorf("device path not in allowlist: %q", path)
	}
	return nil
}

// IsGUIDMember reports ZFS numeric GUID member identifiers.
func IsGUIDMember(name string) bool {
	if name == "" {
		return false
	}
	for _, r := range name {
		if r < '0' || r > '9' {
			return false
		}
	}
	return len(name) >= 10
}
