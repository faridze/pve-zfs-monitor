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
)

const (
	DefaultMaxStdoutBytes = 1 << 20  // 1 MiB
	DefaultMaxStderrBytes = 64 << 10 // 64 KiB
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

	var out Output
	var stdoutTrunc, stderrTrunc bool
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		out.Stdout, stdoutTrunc = readLimited(stdoutPipe, r.Limits.MaxStdoutBytes)
		wg.Done()
	}()
	go func() {
		out.Stderr, stderrTrunc = readLimited(stderrPipe, r.Limits.MaxStderrBytes)
		wg.Done()
	}()
	wg.Wait()

	waitErr := cmd.Wait()
	if stdoutTrunc || stderrTrunc {
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

func readLimited(r io.Reader, max int) ([]byte, bool) {
	if max <= 0 {
		return nil, true
	}
	var buf bytes.Buffer
	_, _ = io.Copy(&buf, io.LimitReader(r, int64(max)+1))
	data := buf.Bytes()
	if len(data) > max {
		return data[:max], true
	}
	return data, false
}

func trimForMessage(stderr []byte) string {
	s := strings.TrimSpace(string(stderr))
	if len(s) > 200 {
		return s[:200] + "..."
	}
	return s
}
