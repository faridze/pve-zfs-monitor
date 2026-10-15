package device

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
)

var (
	blockDevPathRE  = regexp.MustCompile(`^/dev/(?:disk/by-id/[-a-zA-Z0-9_.]+|nvme\d+n\d+(?:p\d+)?|sd[a-z]+(?:\d+)?|vd[a-z]+(?:\d+)?|hd[a-z]+(?:\d+)?)$`)
	nvmeNamespaceRE = regexp.MustCompile(`^nvme\d+n\d+(?:p\d+)?$`)
)

var blockedDevNames = map[string]bool{
	"null": true, "zero": true, "random": true, "urandom": true, "full": true,
}

// ValidateDevicePath ensures path is an absolute /dev block node we may pass to fixed tools.
func ValidateDevicePath(path string) error {
	path = filepath.Clean(path)
	if !strings.HasPrefix(path, "/dev/") {
		return fmt.Errorf("device path outside /dev: %q", path)
	}
	if strings.Contains(path, "..") {
		return fmt.Errorf("invalid device path: %q", path)
	}
	if !blockDevPathRE.MatchString(path) {
		return fmt.Errorf("device path not in allowlist: %q", path)
	}
	base := strings.TrimPrefix(path, "/dev/")
	if blockedDevNames[base] {
		return fmt.Errorf("device path blocked: %q", path)
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

// ByIDLinkName extracts the symlink name from a full by-id path.
func ByIDLinkName(path string) (string, bool) {
	const prefix = "/dev/disk/by-id/"
	path = filepath.Clean(path)
	if !strings.HasPrefix(path, prefix) {
		return "", false
	}
	name := strings.TrimPrefix(path, prefix)
	if name == "" || strings.Contains(name, "/") {
		return "", false
	}
	return name, true
}

func isKernelBlockName(name string) bool {
	if name == "" || strings.Contains(name, "/") {
		return false
	}
	if blockedDevNames[name] {
		return false
	}
	if strings.HasPrefix(name, "sd") || strings.HasPrefix(name, "vd") || strings.HasPrefix(name, "hd") {
		return len(name) >= 3
	}
	return nvmeNamespaceRE.MatchString(name)
}
