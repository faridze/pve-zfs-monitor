package zfs

import "testing"

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
