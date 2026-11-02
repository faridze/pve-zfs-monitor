package outbox

import (
	"strings"
	"testing"

	"github.com/faridze/pve-zfs-monitor/internal/monitor"
)

func TestFormatMessageAllKinds(t *testing.T) {
	key := "pool:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	kinds := []struct {
		kind  monitor.Kind
		level monitor.Level
	}{
		{monitor.Alert, monitor.Critical},
		{monitor.Alert, monitor.Warning},
		{monitor.Alert, monitor.Unknown},
		{monitor.Reminder, monitor.Critical},
		{monitor.Recovery, monitor.Healthy},
	}
	for _, tc := range kinds {
		msg, err := FormatMessage("lab-node", monitor.Event{Kind: tc.kind, Level: tc.level, Key: key})
		if err != nil {
			t.Fatalf("%+v: %v", tc, err)
		}
		if strings.Contains(msg.Body, "/dev/") || strings.Contains(msg.Subject, "@") {
			t.Fatalf("leak in %s", msg.Body)
		}
		if !strings.Contains(msg.Body, key) {
			t.Fatalf("expected hashed key in body")
		}
	}
}
