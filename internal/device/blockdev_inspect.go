package device

import "fmt"

// BlockDevInspector validates that a device path is a live block special node consistent with sysfs.
type BlockDevInspector interface {
	VerifyLiveBlockDevice(devicePath string, kernelName string) error
}

// MapBlockInspector is an injectable inspector for tests (no real /dev or sysfs required).
type MapBlockInspector map[string]BlockSpec

// BlockSpec describes expected live block identity for a device path.
type BlockSpec struct {
	KernelName string
	Major      uint32
	Minor      uint32
	Block      bool
}

func (m MapBlockInspector) VerifyLiveBlockDevice(devicePath string, kernelName string) error {
	spec, ok := m[devicePath]
	if !ok {
		return fmt.Errorf("no test block spec for %q", devicePath)
	}
	if !spec.Block {
		return fmt.Errorf("%q is not a block device in test fixture", devicePath)
	}
	if spec.KernelName != "" && spec.KernelName != kernelName {
		return fmt.Errorf("kernel name mismatch: live %q expected %q", kernelName, spec.KernelName)
	}
	return nil
}
