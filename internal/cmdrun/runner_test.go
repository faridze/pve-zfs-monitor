package cmdrun

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestRunMissingExecutable(t *testing.T) {
	r := NewOSRunner(DefaultLimits())
	_, err := r.Run(context.Background(), "/nonexistent/pve-zfs-zpool", nil)
	var runErr *Error
	if !errors.As(err, &runErr) || runErr.Kind != NotAvailable {
		t.Fatalf("expected NotAvailable, got %v", err)
	}
}

func TestRunSuccessCapturesStdoutStderrSeparately(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix shell helper")
	}
	script := filepath.Join(t.TempDir(), "echo.sh")
	if err := os.WriteFile(script, []byte("#!/bin/sh\nprintf 'out'; printf 'err' 1>&2\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	r := NewOSRunner(DefaultLimits())
	out, err := r.Run(context.Background(), script, nil)
	if err != nil {
		t.Fatal(err)
	}
	if string(out.Stdout) != "out" || string(out.Stderr) != "err" {
		t.Fatalf("unexpected output: stdout=%q stderr=%q", out.Stdout, out.Stderr)
	}
}

func TestRunNonZeroExitIncludesStderr(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix shell helper")
	}
	script := filepath.Join(t.TempDir(), "fail.sh")
	if err := os.WriteFile(script, []byte("#!/bin/sh\necho problem 1>&2\nexit 7\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	r := NewOSRunner(DefaultLimits())
	_, err := r.Run(context.Background(), script, nil)
	var runErr *Error
	if !errors.As(err, &runErr) || runErr.Kind != ExitError || runErr.ExitCode != 7 {
		t.Fatalf("expected exit 7, got %v", err)
	}
	if !strings.Contains(runErr.Error(), "problem") {
		t.Fatalf("stderr not in error: %q", runErr.Error())
	}
}

func TestRunTruncatedStdoutFails(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix shell helper")
	}
	script := filepath.Join(t.TempDir(), "spam.sh")
	if err := os.WriteFile(script, []byte("#!/bin/sh\nyes A | head -c 32\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	r := NewOSRunner(Limits{MaxStdoutBytes: 8, MaxStderrBytes: 64})
	_, err := r.Run(context.Background(), script, nil)
	var runErr *Error
	if !errors.As(err, &runErr) || runErr.Kind != Truncated {
		t.Fatalf("expected Truncated, got %v", err)
	}
}

func TestRunContinuousStdoutDoesNotHang(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix shell helper")
	}
	script := filepath.Join(t.TempDir(), "flood.sh")
	if err := os.WriteFile(script, []byte("#!/bin/sh\nwhile :; do printf '0123456789'; done\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	r := NewOSRunner(Limits{MaxStdoutBytes: 512, MaxStderrBytes: 64})
	done := make(chan error, 1)
	go func() {
		_, err := r.Run(context.Background(), script, nil)
		done <- err
	}()
	select {
	case err := <-done:
		var runErr *Error
		if !errors.As(err, &runErr) || runErr.Kind != Truncated {
			t.Fatalf("expected Truncated, got %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run hung on continuous stdout")
	}
}

func TestRunContinuousStderrDoesNotHang(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix shell helper")
	}
	script := filepath.Join(t.TempDir(), "flood-err.sh")
	if err := os.WriteFile(script, []byte("#!/bin/sh\nwhile :; do printf 'e' 1>&2; done\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	r := NewOSRunner(Limits{MaxStdoutBytes: 4096, MaxStderrBytes: 128})
	done := make(chan error, 1)
	go func() {
		_, err := r.Run(context.Background(), script, nil)
		done <- err
	}()
	select {
	case err := <-done:
		var runErr *Error
		if !errors.As(err, &runErr) || runErr.Kind != Truncated {
			t.Fatalf("expected Truncated, got %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run hung on continuous stderr")
	}
}

func TestRunTimeout(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix sleep helper")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	r := NewOSRunner(DefaultLimits())
	_, err := r.Run(ctx, "/bin/sleep", []string{"2"})
	var runErr *Error
	if !errors.As(err, &runErr) || runErr.Kind != Timeout {
		t.Fatalf("expected Timeout, got %v", err)
	}
}

func TestRunTimeoutDoesNotLeaveSleepOrphan(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("linux /proc check")
	}
	marker := filepath.Join(t.TempDir(), "sleep-marker")
	script := filepath.Join(t.TempDir(), "marked-sleep.sh")
	body := "#!/bin/sh\necho $$ >" + marker + "\nexec sleep 30\n"
	if err := os.WriteFile(script, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 80*time.Millisecond)
	defer cancel()
	r := NewOSRunner(DefaultLimits())
	_, _ = r.Run(ctx, script, nil)
	time.Sleep(100 * time.Millisecond)
	pidBytes, err := os.ReadFile(marker)
	if err != nil {
		t.Fatalf("marker file: %v", err)
	}
	pid := strings.TrimSpace(string(pidBytes))
	if _, err := strconv.Atoi(pid); err != nil {
		t.Fatalf("invalid pid %q", pid)
	}
	if _, err := os.Stat("/proc/" + pid); err == nil {
		t.Fatalf("subprocess %s still running after timeout", pid)
	}
}

type fakeRunner struct {
	fn func(ctx context.Context, path string, args []string) (Output, error)
}

func (f fakeRunner) Run(ctx context.Context, path string, args []string) (Output, error) {
	return f.fn(ctx, path, args)
}

func TestFakeRunnerForCollectors(t *testing.T) {
	fr := fakeRunner{fn: func(_ context.Context, path string, args []string) (Output, error) {
		if path != "/usr/sbin/zpool" {
			t.Fatalf("unexpected path %s", path)
		}
		return Output{Stdout: []byte("rpool\tONLINE\n")}, nil
	}}
	out, err := fr.Run(context.Background(), "/usr/sbin/zpool", []string{"list"})
	if err != nil || string(out.Stdout) == "" {
		t.Fatalf("fake runner failed: %v", err)
	}
}
