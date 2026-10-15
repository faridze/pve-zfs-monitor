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
	out cmdrun.Output
	err error
}

func (s stubRunner) Run(context.Context, string, []string) (cmdrun.Output, error) {
	return s.out, s.err
}

func TestPoolCollectorListPoolsSuccess(t *testing.T) {
	c := &PoolCollector{
		Runner:    stubRunner{out: cmdrun.Output{Stdout: []byte("rpool\tONLINE\n")}},
		ZpoolPath: ZpoolPath,
		ListArgs:  defaultListArgs,
	}
	pools, err := c.ListPools(context.Background())
	if err != nil || len(pools) != 1 || pools[0].Health != "ONLINE" {
		t.Fatalf("unexpected result: %#v err=%v", pools, err)
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
		c := &PoolCollector{
			Runner:    stubRunner{err: tc.err},
			ZpoolPath: ZpoolPath,
			ListArgs:  defaultListArgs,
		}
		_, err := c.ListPools(context.Background())
		if err == nil || !strings.Contains(err.Error(), tc.contain) {
			t.Fatalf("%s: expected error containing %q, got %v", tc.name, tc.contain, err)
		}
	}
}

func TestPoolCollectorMalformedStdout(t *testing.T) {
	c := &PoolCollector{
		Runner:    stubRunner{out: cmdrun.Output{Stdout: []byte("not-tab-separated\n")}},
		ZpoolPath: ZpoolPath,
		ListArgs:  defaultListArgs,
	}
	_, err := c.ListPools(context.Background())
	if err == nil || !strings.Contains(err.Error(), "malformed") {
		t.Fatalf("expected malformed error, got %v", err)
	}
}
