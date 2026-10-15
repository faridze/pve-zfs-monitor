// Package cmdrun provides bounded, context-aware execution of fixed-path commands.
package cmdrun

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"unicode"
)

const (
	DefaultMaxStdoutBytes = 1 << 20  // 1 MiB
	DefaultMaxStderrBytes = 64 << 10 // 64 KiB
	maxStderrMessageRunes = 200
)

// Limits caps captured stdout and stderr per invocation.
type Limits struct {
	MaxStdoutBytes int
	MaxStderrBytes int
}

// DefaultLimits returns conservative defaults for collector commands.
func DefaultLimits() Limits {
	return Limits{
		MaxStdoutBytes: DefaultMaxStdoutBytes,
		MaxStderrBytes: DefaultMaxStderrBytes,
	}
}

// Output holds bounded command stdout and stderr.
type Output struct {
	Stdout []byte
	Stderr []byte
}

// Kind classifies a failed Run.
type Kind int

const (
	NotAvailable Kind = 1
	Timeout      Kind = 2
	ExitError    Kind = 3
	Truncated    Kind = 4
	ReadError    Kind = 5
)

// Error describes a failed command execution.
type Error struct {
	Kind     Kind
	Path     string
	Args     []string
	ExitCode int
	Stdout   []byte
	Stderr   []byte
	Cause    error
}

func (e *Error) Error() string {
	switch e.Kind {
	case NotAvailable:
		return fmt.Sprintf("%s not available", e.Path)
	case Timeout:
		return fmt.Sprintf("%s timed out: %v", e.Path, e.Cause)
	case Truncated:
		return fmt.Sprintf("%s output truncated (stdout=%d stderr=%d bytes captured)", e.Path, len(e.Stdout), len(e.Stderr))
	case ReadError:
		return fmt.Sprintf("%s output read failed: %v", e.Path, e.Cause)
	case ExitError:
		msg := trimForMessage(e.Stderr)
		if msg == "" {
			return fmt.Sprintf("%s exited with status %d", e.Path, e.ExitCode)
		}
		return fmt.Sprintf("%s exited with status %d: %s", e.Path, e.ExitCode, msg)
	default:
		return fmt.Sprintf("%s failed", e.Path)
	}
}

func (e *Error) Unwrap() error { return e.Cause }

// Runner executes a command without shell interpolation.
type Runner interface {
	Run(ctx context.Context, path string, args []string) (Output, error)
}

// OSRunner runs commands on the local host with bounded I/O.
type OSRunner struct {
	Limits Limits
}

// NewOSRunner constructs a runner with the given output limits.
func NewOSRunner(limits Limits) *OSRunner {
	return &OSRunner{Limits: limits}
}

type streamRead struct {
	data      []byte
	truncated bool
	err       error
}

// Run executes path with args under ctx. Truncated output is a failure.
func (r *OSRunner) Run(ctx context.Context, path string, args []string) (Output, error) {
	if err := statExecutable(path); err != nil {
		return Output{}, &Error{Kind: NotAvailable, Path: path, Args: args, Cause: err}
	}

	cmd := exec.CommandContext(ctx, path, args...)
	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return Output{}, fmt.Errorf("stdout pipe: %w", err)
	}
	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		return Output{}, fmt.Errorf("stderr pipe: %w", err)
	}
	if err := cmd.Start(); err != nil {
		if ctx.Err() != nil {
			return Output{}, &Error{
				Kind:  Timeout,
				Path:  path,
				Args:  args,
				Cause: ctx.Err(),
			}
		}
		return Output{}, fmt.Errorf("start %s: %w", path, err)
	}

	var killOnce sync.Once
	killProcess := func() {
		killOnce.Do(func() {
			if cmd.Process != nil {
				_ = cmd.Process.Kill()
			}
		})
	}

	var stdoutRead, stderrRead streamRead
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		stdoutRead = readBounded(stdoutPipe, r.Limits.MaxStdoutBytes, killProcess)
		wg.Done()
	}()
	go func() {
		stderrRead = readBounded(stderrPipe, r.Limits.MaxStderrBytes, killProcess)
		wg.Done()
	}()
	wg.Wait()

	out := Output{Stdout: stdoutRead.data, Stderr: stderrRead.data}
	waitErr := cmd.Wait()

	if stdoutRead.err != nil {
		return out, &Error{
			Kind:   ReadError,
			Path:   path,
			Args:   args,
			Stdout: out.Stdout,
			Stderr: out.Stderr,
			Cause:  stdoutRead.err,
		}
	}
	if stderrRead.err != nil {
		return out, &Error{
			Kind:   ReadError,
			Path:   path,
			Args:   args,
			Stdout: out.Stdout,
			Stderr: out.Stderr,
			Cause:  stderrRead.err,
		}
	}
	if stdoutRead.truncated || stderrRead.truncated {
		return out, &Error{
			Kind:   Truncated,
			Path:   path,
			Args:   args,
			Stdout: out.Stdout,
			Stderr: out.Stderr,
			Cause:  errors.New("output limit exceeded"),
		}
	}
	if ctx.Err() != nil {
		return out, &Error{
			Kind:   Timeout,
			Path:   path,
			Args:   args,
			Stdout: out.Stdout,
			Stderr: out.Stderr,
			Cause:  ctx.Err(),
		}
	}
	if waitErr != nil {
		exitCode := 1
		var ee *exec.ExitError
		if errors.As(waitErr, &ee) {
			exitCode = ee.ExitCode()
		}
		return out, &Error{
			Kind:     ExitError,
			Path:     path,
			Args:     args,
			ExitCode: exitCode,
			Stdout:   out.Stdout,
			Stderr:   out.Stderr,
			Cause:    waitErr,
		}
	}
	return out, nil
}

func statExecutable(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if info.IsDir() {
		return fmt.Errorf("is a directory")
	}
	return nil
}

// readBounded captures up to max bytes and always drains the reader to EOF.
// When the limit is exceeded, onExceeded is invoked once so the subprocess can be terminated.
func readBounded(r io.Reader, max int, onExceeded func()) streamRead {
	if max <= 0 {
		onExceeded()
		_, err := io.Copy(io.Discard, r)
		return streamRead{truncated: true, err: normalizeReadErr(err)}
	}

	var buf bytes.Buffer
	buf.Grow(min(max, 4096))
	scratch := make([]byte, 32*1024)
	truncated := false
	for {
		n, err := r.Read(scratch)
		if n > 0 {
			if !truncated {
				remaining := max - buf.Len()
				if remaining > 0 {
					if n > remaining {
						buf.Write(scratch[:remaining])
						truncated = true
						onExceeded()
					} else {
						buf.Write(scratch[:n])
					}
				}
				if buf.Len() >= max {
					truncated = true
					onExceeded()
				}
			}
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				return streamRead{data: buf.Bytes(), truncated: truncated, err: nil}
			}
			return streamRead{data: buf.Bytes(), truncated: truncated, err: err}
		}
	}
}

func normalizeReadErr(err error) error {
	if err == nil || errors.Is(err, io.EOF) {
		return nil
	}
	return err
}

func trimForMessage(stderr []byte) string {
	var b strings.Builder
	for _, r := range strings.TrimSpace(string(stderr)) {
		if r == '\n' || r == '\r' || r == '\t' {
			b.WriteRune(' ')
			continue
		}
		if unicode.IsControl(r) {
			continue
		}
		b.WriteRune(r)
	}
	s := strings.TrimSpace(b.String())
	if len(s) > maxStderrMessageRunes {
		return s[:maxStderrMessageRunes] + "..."
	}
	return s
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
