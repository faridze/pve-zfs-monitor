package outbox

import (
	"fmt"
	"strings"

	"github.com/faridze/pve-zfs-monitor/internal/monitor"
)

// Message is a privacy-safe mail payload (no raw pool or device names).
type Message struct {
	Subject string
	Body    string
}

// FormatProxmoxStyle builds a plain-text notification compatible with typical
// Proxmox admin mail expectations (subject prefix + short body).
func FormatProxmoxStyle(hostLabel string, e monitor.Event) (Message, error) {
	hostLabel = strings.TrimSpace(hostLabel)
	if hostLabel == "" {
		hostLabel = "pve-host"
	}
	if len(hostLabel) > 64 {
		return Message{}, fmt.Errorf("host label too long")
	}
	subject := fmt.Sprintf("[pve-zfs-monitor] %s %s (%s)", strings.ToUpper(string(e.Kind)), e.Level, hostLabel)
	body := fmt.Sprintf("Event: %s\nSeverity: %s\nSignal: %s\n\nThis message contains hashed signal identifiers only. Inspect local diagnostics for details.\n",
		e.Kind, e.Level, e.Key)
	if strings.Contains(body, "\r") {
		return Message{}, fmt.Errorf("invalid body")
	}
	return Message{Subject: subject, Body: body}, nil
}
