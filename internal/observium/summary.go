package observium

import "github.com/faridze/pve-zfs-monitor/internal/monitor"

const AppName = "pve_zfs_monitor"

// LevelCounts tallies signal levels in one observation snapshot.
type LevelCounts struct {
	Total    int
	Healthy  int
	Warning  int
	Critical int
	Unknown  int
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
