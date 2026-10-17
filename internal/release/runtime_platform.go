package release

import "runtime"

// ExecutableGOOS returns the platform of the running binary.
func ExecutableGOOS() string {
	if BuildGOOS != "" && BuildGOOS != runtime.GOOS {
		return BuildGOOS
	}
	return runtime.GOOS
}

// ExecutableGOARCH returns the architecture of the running binary.
func ExecutableGOARCH() string {
	if BuildGOARCH != "" && BuildGOARCH != runtime.GOARCH {
		return BuildGOARCH
	}
	return runtime.GOARCH
}
