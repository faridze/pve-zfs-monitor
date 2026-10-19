package monitor

import (
	"errors"
	"fmt"
)

// MemberCounters stores the last verified ZFS error counters for one member.
type MemberCounters struct {
	Read  *uint64 `json:"read,omitempty"`
	Write *uint64 `json:"write,omitempty"`
	Cksum *uint64 `json:"cksum,omitempty"`
}

// DeviceCounters stores the last verified SMART counters for one physical device.
type DeviceCounters struct {
	MediaErrors         *int64 `json:"media_errors,omitempty"`
	DataIntegrityErrors *int64 `json:"data_integrity_errors,omitempty"`
	ErrorLogEntries     *int64 `json:"error_log_entries,omitempty"`
}

// Baselines holds privacy-safe counter snapshots keyed by monitor Signal.Key.
type Baselines struct {
	Members map[string]MemberCounters `json:"members,omitempty"`
	Devices map[string]DeviceCounters `json:"devices,omitempty"`
}

func validateBaselines(b *Baselines) error {
	if b == nil {
		return nil
	}
	if len(b.Members) > maxSignals || len(b.Devices) > maxSignals {
		return errors.New("too many baseline entries")
	}
	for k, c := range b.Members {
		if !validKey(k) {
			return fmt.Errorf("invalid member baseline key")
		}
		if err := validateMemberCounters(c); err != nil {
			return err
		}
	}
	for k, c := range b.Devices {
		if !validKey(k) {
			return fmt.Errorf("invalid device baseline key")
		}
		if err := validateDeviceCounters(c); err != nil {
			return err
		}
	}
	return nil
}

func validateMemberCounters(c MemberCounters) error {
	for _, v := range []*uint64{c.Read, c.Write, c.Cksum} {
		if v != nil && *v < 0 {
			return errors.New("invalid member counter")
		}
	}
	return nil
}

func validateDeviceCounters(c DeviceCounters) error {
	for _, v := range []*int64{c.MediaErrors, c.DataIntegrityErrors, c.ErrorLogEntries} {
		if v != nil && *v < 0 {
			return errors.New("invalid device counter")
		}
	}
	return nil
}

// counterDelta classifies a monotonic counter change between observations.
type counterDelta int

const (
	counterUnchanged counterDelta = 0
	counterIncreased counterDelta = 1
	counterDecreased counterDelta = 2
	counterReset     counterDelta = 3
	counterMissing   counterDelta = 4
)

func uint64Delta(prev, cur *uint64) counterDelta {
	if cur == nil {
		return counterMissing
	}
	if prev == nil {
		return counterUnchanged // first baseline; not an increase event
	}
	if *cur < *prev {
		if *cur == 0 {
			return counterReset
		}
		return counterDecreased
	}
	if *cur > *prev {
		return counterIncreased
	}
	return counterUnchanged
}

func int64Delta(prev, cur *int64) counterDelta {
	if cur == nil {
		return counterMissing
	}
	if prev == nil {
		return counterUnchanged
	}
	if *cur < *prev {
		if *cur == 0 {
			return counterReset
		}
		return counterDecreased
	}
	if *cur > *prev {
		return counterIncreased
	}
	return counterUnchanged
}

func worstMemberCounterDelta(prev MemberCounters, cur MemberCounters) (counterDelta, bool) {
	worst := counterUnchanged
	anyKnown := false
	for _, d := range []counterDelta{
		uint64Delta(prev.Read, cur.Read),
		uint64Delta(prev.Write, cur.Write),
		uint64Delta(prev.Cksum, cur.Cksum),
	} {
		if d == counterMissing {
			continue
		}
		anyKnown = true
		if d > worst {
			worst = d
		}
	}
	return worst, anyKnown
}
