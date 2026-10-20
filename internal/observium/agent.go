package observium

import (
	"fmt"
	"strings"
	"time"

	"github.com/faridze/pve-zfs-monitor/internal/monitor"
)

const maxAgentBytes = 8 << 10

// RenderAppSection formats the Observium Unix Agent application block for AppName.
func RenderAppSection(sum Summary) (string, error) {
	if sum.SchemaVersion != 1 {
		return "", fmt.Errorf("unsupported schema version")
	}
	if sum.CollectedAt.IsZero() {
		return "", fmt.Errorf("missing collected_at")
	}
	lines := []string{
		fmt.Sprintf("schema_version=%d", sum.SchemaVersion),
		fmt.Sprintf("collected_at=%s", sum.CollectedAt.UTC().Format(time.RFC3339)),
		fmt.Sprintf("signals_total=%d", sum.Counts.Total),
		fmt.Sprintf("signals_healthy=%d", sum.Counts.Healthy),
		fmt.Sprintf("signals_warning=%d", sum.Counts.Warning),
		fmt.Sprintf("signals_critical=%d", sum.Counts.Critical),
		fmt.Sprintf("signals_unknown=%d", sum.Counts.Unknown),
		fmt.Sprintf("worst_level=%s", sum.WorstLevel),
		fmt.Sprintf("events_pending=%d", sum.EventsPending),
	}
	body := strings.Join(lines, "\n")
	section := fmt.Sprintf("<<<%s>>>\n%s\n", AppName, body)
	if len(section) > maxAgentBytes {
		return "", fmt.Errorf("agent section exceeds size limit")
	}
	return section, nil
}

// ParseAppSection parses agent output for AppName (fake poller / tests).
func ParseAppSection(raw string) (Summary, error) {
	header := fmt.Sprintf("<<<%s>>>", AppName)
	raw = strings.TrimSpace(raw)
	if !strings.HasPrefix(raw, header) {
		return Summary{}, fmt.Errorf("missing application header")
	}
	payload := strings.TrimPrefix(raw, header)
	payload = strings.TrimSpace(payload)
	sum := Summary{SchemaVersion: 0}
	for _, line := range strings.Split(payload, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		key, val, ok := strings.Cut(line, "=")
		if !ok {
			return Summary{}, fmt.Errorf("invalid line")
		}
		switch key {
		case "schema_version":
			var v int
			if _, err := fmt.Sscanf(val, "%d", &v); err != nil {
				return Summary{}, fmt.Errorf("schema_version")
			}
			sum.SchemaVersion = v
		case "collected_at":
			t, err := time.Parse(time.RFC3339, val)
			if err != nil {
				return Summary{}, fmt.Errorf("collected_at")
			}
			sum.CollectedAt = t.UTC()
		case "signals_total":
			fmt.Sscanf(val, "%d", &sum.Counts.Total)
		case "signals_healthy":
			fmt.Sscanf(val, "%d", &sum.Counts.Healthy)
		case "signals_warning":
			fmt.Sscanf(val, "%d", &sum.Counts.Warning)
		case "signals_critical":
			fmt.Sscanf(val, "%d", &sum.Counts.Critical)
		case "signals_unknown":
			fmt.Sscanf(val, "%d", &sum.Counts.Unknown)
		case "worst_level":
			sum.WorstLevel = monitor.Level(val)
		case "events_pending":
			fmt.Sscanf(val, "%d", &sum.EventsPending)
		default:
			return Summary{}, fmt.Errorf("unknown field %q", key)
		}
	}
	if sum.SchemaVersion != 1 || sum.CollectedAt.IsZero() {
		return Summary{}, fmt.Errorf("incomplete summary")
	}
	if !validExportLevel(sum.WorstLevel) {
		return Summary{}, fmt.Errorf("invalid worst_level")
	}
	return sum, nil
}
