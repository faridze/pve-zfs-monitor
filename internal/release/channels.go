package release

import (
	"fmt"
	"os"
	"strings"
)

// DefaultReleaseBaseURL may be set at link time for official builds.
var DefaultReleaseBaseURL = ""

// ResolveChannelURLs builds HTTPS URLs for manifest and signature objects.
func ResolveChannelURLs(channel Channel, version string) (ChannelURLs, error) {
	base := strings.TrimRight(strings.TrimSpace(DefaultReleaseBaseURL), "/")
	if base == "" {
		base = strings.TrimRight(strings.TrimSpace(os.Getenv("PVE_ZFS_MONITOR_RELEASE_BASE")), "/")
	}
	if base == "" {
		return ChannelURLs{}, fmt.Errorf("release base URL not configured (build-time DefaultReleaseBaseURL or PVE_ZFS_MONITOR_RELEASE_BASE)")
	}
	if channel != ChannelStable && channel != ChannelLab {
		return ChannelURLs{}, fmt.Errorf("invalid channel %q", channel)
	}
	ver := strings.TrimSpace(version)
	if ver == "" {
		ver = "latest"
	}
	prefix := fmt.Sprintf("%s/%s/%s", base, channel, ver)
	return ChannelURLs{
		ManifestURL:  prefix + "/manifest.json",
		SignatureURL: prefix + "/manifest.json.sig.json",
	}, nil
}

// ParseChannel validates a channel string from CLI flags.
func ParseChannel(s string) (Channel, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "stable":
		return ChannelStable, nil
	case "lab":
		return ChannelLab, nil
	default:
		return "", fmt.Errorf("unknown channel %q (use stable or lab)", s)
	}
}
