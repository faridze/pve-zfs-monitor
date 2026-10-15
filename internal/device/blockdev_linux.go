//go:build linux

package device

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

type linuxSysfsInspector struct{}

func (linuxSysfsInspector) VerifyLiveBlockDevice(devicePath string, kernelName string) error {
	if kernelName == "" {
		kernelName = strings.TrimPrefix(filepath.Clean(devicePath), "/dev/")
	}
	if kernelName == "" {
		return fmt.Errorf("missing kernel name for %q", devicePath)
	}
	maj, min, err := statBlockDeviceNumbers(devicePath)
	if err != nil {
		return err
	}
	sysMaj, sysMin, err := sysfsBlockNumbers(kernelName)
	if err != nil {
		return err
	}
	if maj != sysMaj || min != sysMin {
		return fmt.Errorf("device %q major:minor %d:%d does not match sysfs %d:%d", devicePath, maj, min, sysMaj, sysMin)
	}
	return nil
}

func statBlockDeviceNumbers(path string) (major, minor uint32, err error) {
	var st syscall.Stat_t
	if e := syscall.Stat(path, &st); e != nil {
		return 0, 0, fmt.Errorf("stat %q: %w", path, e)
	}
	if st.Mode&syscall.S_IFMT != syscall.S_IFBLK {
		return 0, 0, fmt.Errorf("%q is not a block special device", path)
	}
	maj, min := linuxDevMajorMinor(st.Rdev)
	return maj, min, nil
}

func sysfsBlockNumbers(kernelName string) (major, minor uint32, err error) {
	data, err := os.ReadFile(filepath.Join("/sys/class/block", kernelName, "dev"))
	if err != nil {
		return 0, 0, fmt.Errorf("read sysfs dev for %q: %w", kernelName, err)
	}
	parts := strings.SplitN(strings.TrimSpace(string(data)), ":", 2)
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("malformed sysfs dev for %q: %q", kernelName, data)
	}
	var maj64, min64 uint64
	if _, err := fmt.Sscanf(parts[0], "%d", &maj64); err != nil {
		return 0, 0, err
	}
	if _, err := fmt.Sscanf(parts[1], "%d", &min64); err != nil {
		return 0, 0, err
	}
	return uint32(maj64), uint32(min64), nil
}

func linuxDevMajorMinor(rdev uint64) (uint32, uint32) {
	major := uint32((rdev >> 8) & 0xfff)
	minor := uint32((rdev & 0xff) | ((rdev >> 12) & 0xfff00))
	return major, minor
}
