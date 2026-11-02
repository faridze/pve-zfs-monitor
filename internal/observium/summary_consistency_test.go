package observium

import (
	"strings"
	"testing"
	"time"

	"github.com/faridze/pve-zfs-monitor/internal/monitor"
)

func TestSummaryConsistencyTable(t *testing.T) {
	at := time.Date(2026, 11, 2, 12, 0, 0, 0, time.UTC)
	kHost, _ := monitor.Key("host", "heartbeat")
	kPool, _ := monitor.Key("pool", "p1")
	kDev, _ := monitor.Key("device", "d1")

	tests := []struct {
		name      string
		in        ExportInput
		wantOK    bool
		wantFull  bool
		wantHost  monitor.Level
		wantWorst monitor.Level
	}{
		{
			name: "positive verified host healthy",
			in: ExportInput{
				Signals: []monitor.Signal{
					{Key: kHost, Level: monitor.Healthy, Verified: true, ObservedAt: at},
				},
				CollectedAt: at, EvaluatedAt: at, MaxSignalAge: time.Hour,
				SmartCompleteness: SmartComplete, SMARTEvidenceVerified: true,
			},
			wantOK: true, wantFull: true, wantHost: monitor.Healthy, wantWorst: monitor.Healthy,
		},
		{
			name: "no host signals cannot be fully healthy",
			in: ExportInput{
				Signals: []monitor.Signal{
					{Key: kPool, Level: monitor.Healthy, Verified: true, ObservedAt: at},
				},
				CollectedAt: at, EvaluatedAt: at, MaxSignalAge: time.Hour,
				SmartCompleteness: SmartComplete, SMARTEvidenceVerified: true,
			},
			wantOK: true, wantFull: false, wantHost: monitor.Unknown,
		},
		{
			name: "stale host cannot be fully healthy",
			in: ExportInput{
				Signals: []monitor.Signal{
					{Key: kHost, Level: monitor.Healthy, Verified: true, ObservedAt: at.Add(-2 * time.Hour)},
				},
				CollectedAt: at, EvaluatedAt: at, MaxSignalAge: time.Hour,
				SmartCompleteness: SmartComplete, SMARTEvidenceVerified: true,
			},
			wantOK: true, wantFull: false,
		},
		{
			name: "verified critical preserved with unknown device",
			in: ExportInput{
				Signals: []monitor.Signal{
					{Key: kPool, Level: monitor.Critical, Verified: true, ObservedAt: at},
					{Key: kDev, Level: monitor.Unknown, Verified: false, ObservedAt: at},
				},
				CollectedAt: at, EvaluatedAt: at, MaxSignalAge: time.Hour,
				SmartCompleteness: SmartIncomplete,
			},
			wantOK: true, wantFull: false, wantWorst: monitor.Critical,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			sum, err := BuildSummary(tc.in)
			if tc.wantOK && err != nil {
				t.Fatal(err)
			}
			if sum.ReportFullyHealthy != tc.wantFull {
				t.Fatalf("ReportFullyHealthy=%v want %v: %+v", sum.ReportFullyHealthy, tc.wantFull, sum)
			}
			if tc.wantHost != "" && sum.HostOverall != tc.wantHost {
				t.Fatalf("HostOverall=%v want %v", sum.HostOverall, tc.wantHost)
			}
			if tc.wantWorst != "" && sum.WorstLevel != tc.wantWorst {
				t.Fatalf("WorstLevel=%v want %v", sum.WorstLevel, tc.wantWorst)
			}
			if sum.HostOverall == monitor.Unknown && sum.ReportFullyHealthy {
				t.Fatal("HostOverall Unknown with ReportFullyHealthy")
			}
			if err := validateSummary(sum); err != nil && sum.ReportFullyHealthy {
				t.Fatalf("validateSummary: %v", err)
			}
		})
	}
}

func TestParserRejectsForgedHostUnknownFullyHealthy(t *testing.T) {
	at := time.Date(2026, 11, 2, 12, 0, 0, 0, time.UTC)
	forged := strings.Join([]string{
		"<<<pve_zfs_monitor>>>",
		"schema_version=2",
		"collected_at=" + at.Format(time.RFC3339),
		"evaluated_at=" + at.Format(time.RFC3339),
		"host_overall=unknown",
		"host_fully_verified=1",
		"stale_observation=0",
		"collection_failures=0",
		"smart_completeness=complete",
		"signals_total=1",
		"signals_healthy=1",
		"signals_warning=0",
		"signals_critical=0",
		"signals_unknown=0",
		"pools_total=0",
		"pools_online=0",
		"pools_degraded=0",
		"pools_faulted=0",
		"pools_unknown=0",
		"verified_critical_signals=0",
		"worst_level=healthy",
		"events_pending=0",
		"report_fully_healthy=1",
	}, "\n") + "\n"
	if _, err := ParseAppSection(forged); err == nil {
		t.Fatal("parser must reject unknown host with report_fully_healthy")
	}
}
