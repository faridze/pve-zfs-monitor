package device

import (
	"fmt"
	"path/filepath"
	"strings"
)

// DeviceVerifier checks that a device path is a live block device consistent with inventory.
type DeviceVerifier interface {
	VerifyBlockDevice(devicePath string, expect PhysicalIdentity) error
}

// OSDeviceVerifier uses sysfs and live /dev node type/major:minor on Linux.
type OSDeviceVerifier struct {
	Inspector BlockDevInspector
}

func (v OSDeviceVerifier) liveInspector() BlockDevInspector {
	if v.Inspector != nil {
		return v.Inspector
	}
	return linuxSysfsInspector{}
}

// VerifyBlockDevice ensures the path is inventory-consistent and a live block special device.
func (v OSDeviceVerifier) VerifyBlockDevice(devicePath string, expect PhysicalIdentity) error {
	kname := expect.KernelName
	if kname == "" {
		kname = strings.TrimPrefix(filepath.Clean(devicePath), "/dev/")
	}
	if kname == "" {
		return fmt.Errorf("missing kernel name for %q", devicePath)
	}
	if expect.DevicePath != "" && filepath.Clean(expect.DevicePath) != filepath.Clean(devicePath) {
		return fmt.Errorf("inventory path %q does not match target %q", expect.DevicePath, devicePath)
	}
	return v.liveInspector().VerifyLiveBlockDevice(filepath.Clean(devicePath), kname)
}

// StubDeviceVerifier accepts paths present in the provided inventory map (tests).
type StubDeviceVerifier struct {
	Allowed map[string]bool
}

func (s StubDeviceVerifier) VerifyBlockDevice(devicePath string, expect PhysicalIdentity) error {
	if s.Allowed != nil && !s.Allowed[devicePath] {
		return fmt.Errorf("stub verifier rejected %q", devicePath)
	}
	if expect.DevicePath != "" && expect.DevicePath != devicePath {
		return fmt.Errorf("identity mismatch")
	}
	return nil
}

// ResolveSMARTTarget picks the inventory-verified path for smartctl invocation.
func ResolveSMARTTarget(requested string, inv *Inventory, verifier DeviceVerifier) (string, error) {
	if err := ValidateDevicePath(requested); err != nil {
		return "", err
	}
	if inv == nil || len(inv.Devices) == 0 {
		return "", fmt.Errorf("inventory incomplete for SMART target resolution")
	}
	id, ok := inv.Devices[requested]
	if !ok {
		return "", fmt.Errorf("device path %q not in inventory", requested)
	}
	target := requested
	switch id.Kind {
	case BlockKindPart:
		if id.ParentPath == "" {
			return "", fmt.Errorf("partition %q missing verified parent in inventory", requested)
		}
		parent, ok := inv.Devices[id.ParentPath]
		if !ok {
			return "", fmt.Errorf("partition parent %q not in inventory", id.ParentPath)
		}
		target = parent.DevicePath
		id = parent
	case BlockKindDisk, BlockKindNamespace:
		// SMART on namespace/disk node directly.
	default:
		return "", fmt.Errorf("unsupported block kind %q for SMART on %q", id.Kind, requested)
	}
	if verifier == nil {
		return "", fmt.Errorf("device verifier required for SMART target %q", requested)
	}
	if err := verifier.VerifyBlockDevice(target, id); err != nil {
		return "", err
	}
	return target, nil
}
