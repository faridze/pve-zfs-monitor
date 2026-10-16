package release

import "os"

// HermeticTestEnabled is set only in CI/hermetic fixtures; production installs must not rely on it.
func HermeticTestEnabled() bool {
	return os.Getenv("PVE_ZFS_MONITOR_HERMETIC_TEST") == "1"
}
