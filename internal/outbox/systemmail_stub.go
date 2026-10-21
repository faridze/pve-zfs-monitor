package outbox

import (
	"context"
	"errors"
)

// SystemMailTransport is a disabled prototype for Proxmox sendmail integration.
// Enabling requires explicit lab validation; it is never used in CI.
type SystemMailTransport struct {
	Enabled bool
	// SendmailPath is fixed when enabled (e.g. /usr/sbin/sendmail).
	SendmailPath string
}

func (t *SystemMailTransport) Send(ctx context.Context, msg Message) error {
	if !t.Enabled {
		return errors.New("system mail transport disabled")
	}
	if t.SendmailPath == "" {
		return errors.New("sendmail path not configured")
	}
	return errors.New("system mail transport not validated in this build")
}
