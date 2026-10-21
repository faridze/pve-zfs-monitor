package observium

import (
	"strings"
	"testing"
	"time"

	"github.com/faridze/pve-zfs-monitor/internal/monitor"
)

func TestRenderParseRoundTripSchema2(t *testing.T) {
	at := time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)
	kHost, _ := monitor.Key("host", "heartbeat")
	kPool, _ := monitor.Key("pool", "p1")
	kDev, _ := monitor.Key("device", "d1")
	sum, err := BuildSummary(ExportInput{
		Signals: []monitor.Signal{
			{Key: kHost, Level: monitor.Healthy, Verified: true, ObservedAt: at},
			{Key: kPool, Level: monitor.Critical, Verified: true, ObservedAt: at},
			{Key: kDev, Level: monitor.Unknown, Verified: false, ObservedAt: at},
		},
		CollectedAt:       at,
		EvaluatedAt:       at,
		MaxSignalAge:      time.Hour,
		SmartCompleteness: SmartComplete,
		EventsPending:     1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if sum.ReportFullyHealthy {
		t.Fatal("must not be fully healthy")
	}
	raw, err := RenderAppSection(sum)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(raw, "rpool") {
		t.Fatalf("leak: %s", raw)
	}
	got, err := ParseAppSection(raw)
	if err != nil {
		t.Fatal(err)
	}
	if got.Counts != sum.Counts || got.WorstLevel != monitor.Critical {
		t.Fatalf("mismatch %+v", got)
	}
}

func TestParserRejectsDuplicateAndUnknownFields(t *testing.T) {
	at := time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)
	kHost, _ := monitor.Key("host", "heartbeat")
	sum, _ := BuildSummary(ExportInput{
		Signals:           []monitor.Signal{{Key: kHost, Level: monitor.Unknown, Verified: false, ObservedAt: at}},
		CollectedAt:       at,
		EvaluatedAt:       at,
		SmartCompleteness: SmartIncomplete,
	})
	raw, _ := RenderAppSection(sum)
	raw = strings.Replace(raw, "signals_total=", "signals_total=", 1)
	raw += "extra_field=1\n"
	if _, err := ParseAppSection(raw); err == nil {
		t.Fatal("expected unknown field error")
	}
}

func TestEmptySnapshotNotFullyHealthy(t *testing.T) {
	at := time.Now().UTC()
	sum, err := BuildSummary(ExportInput{
		CollectedAt: at, EvaluatedAt: at, SmartCompleteness: SmartUnknownReport,
	})
	if err != nil {
		t.Fatal(err)
	}
	if sum.ReportFullyHealthy || sum.HostOverall != monitor.Unknown {
		t.Fatalf("empty snapshot: %+v", sum)
	}
}

func TestParserRejectsMalformedFramingAndInts(t *testing.T) {
	if _, err := ParseAppSection("<<<wrong_app>>>\nschema_version=2\n"); err == nil {
		t.Fatal("wrong header")
	}
	at := time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)
	kHost, _ := monitor.Key("host", "heartbeat")
	sum, _ := BuildSummary(ExportInput{
		Signals:           []monitor.Signal{{Key: kHost, Level: monitor.Healthy, Verified: true, ObservedAt: at}},
		CollectedAt:       at,
		EvaluatedAt:       at,
		SmartCompleteness: SmartComplete,
	})
	raw, _ := RenderAppSection(sum)
	bad := strings.Replace(raw, "signals_healthy=1", "signals_healthy=-1", 1)
	if _, err := ParseAppSection(bad); err == nil {
		t.Fatal("negative int")
	}
	bad = strings.Replace(raw, "signals_total=1", "signals_total=99999", 1)
	if _, err := ParseAppSection(bad); err == nil {
		t.Fatal("overflow / mismatch")
	}
}

func TestPartiallyVerifiedPreservesCriticalExport(t *testing.T) {
	at := time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)
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
	if sum.VerifiedCriticalCount != 1 || sum.ReportFullyHealthy || sum.WorstLevel != monitor.Critical {
		t.Fatalf("partial verify: %+v", sum)
	}
}
