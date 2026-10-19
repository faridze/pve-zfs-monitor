package monitor

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/faridze/pve-zfs-monitor/internal/zfs"
)

var baseTime = time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)

func signal(scope, name string, level Level, when time.Time) Signal {
	key, err := Key(scope, name)
	if err != nil {
		panic(err)
	}
	return Signal{Key: key, Level: level, Verified: true, ObservedAt: when}
}

func assertEvent(t *testing.T, events []Event, wantKind Kind, wantLevel Level) {
	t.Helper()
	if len(events) != 1 || events[0].Kind != wantKind || events[0].Level != wantLevel {
		t.Fatalf("events = %+v, want %s %s", events, wantKind, wantLevel)
	}
}

func TestTransitionsDedupReminderAndConfirmedRecovery(t *testing.T) {
	k := signal("pool", "private-pool", Healthy, baseTime)
	state, events, err := Decide(State{}, []Signal{k}, baseTime, Options{})
	if err != nil || len(events) != 0 {
		t.Fatalf("first healthy baseline: events=%+v err=%v", events, err)
	}
	now := baseTime.Add(time.Minute)
	k.Level = Warning
	k.ObservedAt = now
	state, events, err = Decide(state, []Signal{k}, now, Options{})
	if err != nil {
		t.Fatal(err)
	}
	assertEvent(t, events, Alert, Warning)
	now = now.Add(time.Minute)
	k.ObservedAt = now
	state, events, err = Decide(state, []Signal{k}, now, Options{})
	if err != nil || len(events) != 0 {
		t.Fatalf("unchanged warning must deduplicate, events=%+v err=%v", events, err)
	}
	now = now.Add(24 * time.Hour)
	k.ObservedAt = now
	state, events, err = Decide(state, []Signal{k}, now, Options{})
	if err != nil {
		t.Fatal(err)
	}
	assertEvent(t, events, Reminder, Warning)
	now = now.Add(time.Minute)
	k.Level = Healthy
	k.ObservedAt = now
	state, events, err = Decide(state, []Signal{k}, now, Options{})
	if err != nil || len(events) != 0 || state.Records[k.Key].HealthyStreak != 1 {
		t.Fatalf("first healthy sample must not recover, events=%+v err=%v", events, err)
	}
	now = now.Add(time.Minute)
	k.ObservedAt = now
	state, events, err = Decide(state, []Signal{k}, now, Options{})
	if err != nil {
		t.Fatal(err)
	}
	assertEvent(t, events, Recovery, Healthy)
	now = now.Add(time.Minute)
	k.ObservedAt = now
	_, events, err = Decide(state, []Signal{k}, now, Options{})
	if err != nil || len(events) != 0 {
		t.Fatalf("no duplicate recovery, events=%+v err=%v", events, err)
	}
}

func TestUnknownNeverSilentlyHealthy(t *testing.T) {
	k := signal("pool", "mirror", Critical, baseTime)
	s, e, err := Decide(State{}, []Signal{k}, baseTime, Options{})
	if err != nil {
		t.Fatal(err)
	}
	assertEvent(t, e, Alert, Critical)

	// Stale health report with ONLINE must be UNKNOWN; it cannot clear failure.
	stale := signal("pool", "mirror", Healthy, baseTime)
	now := baseTime.Add(11 * time.Minute)
	s, e, err = Decide(s, []Signal{stale}, now, Options{})
	if err != nil {
		t.Fatal(err)
	}
	assertEvent(t, e, Alert, Unknown)
	if s.Records[k.Key].Level == Healthy {
		t.Fatal("stale sample incorrectly recovered")
	}

	// One fresh reading after UNKNOWN is insufficient for recovery.
	fresh := signal("pool", "mirror", Healthy, now.Add(time.Minute))
	s, e, err = Decide(s, []Signal{fresh}, now.Add(time.Minute), Options{})
	if err != nil || len(e) != 0 {
		t.Fatalf("unexpected recovery from one reading: %+v %v", e, err)
	}
	fresh.ObservedAt = now.Add(2 * time.Minute)
	_, e, err = Decide(s, []Signal{fresh}, fresh.ObservedAt, Options{})
	if err != nil {
		t.Fatal(err)
	}
	assertEvent(t, e, Recovery, Healthy)
}

func TestUnverifiedAndMissingReadingsBecomeUnknown(t *testing.T) {
	k := signal("pool", "name", Healthy, baseTime)
	state, _, err := Decide(State{}, []Signal{k}, baseTime, Options{})
	if err != nil {
		t.Fatal(err)
	}
	_, events, err := Decide(state, nil, baseTime.Add(time.Minute), Options{})
	if err != nil {
		t.Fatal(err)
	}
	assertEvent(t, events, Alert, Unknown)

	k.Verified = false
	k.Level = Healthy
	_, events, err = Decide(State{}, []Signal{k}, baseTime, Options{})
	if err != nil {
		t.Fatal(err)
	}
	assertEvent(t, events, Alert, Unknown)

	_, events, err = Decide(State{}, nil, baseTime, Options{})
	if err != nil {
		t.Fatal(err)
	}
	assertEvent(t, events, Alert, Unknown)
}

