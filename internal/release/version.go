package release

import (
	"encoding/json"
	"fmt"
	"runtime"
	"strings"
)

// Channel identifies a release distribution stream.
type Channel string

const (
	ChannelStable Channel = "stable"
	ChannelLab    Channel = "lab"
)

// Build metadata injected at link time (see Makefile / CI).
var (
	Version     = "0.0.0-dev"
	Commit      = "unknown"
	BuildTime   = "unknown"
	BuildGOOS   = runtime.GOOS
	BuildGOARCH = runtime.GOARCH
	ChannelName = string(ChannelLab)
	// TrustedRelease is true only for maintainer-signed official builds (ldflags).
	TrustedRelease = "false"
)

// Info is machine-readable build and trust metadata.
type Info struct {
	Version        string `json:"version"`
	Commit         string `json:"commit"`
	BuildTime      string `json:"build_time"`
	GOOS           string `json:"goos"`
	GOARCH         string `json:"goarch"`
	Channel        string `json:"channel"`
	TrustedRelease bool   `json:"trusted_release"`
	Development    bool   `json:"development"`
}

// CurrentInfo returns metadata for the running binary.
func CurrentInfo() Info {
	trusted := strings.EqualFold(TrustedRelease, "true")
	dev := Version == "0.0.0-dev" || Commit == "unknown" || !trusted
	return Info{
		Version:        Version,
		Commit:         Commit,
		BuildTime:      BuildTime,
		GOOS:           BuildGOOS,
		GOARCH:         BuildGOARCH,
		Channel:        ChannelName,
		TrustedRelease: trusted,
		Development:    dev,
	}
}

// String is a human-readable one-line version summary.
func (i Info) String() string {
	tag := i.Version
	if i.Development {
		tag += " (development; not a trusted release)"
	}
	return fmt.Sprintf("%s commit=%s built=%s %s/%s channel=%s",
		tag, shortCommit(i.Commit), i.BuildTime, i.GOOS, i.GOARCH, i.Channel)
}

func shortCommit(c string) string {
	if len(c) > 12 {
		return c[:12]
	}
	return c
}

// MarshalJSON for Info is used by the CLI.
func (i Info) MarshalJSON() ([]byte, error) {
	type alias Info
	return json.Marshal(alias(i))
}
