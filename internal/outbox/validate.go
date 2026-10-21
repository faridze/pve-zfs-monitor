package outbox

import (
	"regexp"
	"strings"

	"github.com/faridze/pve-zfs-monitor/internal/monitor"
)

var (
	idPattern      = regexp.MustCompile(`^[0-9a-f]{32}$`)
	logicalIDPat   = regexp.MustCompile(`^[0-9a-f]{32}$`)
	secretPatterns = []*regexp.Regexp{
		regexp.MustCompile(`(?i)[a-z0-9._%+-]+@[a-z0-9.-]+\.[a-z]{2,}`),
		regexp.MustCompile(`(?i)smtp[s]?://`),
		regexp.MustCompile(`(?i)password=`),
		regexp.MustCompile(`(?i)auth=`),
	}
)

func validEntryID(id string) bool {
	return idPattern.MatchString(id)
}

func validLogicalID(id string) bool {
	return logicalIDPat.MatchString(id)
}

func sanitizeTransportError(msg string) string {
	msg = strings.TrimSpace(msg)
	if msg == "" {
		return "delivery failed"
	}
	for _, re := range secretPatterns {
		if re.MatchString(msg) {
			return "delivery failed"
		}
	}
	if len(msg) > 120 {
		msg = msg[:120]
	}
	return msg
}

func entryLogicalID(ev monitor.Event) (string, error) {
	if ev.EmittedUnix <= 0 {
		return "", errInvalidEntry
	}
	return eventFingerprint(ev), nil
}
