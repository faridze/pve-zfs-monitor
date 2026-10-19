// Package monitor provides pure, offline alert decisions. It does not poll
// devices, schedule jobs, send messages, or access a state file.
package monitor

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

type Level string

const (
	Healthy  Level = "healthy"
	Warning  Level = "warning"
	Critical Level = "critical"
	Unknown  Level = "unknown"
)

type Kind string

const (
	Alert    Kind = "alert"
	Reminder Kind = "reminder"
	Recovery Kind = "recovery"
)

const (
	stateVersion = 1
	maxSignals   = 512
)

// Key creates a privacy-preserving identifier. Raw pool/disk names, serials,
// device paths and model strings are never stored in events or state.
func Key(scope, identity string) (string, error) {
	switch scope {
	case "host", "pool", "device":
	default:
		return "", fmt.Errorf("unsupported signal scope")
	}
	if strings.TrimSpace(identity) == "" {
		return "", fmt.Errorf("empty signal identity")
	}
	sum := sha256.Sum256([]byte(scope + "\x00" + identity))
	return scope + ":" + hex.EncodeToString(sum[:]), nil
}

// Signal is an already-collected, normalized reading; freshness must be
// checked before a positive health verdict.
type Signal struct {
	Key        string    `json:"key"`
	Level      Level     `json:"level"`
	Verified   bool      `json:"verified"`
	ObservedAt time.Time `json:"observed_at"`
}

// Event is safe for future mock adapters: no raw hardware identifiers.
type Event struct {
	Key   string `json:"key"`
	Kind  Kind   `json:"kind"`
	Level Level  `json:"level"`
}

// Record stores only transition/dedupe metadata, never telemetry.
type Record struct {
	Level         Level `json:"level"`
	LastEmittedAt int64 `json:"last_emitted_at,omitempty"`
	HealthyStreak int   `json:"healthy_streak,omitempty"`
}

type State struct {
	Version     int               `json:"version"`
	LastRunUnix int64             `json:"last_run_unix,omitempty"`
	Records     map[string]Record `json:"records"`
	Baselines   *Baselines        `json:"baselines,omitempty"`
}

type Options struct {
	// A repeated non-healthy signal generates a reminder no more often than
	// this interval. Default 24 hours.
	ReminderInterval time.Duration
	// Healthy samples must be recent, and no future timestamp is trusted.
	// Default 10 minutes.
	MaxAge time.Duration
	// Consecutive fresh healthy samples to confirm a recovery. Default 2.
	RecoverySamples int
}

func normalizeOptions(o Options) (Options, error) {
	if o.ReminderInterval < 0 || o.MaxAge < 0 || o.RecoverySamples < 0 {
		return o, errors.New("negative monitoring option")
	}
	if o.ReminderInterval == 0 {
		o.ReminderInterval = 24 * time.Hour
	}
	if o.MaxAge == 0 {
		o.MaxAge = 10 * time.Minute
	}
	if o.RecoverySamples == 0 {
		o.RecoverySamples = 2
	}
	if o.RecoverySamples > 10 {
		return o, errors.New("recovery confirmation count too large")
	}
	return o, nil
}

func validLevel(l Level) bool {
	return l == Healthy || l == Warning || l == Critical || l == Unknown
}

