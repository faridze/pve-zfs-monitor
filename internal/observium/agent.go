package observium

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/faridze/pve-zfs-monitor/internal/monitor"
)

const maxAgentBytes = 8 << 10

var requiredSummaryKeys = []string{
	"schema_version", "collected_at", "evaluated_at", "host_overall", "host_fully_verified",
	"stale_observation", "collection_failures", "smart_completeness",
	"signals_total", "signals_healthy", "signals_warning", "signals_critical", "signals_unknown",
	"pools_total", "pools_online", "pools_degraded", "pools_faulted", "pools_unknown",
	"verified_critical_signals", "worst_level", "events_pending", "report_fully_healthy",
}

// RenderAppSection formats the Observium Unix Agent application block for AppName.
func RenderAppSection(sum Summary) (string, error) {
	if err := validateSummary(sum); err != nil {
		return "", err
	}
	lines := []string{
		fmt.Sprintf("schema_version=%d", sum.SchemaVersion),
		fmt.Sprintf("collected_at=%s", sum.CollectedAt.UTC().Format(time.RFC3339)),
		fmt.Sprintf("evaluated_at=%s", sum.EvaluatedAt.UTC().Format(time.RFC3339)),
		fmt.Sprintf("host_overall=%s", sum.HostOverall),
		fmt.Sprintf("host_fully_verified=%d", bool01(sum.HostFullyVerified)),
		fmt.Sprintf("stale_observation=%d", bool01(sum.StaleObservation)),
		fmt.Sprintf("collection_failures=%d", sum.CollectionFailures),
		fmt.Sprintf("smart_completeness=%s", sum.SmartCompleteness),
		fmt.Sprintf("signals_total=%d", sum.Counts.Total),
		fmt.Sprintf("signals_healthy=%d", sum.Counts.Healthy),
		fmt.Sprintf("signals_warning=%d", sum.Counts.Warning),
		fmt.Sprintf("signals_critical=%d", sum.Counts.Critical),
		fmt.Sprintf("signals_unknown=%d", sum.Counts.Unknown),
		fmt.Sprintf("pools_total=%d", sum.PoolCounts.Total),
		fmt.Sprintf("pools_online=%d", sum.PoolCounts.Online),
		fmt.Sprintf("pools_degraded=%d", sum.PoolCounts.Degraded),
		fmt.Sprintf("pools_faulted=%d", sum.PoolCounts.Faulted),
		fmt.Sprintf("pools_unknown=%d", sum.PoolCounts.Unknown),
		fmt.Sprintf("verified_critical_signals=%d", sum.VerifiedCriticalCount),
		fmt.Sprintf("worst_level=%s", sum.WorstLevel),
		fmt.Sprintf("events_pending=%d", sum.EventsPending),
		fmt.Sprintf("report_fully_healthy=%d", bool01(sum.ReportFullyHealthy)),
	}
	body := strings.Join(lines, "\n")
	section := fmt.Sprintf("<<<%s>>>\n%s\n", AppName, body)
	if len(section) > maxAgentBytes {
		return "", fmt.Errorf("agent section exceeds size limit")
	}
	return section, nil
}

// ParseAppSection parses and strictly validates agent output (fake Observium consumer).
func ParseAppSection(raw string) (Summary, error) {
	header := fmt.Sprintf("<<<%s>>>", AppName)
	raw = strings.TrimSpace(raw)
	if !strings.HasPrefix(raw, header) {
		return Summary{}, fmt.Errorf("missing application header")
	}
	payload := strings.TrimSpace(strings.TrimPrefix(raw, header))
	if payload == "" {
		return Summary{}, fmt.Errorf("empty payload")
	}
	seen := map[string]bool{}
	sum := Summary{}
	for _, line := range strings.Split(payload, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		key, val, ok := strings.Cut(line, "=")
		if !ok || key == "" {
			return Summary{}, fmt.Errorf("invalid line")
		}
		if seen[key] {
			return Summary{}, fmt.Errorf("duplicate field %q", key)
		}
		seen[key] = true
		if err := applySummaryField(&sum, key, val); err != nil {
			return Summary{}, err
		}
	}
	for _, req := range requiredSummaryKeys {
		if !seen[req] {
			return Summary{}, fmt.Errorf("missing field %q", req)
		}
	}
	if len(seen) != len(requiredSummaryKeys) {
		return Summary{}, fmt.Errorf("unknown fields present")
	}
	return sum, validateSummary(sum)
}

