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
	"unicode/utf8"
)

const boundaryLimit = 64

func writeEmitScript(t *testing.T, dir string, name string, body string) string {
	script := filepath.Join(dir, name)
	if err := os.WriteFile(script, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	return script
}

func emitBytesScript(dir string) string {
	return filepath.Join(dir, "emit-bytes.sh")
}

func writeEmitBytesHelper(t *testing.T, dir string) {
	body := `#!/bin/sh
n="$1"
stream="$2"
i=0
while [ "$i" -lt "$n" ]; do
  if [ "$stream" = "stderr" ]; then
    printf 'e'
  else
    printf 'o'
  fi
  i=$((i+1))
done
if [ "$stream" = "stderr" ]; then
  printf '' 1>&2
  i=0
  while [ "$i" -lt "$n" ]; do
    printf 'e' 1>&2
    i=$((i+1))
  done
else
  i=0
  while [ "$i" -lt "$n" ]; do
    printf 'o'
    i=$((i+1))
  done
fi
`
	// Fix script - the above is wrong. Simpler version:
	body = `#!/bin/sh
n="$1"
stream="$2"
i=0
while [ "$i" -lt "$n" ]; do
  if [ "$stream" = "stderr" ]; then
    printf 'e' 1>&2
  else
    printf 'o'
  fi
  i=$((i+1))
done
`
	writeEmitScript(t, dir, "emit-bytes.sh", body)
}

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

func TestRunStdoutByteBoundaries(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix shell helper")
	}
	dir := t.TempDir()
	writeEmitBytesHelper(t, dir)
	script := emitBytesScript(dir)
	limits := Limits{MaxStdoutBytes: boundaryLimit, MaxStderrBytes: boundaryLimit}
	cases := []struct {
		n         int
		wantTrunc bool
		wantLen   int
	}{
		{boundaryLimit - 1, false, boundaryLimit - 1},
		{boundaryLimit, false, boundaryLimit},
		{boundaryLimit + 1, true, boundaryLimit},
	}
	for _, tc := range cases {
		r := NewOSRunner(limits)
		out, err := r.Run(context.Background(), script, []string{strconv.Itoa(tc.n), "stdout"})
		var runErr *Error
		if tc.wantTrunc {
			if !errors.As(err, &runErr) || runErr.Kind != Truncated {
				t.Fatalf("n=%d: expected Truncated, got %v", tc.n, err)
			}
			if len(out.Stdout) != tc.wantLen {
				t.Fatalf("n=%d: stdout len=%d want %d", tc.n, len(out.Stdout), tc.wantLen)
			}
			continue
		}
		if err != nil {
			t.Fatalf("n=%d: unexpected error %v", tc.n, err)
		}
		if len(out.Stdout) != tc.wantLen {
			t.Fatalf("n=%d: stdout len=%d want %d", tc.n, len(out.Stdout), tc.wantLen)
		}
	}
}

func TestRunStderrByteBoundaries(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix shell helper")
	}
	dir := t.TempDir()
	writeEmitBytesHelper(t, dir)
	script := emitBytesScript(dir)
	limits := Limits{MaxStdoutBytes: boundaryLimit, MaxStderrBytes: boundaryLimit}
	cases := []struct {
		n         int
		wantTrunc bool
		wantLen   int
	}{
		{boundaryLimit - 1, false, boundaryLimit - 1},
		{boundaryLimit, false, boundaryLimit},
		{boundaryLimit + 1, true, boundaryLimit},
	}
	for _, tc := range cases {
		r := NewOSRunner(limits)
		out, err := r.Run(context.Background(), script, []string{strconv.Itoa(tc.n), "stderr"})
		var runErr *Error
		if tc.wantTrunc {
			if !errors.As(err, &runErr) || runErr.Kind != Truncated {
				t.Fatalf("n=%d: expected Truncated, got %v", tc.n, err)
			}
			if len(out.Stderr) != tc.wantLen {
				t.Fatalf("n=%d: stderr len=%d want %d", tc.n, len(out.Stderr), tc.wantLen)
			}
			continue
		}
		if err != nil {
			t.Fatalf("n=%d: unexpected error %v", tc.n, err)
		}
		if len(out.Stderr) != tc.wantLen {
			t.Fatalf("n=%d: stderr len=%d want %d", tc.n, len(out.Stderr), tc.wantLen)
		}
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

func TestRunLinuxDescendantOverflowDoesNotHang(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("linux descendant fixture")
	}
	dir := t.TempDir()
	marker := filepath.Join(dir, "child.pid")
	script := writeEmitScript(t, dir, "descendant-flood.sh", `#!/bin/sh
( while :; do printf 'x'; done ) &
echo $! > "`+marker+`"
wait
`)
	r := NewOSRunner(Limits{MaxStdoutBytes: 256, MaxStderrBytes: 64})
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
		t.Fatal("Run hung with descendant holding stdout")
	}
	time.Sleep(50 * time.Millisecond)
	pidBytes, err := os.ReadFile(marker)
	if err == nil {
		pid := strings.TrimSpace(string(pidBytes))
		if pid != "" && isProcAlive(pid) {
			t.Fatalf("descendant %s still running after overflow kill", pid)
		}
	}
}

func TestRunLinuxDescendantTimeoutDoesNotHang(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("linux descendant fixture")
	}
	dir := t.TempDir()
	marker := filepath.Join(dir, "child.pid")
	script := writeEmitScript(t, dir, "descendant-sleep.sh", `#!/bin/sh
( sleep 300 ) &
echo $! > "`+marker+`"
wait
`)
	ctx, cancel := context.WithTimeout(context.Background(), 80*time.Millisecond)
	defer cancel()
	r := NewOSRunner(DefaultLimits())
	done := make(chan error, 1)
	go func() {
		_, err := r.Run(ctx, script, nil)
		done <- err
	}()
	select {
	case err := <-done:
		var runErr *Error
		if !errors.As(err, &runErr) || runErr.Kind != Timeout {
			t.Fatalf("expected Timeout, got %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run hung with descendant during timeout")
	}
	time.Sleep(50 * time.Millisecond)
	pidBytes, err := os.ReadFile(marker)
	if err == nil {
		pid := strings.TrimSpace(string(pidBytes))
		if pid != "" && isProcAlive(pid) {
			t.Fatalf("descendant %s still running after timeout", pid)
		}
	}
}

func TestTrimForMessageUTF8Safe(t *testing.T) {
	var b strings.Builder
	for i := 0; i < 250; i++ {
		b.WriteRune('é')
	}
	msg := trimForMessage([]byte(b.String()))
	if !strings.HasSuffix(msg, "...") {
		t.Fatalf("expected ellipsis suffix, got %q", msg)
	}
	base := strings.TrimSuffix(msg, "...")
	if utf8.RuneCountInString(base) != 200 {
		t.Fatalf("expected 200 runes before ellipsis, got %d", utf8.RuneCountInString(base))
	}
	if !utf8.ValidString(msg) {
		t.Fatalf("invalid utf-8: %q", msg)
	}
}

func isProcAlive(pid string) bool {
	_, err := os.Stat("/proc/" + pid)
	return err == nil
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
