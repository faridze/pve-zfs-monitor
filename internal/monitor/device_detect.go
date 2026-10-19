package monitor

import (
	"time"

	"github.com/faridze/pve-zfs-monitor/internal/device"
)

func deviceSignals(report *device.DeviceReport, ok bool, prev *Baselines, at time.Time, opts DetectionOptions) ([]Signal, map[string]DeviceCounters, []string, error) {
	coverageKey, _ := Key("host", "device-smart-coverage")
	if !ok || report == nil {
		return []Signal{{Key: coverageKey, Level: Unknown, Verified: false, ObservedAt: at}}, nil, nil, nil
	}
	coverageLevel, coverageVerified := deviceCoverageFromReport(report)
	byKey := map[string]Signal{coverageKey: {Key: coverageKey, Level: coverageLevel, Verified: coverageVerified, ObservedAt: at}}
	order := []string{coverageKey}
	physByPath := make(map[string]device.PhysicalIdentity)
	for _, p := range report.Inventory {
		if p.DevicePath != "" {
			physByPath[p.DevicePath] = p
		}
	}
	updates := make(map[string]DeviceCounters)
	var clearKeys []string
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
		sig, cur, update, clear := evalDeviceHealth(key, h, prev, at, opts)
		if existing, dup := byKey[key]; dup {
			sig = mergeAmbiguousSMARTSignals(existing, sig)
			update = false
			clear = true
			delete(updates, key)
		}
		if _, exists := byKey[key]; !exists {
			order = append(order, key)
		}
		byKey[key] = sig
		if update {
			updates[key] = cur
		}
		if clear {
			clearKeys = append(clearKeys, key)
		}
	}
	signals := make([]Signal, 0, len(order))
	for _, k := range order {
		signals = append(signals, byKey[k])
	}
	return signals, updates, clearKeys, nil
}

func mergeAmbiguousSMARTSignals(a, b Signal) Signal {
	key := a.Key
	at := a.ObservedAt
	if b.ObservedAt.After(at) {
		at = b.ObservedAt
	}
	if a.Level == Critical && a.Verified {
		return Signal{Key: key, Level: Critical, Verified: true, ObservedAt: at}
	}
	if b.Level == Critical && b.Verified {
		return Signal{Key: key, Level: Critical, Verified: true, ObservedAt: at}
	}
	if a.Level == Critical || b.Level == Critical {
		return Signal{Key: key, Level: Critical, Verified: false, ObservedAt: at}
	}
	if a.Level == Warning || b.Level == Warning {
		return Signal{Key: key, Level: Warning, Verified: false, ObservedAt: at}
	}
	return Signal{Key: key, Level: Unknown, Verified: false, ObservedAt: at}
}

func evalDeviceHealth(key string, h device.DeviceHealth, prev *Baselines, at time.Time, opts DetectionOptions) (Signal, DeviceCounters, bool, bool) {
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
		return Signal{Key: key, Level: Unknown, Verified: false, ObservedAt: at}, DeviceCounters{}, false, false
	}

	if h.CollectionStatus == device.HealthCollected && !hasUsableSMARTEvidence(h) {
		level = Unknown
		verified = false
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
	clearBaseline := false
	if prev != nil {
		old := prev.Devices[key]
		deltas := []counterDelta{
			int64Delta(old.MediaErrors, cur.MediaErrors),
			int64Delta(old.DataIntegrityErrors, cur.DataIntegrityErrors),
			int64Delta(old.ErrorLogEntries, cur.ErrorLogEntries),
		}
		if memberCounterDeltasConflict(deltas) {
			level = Unknown
			verified = false
			clearBaseline = true
		} else {
			for _, d := range deltas {
				switch d {
				case counterIncreased:
					if level == Healthy {
						level = Warning
					}
				case counterDecreased, counterReset:
					level = Unknown
					verified = false
					clearBaseline = true
				case counterMissing:
					if level == Healthy {
						level = Unknown
						verified = false
					}
				}
			}
		}
	}

	update := h.CollectionStatus == device.HealthCollected && verified && hasUsableSMARTEvidence(h)
	return Signal{Key: key, Level: level, Verified: verified, ObservedAt: at}, cur, update, clearBaseline
}

func numericInt64(n *device.SmartNumeric) *int64 {
	if n == nil || n.Value == nil {
		return nil
	}
	return n.Value
}