func applySummaryField(sum *Summary, key, val string) error {
	switch key {
	case "schema_version":
		v, err := parseNonNegInt(val, 16)
		if err != nil || v != SchemaVersion {
			return fmt.Errorf("schema_version")
		}
		sum.SchemaVersion = v
	case "collected_at":
		t, err := time.Parse(time.RFC3339, val)
		if err != nil {
			return fmt.Errorf("collected_at")
		}
		sum.CollectedAt = t.UTC()
	case "evaluated_at":
		t, err := time.Parse(time.RFC3339, val)
		if err != nil {
			return fmt.Errorf("evaluated_at")
		}
		sum.EvaluatedAt = t.UTC()
	case "host_overall":
		sum.HostOverall = monitor.Level(val)
	case "host_fully_verified", "stale_observation", "report_fully_healthy":
		b, err := parseBool01(val)
		if err != nil {
			return fmt.Errorf("%s", key)
		}
		switch key {
		case "host_fully_verified":
			sum.HostFullyVerified = b
		case "stale_observation":
			sum.StaleObservation = b
		case "report_fully_healthy":
			sum.ReportFullyHealthy = b
		}
	case "collection_failures":
		v, err := parseNonNegInt(val, 8)
		if err != nil {
			return fmt.Errorf("collection_failures")
		}
		sum.CollectionFailures = v
	case "smart_completeness":
		switch SmartCompleteness(val) {
		case SmartComplete, SmartIncomplete, SmartNotAttempted, SmartUnknownReport:
			sum.SmartCompleteness = SmartCompleteness(val)
		default:
			return fmt.Errorf("smart_completeness")
		}
	case "signals_total":
		v, err := parseNonNegInt(val, 512)
		if err != nil {
			return err
		}
		sum.Counts.Total = v
	case "signals_healthy":
		v, err := parseNonNegInt(val, 512)
		if err != nil {
			return err
		}
		sum.Counts.Healthy = v
	case "signals_warning":
		v, err := parseNonNegInt(val, 512)
		if err != nil {
			return err
		}
		sum.Counts.Warning = v
	case "signals_critical":
		v, err := parseNonNegInt(val, 512)
		if err != nil {
			return err
		}
		sum.Counts.Critical = v
	case "signals_unknown":
		v, err := parseNonNegInt(val, 512)
		if err != nil {
			return err
		}
		sum.Counts.Unknown = v
	case "pools_total":
		v, err := parseNonNegInt(val, 128)
		if err != nil {
			return err
		}
		sum.PoolCounts.Total = v
	case "pools_online":
		v, err := parseNonNegInt(val, 128)
		if err != nil {
			return err
		}
		sum.PoolCounts.Online = v
	case "pools_degraded":
		v, err := parseNonNegInt(val, 128)
		if err != nil {
			return err
		}
		sum.PoolCounts.Degraded = v
	case "pools_unknown":
		v, err := parseNonNegInt(val, 128)
		if err != nil {
			return err
		}
		sum.PoolCounts.Unknown = v
	case "pools_faulted":
		v, err := parseNonNegInt(val, 128)
		if err != nil {
			return err
		}
		sum.PoolCounts.Faulted = v
	case "verified_critical_signals":
		v, err := parseNonNegInt(val, 512)
		if err != nil {
			return err
		}
		sum.VerifiedCriticalCount = v
	case "worst_level":
		sum.WorstLevel = monitor.Level(val)
	case "events_pending":
		v, err := parseNonNegInt(val, maxEntriesExport)
		if err != nil {
			return err
		}
		sum.EventsPending = v
	default:
		return fmt.Errorf("unknown field %q", key)
	}
	return nil
}

func validateSummary(sum Summary) error {
	if sum.SchemaVersion != SchemaVersion || sum.CollectedAt.IsZero() || sum.EvaluatedAt.IsZero() {
		return fmt.Errorf("incomplete summary")
	}
	if !validExportLevel(sum.HostOverall) || !validExportLevel(sum.WorstLevel) {
		return fmt.Errorf("invalid level")
	}
	if sum.Counts.Total != sum.Counts.Healthy+sum.Counts.Warning+sum.Counts.Critical+sum.Counts.Unknown {
		return fmt.Errorf("signal count mismatch")
	}
	if sum.PoolCounts.Total != sum.PoolCounts.Online+sum.PoolCounts.Degraded+sum.PoolCounts.Faulted+sum.PoolCounts.Unknown {
		return fmt.Errorf("pool count mismatch")
	}
	if sum.ReportFullyHealthy && (sum.WorstLevel != monitor.Healthy || sum.VerifiedCriticalCount > 0 || sum.StaleObservation) {
		return fmt.Errorf("report_fully_healthy inconsistent")
	}
	if levelRank(sum.WorstLevel) < levelRankFromCounts(sum.Counts) {
		return fmt.Errorf("worst_level inconsistent")
	}
	if sum.VerifiedCriticalCount > 0 && levelRank(sum.WorstLevel) < levelRank(monitor.Critical) {
		return fmt.Errorf("verified critical inconsistent with worst_level")
	}
	return nil
}

func levelRankFromCounts(c LevelCounts) int {
	w := monitor.Healthy
	if c.Critical > 0 {
		w = monitor.Critical
	} else if c.Warning > 0 {
		w = monitor.Warning
	} else if c.Unknown > 0 {
		w = monitor.Unknown
	}
	return levelRank(w)
}

func parseNonNegInt(s string, max int) (int, error) {
	if s == "" || strings.HasPrefix(s, "-") || strings.HasPrefix(s, "+") {
		return 0, fmt.Errorf("invalid int")
	}
	v, err := strconv.ParseInt(s, 10, 64)
	if err != nil || v < 0 || v > int64(max) {
		return 0, fmt.Errorf("invalid int")
	}
	return int(v), nil
}

func parseBool01(s string) (bool, error) {
	switch s {
	case "0":
		return false, nil
	case "1":
		return true, nil
	default:
		return false, fmt.Errorf("invalid bool")
	}
}

func bool01(b bool) int {
	if b {
		return 1
	}
	return 0
}
