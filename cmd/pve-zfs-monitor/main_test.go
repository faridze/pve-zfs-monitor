package main

import (
    "bytes"
    "strings"
    "testing"
)

func TestSafeCLICommands(t *testing.T) {
    var out, errOut bytes.Buffer
    if code := run([]string{"version"}, &out, &errOut); code != 0 || out.Len() == 0 {
        t.Fatalf("version: code=%d out=%q", code, out.String())
    }
    out.Reset()
    if code := run([]string{"invalid"}, &out, &errOut); code != 64 {
        t.Fatalf("unknown command: code=%d", code)
    }
    errOut.Reset()
    if code := run([]string{"check", "--dangerous"}, &out, &errOut); code != 64 || !strings.Contains(errOut.String(), "invalid") {
        t.Fatalf("invalid arguments: code=%d stderr=%q", code, errOut.String())
    }
}
