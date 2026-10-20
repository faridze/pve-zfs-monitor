package observium

import (
	"fmt"
	"time"

	"github.com/faridze/pve-zfs-monitor/internal/monitor"
)

const AppName = "pve_zfs_monitor"

// Summary is a privacy-safe aggregate of monitor signals for Observium export.
type Summary struct {
	SchemaVersion int
	CollectedAt   time.Time
	Counts        LevelCounts
	WorstLevel    monitor.Level
	EventsPending int
}

// LevelCounts tallies signal levels in one observation snapshot.
type LevelCounts struct {
	Total    int
	Healthy  int
	Warning  int
	Critical int
	Unknown  int
}

// SummarizeSignals builds an export summary from normalized monitor signals.
func SummarizeSignals(signals []monitor.Signal, collectedAt time.Time, pendingEvents int) (Summary, error) {
	if collectedAt.IsZero() {
		return Summary{}, fmt.Errorf("missing collected_at")
	}
	if pendingEvents < 0 {
		return Summary{}, fmt.Errorf("invalid pending events")
	}
	if len(signals) > 512 {
		return Summary{}, fmt.Errorf("too many signals")
	}
	var counts LevelCounts
	worst := monitor.Healthy
	for _, s := range signals {
		if !validExportLevel(s.Level) {
			return Summary{}, fmt.Errorf("invalid signal level")
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
	}
	return Summary{
		SchemaVersion: 1,
		CollectedAt:   collectedAt.UTC(),
		Counts:        counts,
		WorstLevel:    worst,
		EventsPending: pendingEvents,
	}, nil
}

func levelRank(l monitor.Level) int {
	switch l {
	case monitor.Healthy:
		return 1
	case monitor.Unknown:
		return 2
	case monitor.Warning:
		return 3
	case monitor.Critical:
		return 4
	default:
		return -1
	}
}

func validExportLevel(l monitor.Level) bool {
	return l == monitor.Healthy || l == monitor.Warning || l == monitor.Critical || l == monitor.Unknown
}
