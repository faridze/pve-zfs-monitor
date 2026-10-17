package main

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/faridze/pve-zfs-monitor/internal/release"
)

func runVersion(args []string, stdout io.Writer, stderr io.Writer) int {
	jsonOut := false
	for _, a := range args {
		if a == "--json" {
			jsonOut = true
		} else if a == "--help" {
			fmt.Fprintln(stdout, "Usage: pve-zfs-monitor version [--json]")
			return 0
		} else {
			fmt.Fprintf(stderr, "unknown version flag %q\n", a)
			return 64
		}
	}
	info := release.CurrentInfo()
	if jsonOut {
		if err := json.NewEncoder(stdout).Encode(info); err != nil {
			fmt.Fprintf(stderr, "encode: %v\n", err)
			return 3
		}
		return 0
	}
	fmt.Fprintln(stdout, info.String())
	return 0
}
