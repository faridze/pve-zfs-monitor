package observium

import (
	"fmt"
	"strings"
	"time"

	"github.com/faridze/pve-zfs-monitor/internal/device"
	"github.com/faridze/pve-zfs-monitor/internal/monitor"
)

const SchemaVersion = 2

// SmartCompleteness mirrors device report completeness for export (no raw paths).
type SmartCompleteness string

const (
	SmartComplete      SmartCompleteness = "complete"
	SmartIncomplete    SmartCompleteness = "incomplete"
	SmartNotAttempted  SmartCompleteness = "not_attempted"
	SmartUnknownReport SmartCompleteness = "unknown"
)

// ExportInput is privacy-safe metadata for one observation cycle.
type ExportInput struct {
	Signals           []monitor.Signal
	CollectedAt       time.Time
	EvaluatedAt       time.Time
	MaxSignalAge      time.Duration
	SmartCompleteness SmartCompleteness
	CollectionFailed  bool
	EventsPending     int
}

// Summary is the versioned Observium application payload.
type Summary struct {
	SchemaVersion         int
	CollectedAt           time.Time
	EvaluatedAt           time.Time
	HostOverall           monitor.Level
	HostFullyVerified     bool
	StaleObservation      bool
	CollectionFailures    int
	SmartCompleteness     SmartCompleteness
	Counts                LevelCounts
	PoolCounts            PoolHealthCounts
	VerifiedCriticalCount int
	WorstLevel            monitor.Level
	EventsPending         int
	ReportFullyHealthy    bool
}

// PoolHealthCounts aggregate pool-scoped signal levels.
type PoolHealthCounts struct {
	Total    int
	Online   int
	Degraded int
	Faulted  int
	Unknown  int
}

// BuildSummary constructs a fail-closed export summary.
func BuildSummary(in ExportInput) (Summary, error) {
	if in.CollectedAt.IsZero() || in.EvaluatedAt.IsZero() {
		return Summary{}, fmt.Errorf("missing timestamps")
	}
	if in.EventsPending < 0 || in.EventsPending > maxEntriesExport {
		return Summary{}, fmt.Errorf("invalid pending count")
	}
	if len(in.Signals) > 512 {
		return Summary{}, fmt.Errorf("too many signals")
	}
	stale := false
	if in.MaxSignalAge > 0 {
		for _, s := range in.Signals {
			if s.ObservedAt.IsZero() || in.EvaluatedAt.Sub(s.ObservedAt) > in.MaxSignalAge || s.ObservedAt.After(in.EvaluatedAt) {
				stale = true
				break
			}
		}
	}
	var counts LevelCounts
	var pools PoolHealthCounts
	worst := monitor.Healthy
	verifiedCritical := 0
	hostSignals := 0
	hostWorst := monitor.Healthy
	allVerified := true
	empty := len(in.Signals) == 0

	for _, s := range in.Signals {
		if !validExportLevel(s.Level) {
			return Summary{}, fmt.Errorf("invalid level")
		}
		if !s.Verified {
			allVerified = false
		}
		if s.Level == monitor.Critical && s.Verified {
			verifiedCritical++
		}
		counts.Total++
		switch s.Level {
		case monitor.Healthy:
			counts.Healthy++
		case monitor.Warning:
			counts.Warning++
		case monitor.Critical:
			counts.Critical++
		case monitor.Unknown:
			counts.Unknown++
		}
		if levelRank(s.Level) > levelRank(worst) {
			worst = s.Level
		}
		scope, _, ok := strings.Cut(s.Key, ":")
		if !ok {
			return Summary{}, fmt.Errorf("invalid signal key")
		}
		switch scope {
		case "pool":
			pools.Total++
			switch s.Level {
			case monitor.Healthy:
				pools.Online++
			case monitor.Warning:
				pools.Degraded++
			case monitor.Critical:
				pools.Faulted++
			default:
				pools.Unknown++
			}
		case "host":
			hostSignals++
			if levelRank(s.Level) > levelRank(hostWorst) {
				hostWorst = s.Level
			}
		}
	}
	if empty {
		allVerified = false
		hostWorst = monitor.Unknown
		worst = monitor.Unknown
	}
	if stale || in.CollectionFailed || in.SmartCompleteness != SmartComplete {
		allVerified = false
	}
	hostOverall := hostWorst
	if hostSignals == 0 {
		hostOverall = monitor.Unknown
	}
	if verifiedCritical > 0 {
		if levelRank(monitor.Critical) >= levelRank(worst) {
			worst = monitor.Critical
		}
	}
	reportHealthy := !empty && !stale && !in.CollectionFailed &&
		in.SmartCompleteness == SmartComplete && allVerified &&
		worst == monitor.Healthy && verifiedCritical == 0

	cf := 0
	if in.CollectionFailed {
		cf = 1
	}
	return Summary{
		SchemaVersion:         SchemaVersion,
		CollectedAt:           in.CollectedAt.UTC(),
		EvaluatedAt:           in.EvaluatedAt.UTC(),
		HostOverall:           hostOverall,
		HostFullyVerified:     allVerified && !empty,
		StaleObservation:      stale,
		CollectionFailures:    cf,
		SmartCompleteness:     in.SmartCompleteness,
		Counts:                counts,
		PoolCounts:            pools,
		VerifiedCriticalCount: verifiedCritical,
		WorstLevel:            worst,
		EventsPending:         in.EventsPending,
		ReportFullyHealthy:    reportHealthy,
	}, nil
}

// SmartCompletenessFromDevice maps device report fields to export enum.
func SmartCompletenessFromDevice(report *device.DeviceReport, devicesOK bool) SmartCompleteness {
	if !devicesOK || report == nil {
		return SmartUnknownReport
	}
	switch report.SmartCompleteness {
	case device.ReportSmartComplete:
		return SmartComplete
	case device.ReportSmartIncomplete:
		return SmartIncomplete
	case device.ReportSmartNotAttempted:
		return SmartNotAttempted
	default:
		return SmartUnknownReport
	}
}

const maxEntriesExport = 256
