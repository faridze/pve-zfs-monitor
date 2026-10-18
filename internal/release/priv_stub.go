//go:build !unix

package release

// HasRootPrivileges on non-Unix is always false.
func HasRootPrivileges() bool {
	return false
}
