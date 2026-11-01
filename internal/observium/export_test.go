package observium

import (
	"testing"
	"time"

	"github.com/faridze/pve-zfs-monitor/internal/device"
	"github.com/faridze/pve-zfs-monitor/internal/monitor"
)

func TestBuildSummarySmartCompleteWithoutEvidenceNotHealthy(t *testing.T) {
	at := time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)
	kHost, _ := monitor.Key("host", "heartbeat")
	sum, err := BuildSummary(ExportInput{
		Signals:           []monitor.Signal{{Key: kHost, Level: monitor.Healthy, Verified: true, ObservedAt: at}},
		CollectedAt:       at,
		EvaluatedAt:       at,
		SmartCompleteness: SmartComplete,
	})
	if err != nil {
		t.Fatal(err)
	}
	if sum.ReportFullyHealthy {
		t.Fatalf("SmartComplete flag without export proof must not be fully healthy: %+v", sum)
	}
}

func TestSmartCompletenessFromDeviceEmptyInventoryFailClosed(t *testing.T) {
	report := &device.DeviceReport{
		InventoryCompleteness: device.ReportInventoryComplete,
		MappingCompleteness:   device.ReportMappingComplete,
		SmartCompleteness:     device.ReportSmartComplete,
	}
	if SmartCompletenessFromDevice(report, true) != SmartIncomplete {
		t.Fatalf("expected incomplete, got %s", SmartCompletenessFromDevice(report, true))
	}
}

func TestBuildSummaryEmptySignalsWithSmartComplete(t *testing.T) {
	at := time.Now().UTC()
	sum, err := BuildSummary(ExportInput{
		CollectedAt: at, EvaluatedAt: at, SmartCompleteness: SmartComplete,
	})
	if err != nil {
		t.Fatal(err)
	}
	if sum.ReportFullyHealthy || sum.HostOverall == monitor.Healthy {
		t.Fatalf("empty signals: %+v", sum)
	}
}

func TestBuildSummaryStaleWhenMaxAgeOmitted(t *testing.T) {
	at := time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)
	kHost, _ := monitor.Key("host", "heartbeat")
	sum, err := BuildSummary(ExportInput{
		Signals:           []monitor.Signal{{Key: kHost, Level: monitor.Healthy, Verified: true, ObservedAt: at.Add(-time.Hour)}},
		CollectedAt:       at,
		EvaluatedAt:       at,
		MaxSignalAge:      0,
		SmartCompleteness: SmartComplete,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !sum.StaleObservation || sum.ReportFullyHealthy {
		t.Fatalf("default freshness must mark stale: %+v", sum)
	}
}

func TestBuildSummaryVerifiedCriticalWithUnknown(t *testing.T) {
	at := time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)
	kPool, _ := monitor.Key("pool", "p1")
	kDev, _ := monitor.Key("device", "d1")
	sum, err := BuildSummary(ExportInput{
		Signals: []monitor.Signal{
			{Key: kPool, Level: monitor.Critical, Verified: true, ObservedAt: at},
			{Key: kDev, Level: monitor.Unknown, Verified: false, ObservedAt: at},
		},
		CollectedAt:       at,
		EvaluatedAt:       at,
		SmartCompleteness: SmartIncomplete,
	})
	if err != nil {
		t.Fatal(err)
	}
	if sum.WorstLevel != monitor.Critical || sum.VerifiedCriticalCount != 1 || sum.ReportFullyHealthy {
		t.Fatalf("partial: %+v", sum)
	}
}

func TestParserRejectsForgedFullyHealthy(t *testing.T) {
	at := time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)
	forged := "<<<pve_zfs_monitor>>>\n" +
		"schema_version=2\n" +
		"collected_at=" + at.Format(time.RFC3339) + "\n" +
		"evaluated_at=" + at.Format(time.RFC3339) + "\n" +
		"host_overall=healthy\nhost_fully_verified=1\nstale_observation=0\ncollection_failures=0\n" +
		"smart_completeness=incomplete\n" +
		"signals_total=0\nsignals_healthy=0\nsignals_warning=0\nsignals_critical=0\nsignals_unknown=0\n" +
		"pools_total=0\npools_online=0\npools_degraded=0\npools_faulted=0\npools_unknown=0\n" +
		"verified_critical_signals=0\nworst_level=unknown\nevents_pending=0\nreport_fully_healthy=1\n"
	if _, err := ParseAppSection(forged); err == nil {
		t.Fatal("parser must reject forged report_fully_healthy with zero signals")
	}
}
