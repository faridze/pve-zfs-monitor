package main

import (
	"context"
	"crypto/ed25519"
	"fmt"
	"io"
	"runtime"
	"strings"
	"time"

	"github.com/faridze/pve-zfs-monitor/internal/release"
)

func runUpdate(args []string, stdout, stderr io.Writer) int {
	checkOnly := false
	dryRun := false
	channel := strings.TrimSpace(release.ChannelName)
	for _, a := range args {
		switch a {
		case "--check":
			checkOnly = true
		case "--dry-run":
			dryRun = true
		case "--help":
			fmt.Fprintln(stdout, "Usage: pve-zfs-monitor update [--check] [--dry-run]")
			fmt.Fprintln(stdout, "Requires root for installation. Network access occurs only for update/check.")
			return 0
		default:
			if strings.HasPrefix(a, "--channel=") {
				ch, err := release.ParseChannel(strings.TrimPrefix(a, "--channel="))
				if err != nil {
					fmt.Fprintln(stderr, err.Error())
					return 64
				}
				channel = string(ch)
			} else {
				fmt.Fprintf(stderr, "unknown update flag %q\n", a)
				return 64
			}
		}
	}
	if !checkOnly && !dryRun && !release.HasRootPrivileges() {
		fmt.Fprintln(stderr, "update: root privileges required (use sudo)")
		return 77
	}
	pub, err := release.LoadTrustedPublicKey()
	if err != nil {
		fmt.Fprintln(stderr, "update: trusted signing key not configured:", err)
		return 3
	}
	current := release.CurrentInfo()
	if cfg, err := release.LoadPersistedConfig(); err == nil {
		channel = string(cfg.Channel)
	}
	if current.Channel != "" && channel != current.Channel {
		fmt.Fprintf(stderr, "update: refusing channel switch from installed %q to %q\n", current.Channel, channel)
		return 64
	}
	secure := release.SecureHTTPClient{UserAgent: "pve-zfs-monitor/" + current.Version}
	up := release.Updater{
		Secure:           secure,
		PublicKey:        pub,
		Current:          current,
		Channel:          release.Channel(channel),
		InstallOpts:      release.DefaultInstallOptions(),
		ExecutableGOOS:   runtime.GOOS,
		ExecutableGOARCH: runtime.GOARCH,
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	plan, _, _, err := up.CheckForUpdate(ctx)
	if err != nil {
		fmt.Fprintf(stderr, "update check failed: %v\n", err)
		return 3
	}
	if checkOnly {
		fmt.Fprintf(stdout, "update available: %s -> %s (%s)\n", current.Version, plan.Available.Version, plan.Available.Commit)
		return 0
	}
	if dryRun {
		if err := up.ApplyUpdate(ctx, plan, true); err != nil {
			fmt.Fprintf(stderr, "dry-run failed: %v\n", err)
			return 3
		}
		fmt.Fprintf(stdout, "dry-run: manifest signature, artifact size and sha256 verified for %s\n", plan.Artifact.Filename)
		return 0
	}
	if err := up.ApplyUpdate(ctx, plan, false); err != nil {
		fmt.Fprintf(stderr, "update failed: %v\n", err)
		return 3
	}
	fmt.Fprintf(stdout, "updated to %s\n", plan.Available.Version)
	return 0
}

func runRollback(stdout, stderr io.Writer) int {
	if !release.HasRootPrivileges() {
		fmt.Fprintln(stderr, "rollback: root privileges required (use sudo)")
		return 77
	}
	if err := release.Rollback(release.DefaultInstallOptions()); err != nil {
		fmt.Fprintf(stderr, "rollback failed: %v\n", err)
		return 3
	}
	fmt.Fprintln(stdout, "rollback complete")
	return 0
}

func runInstallRelease(args []string, stdout, stderr io.Writer) int {
	var manifestPath, sigPath, binPath, releaseBase, channelStr, version string
	var trustKeyFile string
	for _, a := range args {
		if strings.HasPrefix(a, "--manifest=") {
			manifestPath = strings.TrimPrefix(a, "--manifest=")
		} else if strings.HasPrefix(a, "--signature=") {
			sigPath = strings.TrimPrefix(a, "--signature=")
		} else if strings.HasPrefix(a, "--binary=") {
			binPath = strings.TrimPrefix(a, "--binary=")
		} else if strings.HasPrefix(a, "--release-base=") {
			releaseBase = strings.TrimPrefix(a, "--release-base=")
		} else if strings.HasPrefix(a, "--channel=") {
			channelStr = strings.TrimPrefix(a, "--channel=")
		} else if strings.HasPrefix(a, "--version=") {
			version = strings.TrimPrefix(a, "--version=")
		} else if strings.HasPrefix(a, "--trust-key-file=") {
			trustKeyFile = strings.TrimPrefix(a, "--trust-key-file=")
		}
	}
	if manifestPath == "" || sigPath == "" || binPath == "" {
		fmt.Fprintln(stderr, "install-release requires --manifest= --signature= --binary=")
		return 64
	}
	if !release.HasRootPrivileges() {
		fmt.Fprintln(stderr, "install-release: root privileges required")
		return 77
	}
	var pub ed25519.PublicKey
	var err error
	if trustKeyFile != "" {
		pub, err = release.LoadPublicKeyFromFile(trustKeyFile)
	} else {
		pub, err = release.LoadTrustedPublicKey()
	}
	if err != nil {
		fmt.Fprintln(stderr, err.Error())
		return 3
	}
	ch, err := release.ParseChannel(channelStr)
	if err != nil && channelStr != "" {
		fmt.Fprintln(stderr, err.Error())
		return 64
	}
	if channelStr == "" {
		ch = release.Channel(release.ChannelName)
	}
	if err := release.InstallRelease(release.InstallReleaseRequest{
		ManifestPath:       manifestPath,
		SignaturePath:      sigPath,
		ArtifactPath:       binPath,
		PublicKey:          pub,
		Opts:               release.DefaultInstallOptions(),
		ReleaseBase:        releaseBase,
		Channel:            ch,
		ExpectedVersion:    version,
		SourceTrustKeyFile: trustKeyFile,
	}); err != nil {
		fmt.Fprintf(stderr, "install failed: %v\n", err)
		return 3
	}
	fmt.Fprintln(stdout, "installation complete")
	return 0
}
