package monitor

import (
	"testing"

	"github.com/faridze/pve-zfs-monitor/internal/device"
)

func TestEvalDeviceHealthSMARTEvidenceTable(t *testing.T) {
	key := "device:test"
	at := baseTime
	zero := int64(0)
	five := int64(5)
	pct := int64(90)
	spare := int64(10)
	temp := int64(55)
	passedTrue := true
	passedFalse := false
	cw := uint8(4)

	ancillaryOnly := []device.DeviceHealth{
		{CollectionStatus: device.HealthCollected, Temperature: &device.SmartNumeric{Value: &temp}},
		{CollectionStatus: device.HealthCollected, PercentageUsed: &device.SmartNumeric{Value: &pct}},
		{CollectionStatus: device.HealthCollected, AvailableSpare: &device.SmartNumeric{Value: &spare}},
		{CollectionStatus: device.HealthCollected, ErrorLogEntries: &device.SmartNumeric{Value: &zero}},
		{CollectionStatus: device.HealthCollected, MediaErrors: &device.SmartNumeric{Value: &zero}},
		{CollectionStatus: device.HealthCollected, SmartStatusString: "vendor-opaque-status"},
	}

	for _, h := range ancillaryOnly {
		sig, _, _, _ := evalDeviceHealth(key, h, nil, at, DetectionOptions{})
		if sig.Level == Healthy && sig.Verified {
			t.Fatalf("ancillary-only must not be verified healthy: %#v -> %+v", h, sig)
		}
	}

	validPass := device.DeviceHealth{
		CollectionStatus: device.HealthCollected,
		SmartPassed:      &passedTrue,
	}
	sig, _, update, _ := evalDeviceHealth(key, validPass, nil, at, DetectionOptions{})
	if sig.Level != Healthy || !sig.Verified || !update {
		t.Fatalf("validated SMART pass expected healthy verified: %+v", sig)
	}

	nvmePass := device.DeviceHealth{
		CollectionStatus: device.HealthCollected,
		CriticalWarning:  func() *uint8 { z := uint8(0); return &z }(),
	}
	sig, _, _, _ = evalDeviceHealth(key, nvmePass, nil, at, DetectionOptions{})
	if sig.Level != Healthy || !sig.Verified {
		t.Fatalf("NVMe critical_warning=0 expected verified healthy: %+v", sig)
	}

	failSmart := device.DeviceHealth{
		CollectionStatus: device.HealthCollected,
		SmartPassed:      &passedFalse,
	}
	sig, _, _, _ = evalDeviceHealth(key, failSmart, nil, at, DetectionOptions{})
	if sig.Level != Critical || !sig.Verified {
		t.Fatalf("SmartPassed false must stay verified critical: %+v", sig)
	}

	failNVMe := device.DeviceHealth{
		CollectionStatus: device.HealthCollected,
		CriticalWarning:  &cw,
	}
	sig, _, _, _ = evalDeviceHealth(key, failNVMe, nil, at, DetectionOptions{})
	if sig.Level != Warning || !sig.Verified {
		t.Fatalf("critical_warning must be verified warning: %+v", sig)
	}

	mediaOnly := device.DeviceHealth{
		CollectionStatus: device.HealthCollected,
		MediaErrors:      &device.SmartNumeric{Value: &five},
	}
	sig, _, _, _ = evalDeviceHealth(key, mediaOnly, nil, at, DetectionOptions{})
	if sig.Level == Healthy && sig.Verified {
		t.Fatalf("non-zero media errors must not be verified healthy: %+v", sig)
	}

	logOnly := device.DeviceHealth{
		CollectionStatus: device.HealthCollected,
		ErrorLogEntries:  &device.SmartNumeric{Value: &five},
	}
	sig, _, _, _ = evalDeviceHealth(key, logOnly, nil, at, DetectionOptions{})
	if sig.Level == Healthy && sig.Verified {
		t.Fatalf("error log count alone must not be verified healthy: %+v", sig)
	}
}

func TestEvalDeviceHealthThresholdWarningWithoutOverallPass(t *testing.T) {
	key := "device:test"
	at := baseTime
	temp := int64(55)
	h := device.DeviceHealth{
		CollectionStatus: device.HealthCollected,
		Temperature:      &device.SmartNumeric{Value: &temp},
	}
	sig, _, _, _ := evalDeviceHealth(key, h, nil, at, DetectionOptions{MaxTemperatureCelsius: 50})
	if sig.Level != Warning || !sig.Verified {
		t.Fatalf("opt-in temperature threshold should warn with verification: %+v", sig)
	}
}
