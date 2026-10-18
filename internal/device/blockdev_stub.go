//go:build !linux

package device

import "fmt"

type linuxSysfsInspector struct{}

func (linuxSysfsInspector) VerifyLiveBlockDevice(devicePath string, kernelName string) error {
	return fmt.Errorf("live block device verification requires linux (path %q)", devicePath)
}
