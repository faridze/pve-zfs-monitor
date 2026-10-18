//go:build unix

package release

import "syscall"

// HasRootPrivileges reports whether the process can install system binaries.
func HasRootPrivileges() bool {
	return syscall.Geteuid() == 0
}
