package monitor

import (
	"fmt"
	"strings"
	"time"

	"github.com/faridze/pve-zfs-monitor/internal/zfs"
)

// PoolSignals normalizes the read-only zpool list result. It is not an
// automatic collector: callers must pass the collection result explicitly.
// Member, SMART and scrub findings require separate future detectors.
func PoolSignals(pools []zfs.Pool, observedAt time.Time, collectionOK bool) ([]Signal, error) {
	if !collectionOK || len(pools) == 0 {
		key, _ := Key("host", "zpool-list")
		return []Signal{{Key: key, Level: Unknown, Verified: false, ObservedAt: observedAt}}, nil
	}
	// Include an explicit collector heartbeat in every valid snapshot, so
	// an earlier collector outage can recover after verified good polls.
	hostKey, _ := Key("host", "zpool-list")
	signals := []Signal{{Key: hostKey, Level: Healthy, Verified: true, ObservedAt: observedAt}}
	seen := map[string]bool{}
	for _, p := range pools {
		name := strings.TrimSpace(p.Name)
		if name == "" || seen[name] {
			return nil, fmt.Errorf("invalid or duplicate ZFS pool")
		}
		seen[name] = true
		key, _ := Key("pool", name)
		level := Unknown
		verified := true
		switch p.Health {
		case "ONLINE":
			level = Healthy
		case "DEGRADED":
			level = Warning
		case "FAULTED", "OFFLINE", "UNAVAIL", "REMOVED", "SUSPENDED":
			level = Critical
		default:
			verified = false
		}
		signals = append(signals, Signal{
			Key: key, Level: level, Verified: verified, ObservedAt: observedAt,
		})
	}
	return signals, nil
}
