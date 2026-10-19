package monitor

import (
	"time"

	"github.com/faridze/pve-zfs-monitor/internal/device"
)

func deviceSignals(report *device.DeviceReport, ok bool, prev *Baselines, at time.Time, opts DetectionOptions) ([]Signal, map[string]DeviceCounters, error) {
	coverageKey, _ := Key("host", "device-smart-coverage")
	if !ok || report == nil {
		return []Signal{{Key: coverageKey, Level: Unknown, Verified: false, ObservedAt: at}}, nil, nil
	}
	coverageLevel := Healthy
	coverageVerified := true
	switch report.SmartCompleteness {
	case device.ReportSmartIncomplete, device.ReportInventoryIncomplete, device.ReportInventoryFailed:
		coverageLevel = Unknown
		coverageVerified = false
	case device.ReportSmartNotAttempted:
		coverageLevel = Unknown
		coverageVerified = false
	}
	signals := []Signal{{Key: coverageKey, Level: coverageLevel, Verified: coverageVerified, ObservedAt: at}}
	physByPath := make(map[string]device.PhysicalIdentity)
	for _, p := range report.Inventory {
		if p.DevicePath != "" {
			physByPath[p.DevicePath] = p
		}
	}
	updates := make(map[string]DeviceCounters)
	seen := make(map[string]bool)
	for _, h := range report.DeviceHealth {
		phys := physByPath[h.DevicePath]
		id, err := deviceIdentity(&phys)
		if err != nil {
			continue
		}
		key, err := Key("device", "smart:"+id)
		if err != nil {
			continue
		}
		if seen[key] {
			// Same physical identity must not be evaluated twice from duplicate paths.
			signals = append(signals, Signal{Key: key, Level: Unknown, Verified: false, ObservedAt: at})
			continue
		}
		seen[key] = true
		sig, cur, update := evalDeviceHealth(key, h, prev, at, opts)
		signals = append(signals, sig)
		if update {
			updates[key] = cur
		}
	}
	return signals, updates, nil
}

func evalDeviceHealth(key string, h device.DeviceHealth, prev *Baselines, at time.Time, opts DetectionOptions) (Signal, DeviceCounters, bool) {
	verified := true
	level := Healthy

	switch h.CollectionStatus {
	case device.HealthCollected:
	case device.HealthPartial:
		level = Unknown
		verified = false
	case device.HealthConflict:
		level = Unknown
		verified = false
	default:
		return Signal{Key: key, Level: Unknown, Verified: false, ObservedAt: at}, DeviceCounters{}, false
	}

	if h.SmartPassed != nil && !*h.SmartPassed {
		level = Critical
	}
	if h.CriticalWarning != nil && *h.CriticalWarning != 0 {
		if level == Healthy {
			level = Warning
		}
	}
	if h.AvailableSpare != nil && h.SpareThreshold != nil &&
		h.AvailableSpare.Value != nil && h.SpareThreshold.Value != nil &&
		*h.AvailableSpare.Value < *h.SpareThreshold.Value {
		if level == Healthy {
			level = Warning
		}
	}
	if opts.MaxPercentageUsed > 0 && h.PercentageUsed != nil && h.PercentageUsed.Value != nil &&
		int(*h.PercentageUsed.Value) >= opts.MaxPercentageUsed {
		if level == Healthy {
			level = Warning
		}
	}
	if opts.MaxTemperatureCelsius > 0 && h.Temperature != nil && h.Temperature.Value != nil &&
		int(*h.Temperature.Value) >= opts.MaxTemperatureCelsius {
		if level == Healthy {
			level = Warning
		}
	}

	cur := DeviceCounters{
		MediaErrors:         numericInt64(h.MediaErrors),
		DataIntegrityErrors: numericInt64(h.DataIntegrityErrors),
		ErrorLogEntries:     numericInt64(h.ErrorLogEntries),
	}
	if prev != nil {
		old := prev.Devices[key]
		for _, d := range []counterDelta{
			int64Delta(old.MediaErrors, cur.MediaErrors),
			int64Delta(old.DataIntegrityErrors, cur.DataIntegrityErrors),
			int64Delta(old.ErrorLogEntries, cur.ErrorLogEntries),
		} {
			switch d {
			case counterIncreased:
				if level == Healthy {
					level = Warning
				}
			case counterDecreased, counterReset:
				level = Unknown
				verified = false
			case counterMissing:
				if level == Healthy {
					level = Unknown
					verified = false
				}
			}
		}
	}

	update := h.CollectionStatus == device.HealthCollected && verified
	return Signal{Key: key, Level: level, Verified: verified, ObservedAt: at}, cur, update
}

func numericInt64(n *device.SmartNumeric) *int64 {
	if n == nil || n.Value == nil {
		return nil
	}
	return n.Value
}
