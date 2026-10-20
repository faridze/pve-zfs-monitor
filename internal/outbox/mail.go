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

// FormatMessage builds a plain-text notification for all event kinds.
func FormatMessage(hostLabel string, e monitor.Event) (Message, error) {
	hostLabel = sanitizeHostLabel(hostLabel)
	subject := fmt.Sprintf("[pve-zfs-monitor] %s %s (%s)",
		strings.ToUpper(string(e.Kind)), strings.ToUpper(string(e.Level)), hostLabel)
	body := fmt.Sprintf(
		"Kind: %s\nSeverity: %s\nSignal: %s\n\nHashed signal identifier only — inspect local diagnostics for details.\n",
		e.Kind, e.Level, e.Key,
	)
	if strings.Contains(subject, "\r") || strings.Contains(body, "\r") {
		return Message{}, fmt.Errorf("invalid message")
	}
	if len(subject) > 256 || len(body) > 4096 {
		return Message{}, fmt.Errorf("message too large")
	}
	return Message{Subject: subject, Body: body}, nil
}

// FormatProxmoxStyle is an alias for FormatMessage.
func FormatProxmoxStyle(hostLabel string, e monitor.Event) (Message, error) {
	return FormatMessage(hostLabel, e)
}

func sanitizeHostLabel(hostLabel string) string {
	hostLabel = strings.TrimSpace(hostLabel)
	if hostLabel == "" {
		return "pve-host"
	}
	if len(hostLabel) > 64 {
		return hostLabel[:64]
	}
	return hostLabel
}
