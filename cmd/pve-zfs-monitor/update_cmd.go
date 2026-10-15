package main

import (
	"context"
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
	if current.Channel != "" && channel != current.Channel {
		fmt.Fprintf(stderr, "update: refusing channel switch from installed %q to %q\n", current.Channel, channel)
		return 64
	}
	up := release.Updater{
		Client:           release.NetHTTPClient{UserAgent: "pve-zfs-monitor/" + current.Version},
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
	if checkOnly || dryRun {
		fmt.Fprintf(stdout, "update available: %s -> %s (%s)\n", current.Version, plan.Available.Version, plan.Available.Commit)
		if dryRun {
			fmt.Fprintln(stdout, "dry-run: verification OK; no files changed")
		}
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
	var manifestPath, sigPath, binPath string
	for _, a := range args {
		if strings.HasPrefix(a, "--manifest=") {
			manifestPath = strings.TrimPrefix(a, "--manifest=")
		} else if strings.HasPrefix(a, "--signature=") {
			sigPath = strings.TrimPrefix(a, "--signature=")
		} else if strings.HasPrefix(a, "--binary=") {
			binPath = strings.TrimPrefix(a, "--binary=")
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
	pub, err := release.LoadTrustedPublicKey()
	if err != nil {
		fmt.Fprintln(stderr, err.Error())
		return 3
	}
	if err := release.InstallFromPaths(manifestPath, sigPath, binPath, pub, release.DefaultInstallOptions()); err != nil {
		fmt.Fprintf(stderr, "install failed: %v\n", err)
		return 3
	}
	fmt.Fprintln(stdout, "installation complete")
	return 0
}
