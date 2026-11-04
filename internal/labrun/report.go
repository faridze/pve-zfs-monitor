package labrun

import (
	"github.com/faridze/pve-zfs-monitor/internal/monitor"
)

// SafeEvent is a privacy-safe notification intent for JSON export.
type SafeEvent struct {
	Key         string        `json:"key"`
	Kind        monitor.Kind  `json:"kind"`
	Level       monitor.Level `json:"level"`
	EmittedUnix int64         `json:"emitted_unix"`
}

// RunOnceReport is bounded JSON output for lab orchestration.
type RunOnceReport struct {
	Lab                   bool          `json:"lab"`
	DeliveryMode          string        `json:"delivery_mode"`
	Overall               monitor.Level `json:"overall"`
	VerifiedCritical      int           `json:"verified_critical"`
	VerifiedWarning       int           `json:"verified_warning"`
	UnknownSignals        int           `json:"unknown_signals"`
	HealthySignals        int           `json:"healthy_signals"`
	EventsEmitted         []SafeEvent   `json:"events_emitted"`
	EventsEmittedCount    int           `json:"events_emitted_count"`
	PendingNotifications  int           `json:"pending_notifications"`
	SimulatedDeliveries   int           `json:"simulated_deliveries"`
	SimulatedAck          bool          `json:"simulated_acknowledgment,omitempty"`
	CollectionFailures    []string      `json:"collection_failures,omitempty"`
	StatePersisted        bool          `json:"state_persisted"`
	AtLeastOnceLimitation string        `json:"at_least_once_limitation,omitempty"`
}

func safeEvents(events []monitor.Event) []SafeEvent {
	out := make([]SafeEvent, 0, len(events))
	for _, e := range events {
		out = append(out, SafeEvent{
			Key: e.Key, Kind: e.Kind, Level: e.Level, EmittedUnix: e.EmittedUnix,
		})
	}
	return out
}

func summarizeSignals(signals []monitor.Signal) (overall monitor.Level, crit, warn, unk, healthy int) {
	overall = monitor.Healthy
	for _, s := range signals {
		if s.Verified {
			switch s.Level {
			case monitor.Critical:
				crit++
			case monitor.Warning:
				warn++
			case monitor.Healthy:
				healthy++
			}
		} else if s.Level != monitor.Healthy {
			unk++
		}
		effective := s.Level
		if !s.Verified && s.Level != monitor.Healthy {
			effective = monitor.Unknown
		}
		if levelRank(effective) > levelRank(overall) {
			overall = effective
		}
	}
	return overall, crit, warn, unk, healthy
}

// applyCollectionFailuresToOverall prevents a false Healthy summary when collection failed.
func applyCollectionFailuresToOverall(overall monitor.Level, failures []string) monitor.Level {
	if len(failures) == 0 {
		return overall
	}
	if overall == monitor.Healthy {
		return monitor.Unknown
	}
	return overall
}

func levelRank(l monitor.Level) int {
	switch l {
	case monitor.Critical:
		return 4
	case monitor.Warning:
		return 3
	case monitor.Unknown:
		return 2
	case monitor.Healthy:
		return 1
	default:
		return 0
	}
}
