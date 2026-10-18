package main

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/faridze/pve-zfs-monitor/internal/release"
)

func TestVersionJSON(t *testing.T) {
	var out, errOut bytes.Buffer
	code := runVersion([]string{"--json"}, &out, &errOut)
	if code != 0 {
		t.Fatalf("exit=%d err=%s", code, errOut.String())
	}
	var info release.Info
	if err := json.Unmarshal(out.Bytes(), &info); err != nil {
		t.Fatal(err)
	}
	if info.Version == "" {
		t.Fatal("missing version")
	}
	if info.TrustedRelease {
		t.Fatal("default build must not be trusted")
	}
}