func TestClockRegressionsAndInvalidStateFailClosed(t *testing.T) {
	k := signal("pool", "name", Critical, baseTime)
	s, _, err := Decide(State{}, []Signal{k}, baseTime, Options{})
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name string
		prev State
		sig  []Signal
		now  time.Time
		opt  Options
	}{
		{"time backwards", s, []Signal{k}, baseTime.Add(-time.Minute), Options{}},
		{"schema mismatch", State{Version: 99}, []Signal{k}, baseTime, Options{}},
		{"negative interval", State{}, []Signal{k}, baseTime, Options{ReminderInterval: -time.Second}},
		{"invalid key", State{}, []Signal{{Key: "/dev/nvme0n1", Level: Healthy, Verified: true, ObservedAt: baseTime}}, baseTime, Options{}},
		{"duplicate keys", State{}, []Signal{k, k}, baseTime, Options{}},
		{"corrupted previous", State{Version: 1, Records: map[string]Record{k.Key: {Level: "broken"}}}, []Signal{k}, baseTime, Options{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, _, err := Decide(tc.prev, tc.sig, tc.now, tc.opt)
			if err == nil {
				t.Fatal("invalid input unexpectedly accepted")
			}
		})
	}
}

func TestStateRoundtripAndPrivacy(t *testing.T) {
	serial := "SERIAL-SHOULD-NOT-APPEAR"
	k := signal("device", serial, Critical, baseTime)
	s, events, err := Decide(State{}, []Signal{k}, baseTime, Options{})
	if err != nil {
		t.Fatal(err)
	}
	blob, err := json.Marshal(struct {
		State  State   `json:"state"`
		Events []Event `json:"events"`
	}{s, events})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(blob), serial) || strings.Contains(string(blob), "/dev/") {
		t.Fatal("sensitive identity persisted")
	}
	var copy State
	b, _ := json.Marshal(s)
	if err := json.Unmarshal(b, &copy); err != nil {
		t.Fatal(err)
	}
	next := baseTime.Add(time.Minute)
	k.ObservedAt = next
	_, events, err = Decide(copy, []Signal{k}, next, Options{})
	if err != nil || len(events) != 0 {
		t.Fatalf("dedup across persisted state failed: events=%+v err=%v", events, err)
	}
}

func TestPoolSignalsKnownAndUnknownHealth(t *testing.T) {
	pools := []zfs.Pool{{Name: "zfs-private", Health: "ONLINE"}, {Name: "backup", Health: "DEGRADED"}}
	got, err := PoolSignals(pools, baseTime, true)
	if err != nil || len(got) != 3 || got[0].Level != Healthy || got[1].Level != Healthy || got[2].Level != Warning {
		t.Fatalf("pool normalization: %+v %v", got, err)
	}
	buf, _ := json.Marshal(got)
	if strings.Contains(string(buf), "zfs-private") || strings.Contains(string(buf), "backup") {
		t.Fatal("pool identifiers leaked into signals")
	}
	if _, err := PoolSignals(append(pools, pools[0]), baseTime, true); err == nil {
		t.Fatal("duplicate pool accepted")
	}
	for _, health := range []string{"", "WEIRD", "online"} {
		got, err := PoolSignals([]zfs.Pool{{Name: "p", Health: health}}, baseTime, true)
		if err != nil || got[0].Level != Unknown || got[0].Verified {
			t.Fatalf("unexpected healthy unknown state: %q -> %+v %v", health, got, err)
		}
	}
	got, err = PoolSignals(nil, baseTime, false)
	if err != nil || len(got) != 1 || got[0].Level != Unknown {
		t.Fatalf("collector error must be UNKNOWN: %+v %v", got, err)
	}
}

func TestSimulatedEmailObserviumOutputsAreNotTransports(t *testing.T) {
	// Future output adapters can accept events. This fake captures both paths
	// without sockets, addresses, credentials, mail or network I/O.
	sig := signal("pool", "private", Critical, baseTime)
	_, events, err := Decide(State{}, []Signal{sig}, baseTime, Options{})
	if err != nil {
		t.Fatal(err)
	}
	fakeEmail := append([]Event(nil), events...)
	fakeObservium := append([]Event(nil), events...)
	if len(fakeEmail) != 1 || len(fakeObservium) != 1 || fakeEmail[0].Kind != Alert {
		t.Fatal("fake adapters failed")
	}
}


func TestCollectorOutageRecoveryHasVerifiedHeartbeat(t *testing.T) {
	outage, err := PoolSignals(nil, baseTime, false)
	if err != nil {
		t.Fatal(err)
	}
	state, events, err := Decide(State{}, outage, baseTime, Options{})
	if err != nil {
		t.Fatal(err)
	}
	assertEvent(t, events, Alert, Unknown)

	// The host-scoped signal becomes healthy, but cannot recover from a
	// single sample. The returned pool is a separate healthy baseline.
	polls, err := PoolSignals([]zfs.Pool{{Name: "private", Health: "ONLINE"}}, baseTime.Add(time.Minute), true)
	if err != nil {
		t.Fatal(err)
	}
	state, events, err = Decide(state, polls, baseTime.Add(time.Minute), Options{})
	if err != nil || len(events) != 0 {
		t.Fatalf("unverified recovery: events=%+v err=%v", events, err)
	}
	polls, err = PoolSignals([]zfs.Pool{{Name: "private", Health: "ONLINE"}}, baseTime.Add(2*time.Minute), true)
	if err != nil {
		t.Fatal(err)
	}
	_, events, err = Decide(state, polls, baseTime.Add(2*time.Minute), Options{})
	if err != nil {
		t.Fatal(err)
	}
	assertEvent(t, events, Recovery, Healthy)
}