func validKey(key string) bool {
	i := strings.IndexByte(key, ':')
	if i < 0 || (key[:i] != "host" && key[:i] != "pool" && key[:i] != "device") || len(key[i+1:]) != 64 {
		return false
	}
	for _, c := range key[i+1:] {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}

// Decide returns a new snapshot and alert/recovery transitions with no side
// effects. Nothing is delivered, persisted or sent over the network.
// Missing, stale and unverified readings become UNKNOWN, never healthy.
func Decide(previous State, readings []Signal, now time.Time, opts Options) (State, []Event, error) {
	o, err := normalizeOptions(opts)
	if err != nil {
		return State{}, nil, err
	}
	if now.IsZero() || now.Unix() <= 0 {
		return State{}, nil, errors.New("invalid observation clock")
	}
	if previous.Version == 0 {
		if previous.LastRunUnix != 0 || len(previous.Records) > 0 {
			return State{}, nil, errors.New("invalid previous monitoring state")
		}
	} else if previous.Version != stateVersion {
		return State{}, nil, errors.New("unsupported state schema")
	}
	if previous.LastRunUnix > now.Unix() {
		return State{}, nil, errors.New("observation clock moved backwards")
	}
	if len(readings) > maxSignals || len(previous.Records) > maxSignals {
		return State{}, nil, errors.New("too many signals")
	}

	current := make(map[string]Level, len(readings))
	for _, s := range readings {
		if !validKey(s.Key) || !validLevel(s.Level) {
			return State{}, nil, errors.New("invalid signal key or level")
		}
		if _, ok := current[s.Key]; ok {
			return State{}, nil, errors.New("duplicate signal key")
		}
		level := s.Level
		if !s.Verified || s.ObservedAt.IsZero() || s.ObservedAt.After(now) ||
			now.Sub(s.ObservedAt) > o.MaxAge {
			level = Unknown
		}
		current[s.Key] = level
	}

	for k, r := range previous.Records {
		if !validKey(k) || !validLevel(r.Level) || r.HealthyStreak < 0 ||
			r.HealthyStreak >= o.RecoverySamples || r.LastEmittedAt > now.Unix() {
			return State{}, nil, errors.New("invalid previous monitoring state")
		}
		if _, ok := current[k]; !ok {
			current[k] = Unknown
		}
	}
	if len(current) > maxSignals {
		return State{}, nil, errors.New("too many combined signals")
	}
	if len(current) == 0 {
		// No telemetry is not a healthy baseline.
		k, _ := Key("host", "missing-all-telemetry")
		current[k] = Unknown
	}

	next := State{Version: stateVersion, LastRunUnix: now.Unix(), Records: make(map[string]Record, len(current))}
	keys := make([]string, 0, len(current))
	for k := range current {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var events []Event
	for _, k := range keys {
		level := current[k]
		old, exists := previous.Records[k]
		if level == Healthy {
			if !exists || old.Level == Healthy {
				next.Records[k] = Record{Level: Healthy}
				continue
			}
			old.HealthyStreak++
			if old.HealthyStreak >= o.RecoverySamples {
				next.Records[k] = Record{Level: Healthy}
				events = append(events, Event{Key: k, Kind: Recovery, Level: Healthy})
			} else {
				next.Records[k] = old
			}
			continue
		}

		r := Record{Level: level}
		switch {
		case !exists || old.Level != level:
			r.LastEmittedAt = now.Unix()
			events = append(events, Event{Key: k, Kind: Alert, Level: level})
		case now.Sub(time.Unix(old.LastEmittedAt, 0)) >= o.ReminderInterval:
			r.LastEmittedAt = now.Unix()
			events = append(events, Event{Key: k, Kind: Reminder, Level: level})
		default:
			r.LastEmittedAt = old.LastEmittedAt
		}
		next.Records[k] = r
	}
	return next, events, nil
}

// validatePersistedState rejects on-disk snapshots that are present but not a
// supported schema. It must run after JSON decode and before Decide.
func validatePersistedState(s State) error {
	if s.Version != stateVersion {
		return fmt.Errorf("invalid state version %d", s.Version)
	}
	if s.Records == nil {
		return errors.New("state records missing")
	}
	if s.LastRunUnix < 0 {
		return errors.New("invalid last_run_unix")
	}
	if len(s.Records) > maxSignals {
		return errors.New("too many state records")
	}
	if err := validateBaselines(s.Baselines); err != nil {
		return err
	}
	for k, r := range s.Records {
		if !validKey(k) || !validLevel(r.Level) {
			return errors.New("invalid record in state")
		}
		if r.HealthyStreak < 0 || r.HealthyStreak > 10 {
			return errors.New("invalid healthy streak in state")
		}
		if r.LastEmittedAt < 0 {
			return errors.New("invalid last_emitted_at in state")
		}
	}
	return nil
}
