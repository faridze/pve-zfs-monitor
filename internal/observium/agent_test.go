package observium

import (
	"strings"
	"testing"
	"time"

	"github.com/faridze/pve-zfs-monitor/internal/monitor"
)

func TestRenderAndParseAppSectionRoundTrip(t *testing.T) {
	at := time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)
	signals := []monitor.Signal{
		{Key: "host:abc", Level: monitor.Healthy, Verified: true, ObservedAt: at},
		{Key: "pool:def", Level: monitor.Critical, Verified: true, ObservedAt: at},
		{Key: "device:ghi", Level: monitor.Unknown, Verified: false, ObservedAt: at},
	}
	sum, err := SummarizeSignals(signals, at, 2)
	if err != nil {
		t.Fatal(err)
	}
	if sum.WorstLevel != monitor.Critical {
		t.Fatalf("worst=%v", sum.WorstLevel)
	}
	raw, err := RenderAppSection(sum)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(raw, "/dev/") || strings.Contains(raw, "rpool") {
		t.Fatalf("leaked identifiers: %s", raw)
	}
	got, err := ParseAppSection(raw)
	if err != nil {
		t.Fatal(err)
	}
	if got.Counts != sum.Counts || got.WorstLevel != sum.WorstLevel || got.EventsPending != 2 {
		t.Fatalf("roundtrip mismatch: %+v vs %+v", got, sum)
	}
}

func TestFakePollerRejectsMissingHeader(t *testing.T) {
	_, err := ParseAppSection("<<<other>>>\nschema_version=1\n")
	if err == nil {
		t.Fatal("expected error")
	}
}
