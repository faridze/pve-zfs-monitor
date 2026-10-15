package zfs

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/faridze/pve-zfs-monitor/internal/cmdrun"
)

func TestParseList(t *testing.T) {
	pools, err := ParseList([]byte("rpool\tONLINE\nbackup\tDEGRADED\n"))
	if err != nil || len(pools) != 2 {
		t.Fatalf("unexpected parse result: %#v, %v", pools, err)
	}
	if pools[0].Name != "rpool" || pools[1].Health != "DEGRADED" {
		t.Fatalf("unexpected pools: %#v", pools)
	}
}

func TestParseListRejectsIncompleteOrDuplicate(t *testing.T) {
	for _, input := range []string{
		"", "rpool\n", "rpool\t\n", "rpool\tONLINE\trandom\n",
		"rpool\tONLINE\nrpool\tONLINE\n",
	} {
		if _, err := ParseList([]byte(input)); err == nil {
			t.Errorf("accepted invalid input: %q", input)
		}
	}
}

func TestAssessFailClosed(t *testing.T) {
	for _, tc := range []struct {
		health string
		want   Result
	}{
		{"ONLINE", Healthy},
		{"DEGRADED", Unhealthy},
		{"FAULTED", Unhealthy},
		{"UNAVAIL", Unhealthy},
		{"FUTURE_STATUS", Unknown},
	} {
		got := Assess([]Pool{{Name: "rpool", Health: tc.health}})
		if got != tc.want {
			t.Errorf("%s: got %v, want %v", tc.health, got, tc.want)
		}
	}
	if got := Assess(nil); got != Unknown {
		t.Errorf("empty pool list: got %v", got)
	}
	if got := Assess([]Pool{{"rpool", "ONLINE"}, {"data", "DEGRADED"}}); got != Unhealthy {
		t.Errorf("mixed pools: got %v", got)
	}
}

type stubRunner struct {
	out      cmdrun.Output
	err      error
	lastPath string
	lastArgs []string
}

func (s *stubRunner) Run(_ context.Context, path string, args []string) (cmdrun.Output, error) {
	s.lastPath = path
	s.lastArgs = append([]string(nil), args...)
	return s.out, s.err
}

func TestPoolCollectorListPoolsSuccess(t *testing.T) {
	stub := &stubRunner{out: cmdrun.Output{Stdout: []byte("rpool\tONLINE\n")}}
	c := NewPoolCollector(stub)
	pools, err := c.ListPools(context.Background())
	if err != nil || len(pools) != 1 || pools[0].Health != "ONLINE" {
		t.Fatalf("unexpected result: %#v err=%v", pools, err)
	}
}

func TestPoolCollectorUsesFixedReadOnlyInvocation(t *testing.T) {
	stub := &stubRunner{out: cmdrun.Output{Stdout: []byte("rpool\tONLINE\n")}}
	c := NewPoolCollector(stub)
	if _, err := c.ListPools(context.Background()); err != nil {
		t.Fatal(err)
	}
	if stub.lastPath != ZpoolPath {
		t.Fatalf("path=%q want %q", stub.lastPath, ZpoolPath)
	}
	wantArgs := []string{"list", "-H", "-o", "name,health"}
	if len(stub.lastArgs) != len(wantArgs) {
		t.Fatalf("args=%v want %v", stub.lastArgs, wantArgs)
	}
	for i := range wantArgs {
		if stub.lastArgs[i] != wantArgs[i] {
			t.Fatalf("args=%v want %v", stub.lastArgs, wantArgs)
		}
	}
}

func TestPoolCollectorListPoolsFailures(t *testing.T) {
	cases := []struct {
		name    string
		err     error
		contain string
	}{
		{
			"not available",
			&cmdrun.Error{Kind: cmdrun.NotAvailable, Path: ZpoolPath},
			"not available",
		},
		{
			"exit error",
			&cmdrun.Error{Kind: cmdrun.ExitError, Path: ZpoolPath, ExitCode: 1, Cause: errors.New("exit status 1")},
			"zpool list failed",
		},
		{
			"truncated",
			&cmdrun.Error{Kind: cmdrun.Truncated, Path: ZpoolPath},
			"truncated",
		},
		{
			"timeout",
			&cmdrun.Error{Kind: cmdrun.Timeout, Path: ZpoolPath, Cause: context.DeadlineExceeded},
			"timed out",
		},
	}
	for _, tc := range cases {
		c := NewPoolCollector(&stubRunner{err: tc.err})
		_, err := c.ListPools(context.Background())
		if err == nil || !strings.Contains(err.Error(), tc.contain) {
			t.Fatalf("%s: expected error containing %q, got %v", tc.name, tc.contain, err)
		}
	}
}

func TestPoolCollectorTimeoutClassification(t *testing.T) {
	c := NewPoolCollector(&stubRunner{err: &cmdrun.Error{
		Kind:  cmdrun.Timeout,
		Path:  ZpoolPath,
		Cause: context.DeadlineExceeded,
	}})
	_, err := c.ListPools(context.Background())
	if err == nil || !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("expected timeout error, got %v", err)
	}
}

func TestPoolCollectorMalformedStdout(t *testing.T) {
	c := NewPoolCollector(&stubRunner{out: cmdrun.Output{Stdout: []byte("not-tab-separated\n")}})
	_, err := c.ListPools(context.Background())
	if err == nil || !strings.Contains(err.Error(), "malformed") {
		t.Fatalf("expected malformed error, got %v", err)
	}
}
