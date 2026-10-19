package monitor

import (
	"time"

	"github.com/faridze/pve-zfs-monitor/internal/device"
	"github.com/faridze/pve-zfs-monitor/internal/zfs"
)

func topologySignals(topo *zfs.TopologyReport, ok bool, report *device.DeviceReport, prev *Baselines, at time.Time) ([]Signal, map[string]MemberCounters, []string, error) {
	hostKey, _ := Key("host", "zpool-topology")
	if !ok || topo == nil || len(topo.Pools) == 0 {
		return []Signal{{Key: hostKey, Level: Unknown, Verified: false, ObservedAt: at}}, nil, nil, nil
	}
	hostLevel := Healthy
	hostVerified := true
	if !topologyCollectionVerified(topo) {
		hostLevel = Unknown
		hostVerified = false
	}
	byKey := map[string]Signal{hostKey: {Key: hostKey, Level: hostLevel, Verified: hostVerified, ObservedAt: at}}
	order := []string{hostKey}
	mappingByMember := indexMappings(report)
	updates := make(map[string]MemberCounters)
	var clearKeys []string
	hwToKeys := make(map[string][]string)

	for _, pool := range topo.Pools {
		if pool.Root == nil {
			poolKey, _ := Key("pool", pool.Name)
			if _, exists := byKey[poolKey]; !exists {
				order = append(order, poolKey)
			}
			byKey[poolKey] = Signal{Key: poolKey, Level: Unknown, Verified: false, ObservedAt: at}
			continue
		}
		walkLeaves(pool.Name, pool.Root, func(leaf zfs.VdevNode) {
			sig, counters, update, clear, hw := evalMemberLeaf(pool.Name, leaf, mappingByMember, prev, at)
			if _, exists := byKey[sig.Key]; !exists {
				order = append(order, sig.Key)
			}
			byKey[sig.Key] = sig
			if hw != "" {
				hwToKeys[hw] = append(hwToKeys[hw], sig.Key)
			}
			if update {
				updates[sig.Key] = counters
			}
			if clear {
				clearKeys = append(clearKeys, sig.Key)
			}
		})
	}

	for _, keys := range hwToKeys {
		if len(keys) < 2 {
			continue
		}
		seen := make(map[string]bool)
		for _, k := range keys {
			if seen[k] {
				continue
			}
			seen[k] = true
			byKey[k] = Signal{Key: k, Level: Unknown, Verified: false, ObservedAt: at}
			clearKeys = append(clearKeys, k)
			delete(updates, k)
		}
	}

	signals := make([]Signal, 0, len(order))
	for _, k := range order {
		signals = append(signals, byKey[k])
	}
	return signals, updates, clearKeys, nil
}

func indexMappings(report *device.DeviceReport) map[string]*device.MemberMapping {
	out := make(map[string]*device.MemberMapping)
	if report == nil {
		return out
	}
	for i := range report.MemberMappings {
		m := &report.MemberMappings[i]
		out[m.PoolName+"\x00"+m.ZFSMemberName] = m
	}
	return out
}

func walkLeaves(pool string, node *zfs.VdevNode, fn func(zfs.VdevNode)) {
	if node == nil {
		return
	}
	if isTopologyLeaf(node) {
		fn(*node)
		return
	}
	for i := range node.Children {
		walkLeaves(pool, &node.Children[i], fn)
	}
}

func isTopologyLeaf(n *zfs.VdevNode) bool {
	if n == nil || len(n.Children) > 0 {
		return false
	}
	switch n.VdevClass {
	case "section", "spare", "logs", "cache", "special", "pool", "mirror", "raidz", "stripe":
		return false
	default:
		return true
	}
}

func evalMemberLeaf(pool string, leaf zfs.VdevNode, mappings map[string]*device.MemberMapping, prev *Baselines, at time.Time) (Signal, MemberCounters, bool, bool, string) {
	mapKey := pool + "\x00" + leaf.Name
	mapping := mappings[mapKey]
	id := memberIdentity(pool, leaf.Name, mapping)
	hw := ""
	if mapping != nil && mapping.Physical != nil && mapping.Physical.StableHardwareID != "" {
		hw = "hw:" + mapping.Physical.StableHardwareID
	}
	key, err := Key("device", id)
	if err != nil {
		key, _ = Key("device", "invalid-member")
	}
	verified := leaf.StateObserved
	level := zfsMemberLevel(leaf.State, verified)

	if mapping != nil {
		switch mapping.Outcome {
		case device.MappingAmbiguous:
			if sev, ok := zfsStateSeverity(leaf); ok && (sev == Critical || sev == Warning) {
				return Signal{Key: key, Level: sev, Verified: leaf.StateObserved, ObservedAt: at}, MemberCounters{}, false, false, hw
			}
			return Signal{Key: key, Level: Unknown, Verified: false, ObservedAt: at}, MemberCounters{}, false, false, hw
		case device.MappingMissing, device.MappingUnsupported:
			if sev, ok := zfsStateSeverity(leaf); ok && (sev == Critical || sev == Warning) {
				level = sev
				verified = leaf.StateObserved
			} else if level == Healthy {
				level = Unknown
				verified = false
			} else {
				verified = false
			}
		}
	}

	cur := MemberCounters{Read: leaf.ReadErrors, Write: leaf.WriteErrors, Cksum: leaf.CksumErrors}
	clearBaseline := false
	if !countersComplete(cur) {
		if level == Healthy {
			level = Unknown
		}
		verified = false
	} else if prev != nil {
		old := prev.Members[key]
		deltas := []counterDelta{
			uint64Delta(old.Read, cur.Read),
			uint64Delta(old.Write, cur.Write),
			uint64Delta(old.Cksum, cur.Cksum),
		}
		if memberCounterDeltasConflict(deltas) {
			level = Unknown
			verified = false
			clearBaseline = true
		} else {
			delta, _ := worstMemberCounterDelta(old, cur)
			switch delta {
			case counterIncreased:
				if level == Healthy {
					level = Warning
				}
				if leaf.CksumErrors != nil && old.Cksum != nil && *leaf.CksumErrors > *old.Cksum {
					level = Critical
				}
			case counterDecreased, counterReset:
				level = Unknown
				verified = false
				clearBaseline = true
			case counterMissing:
				if level == Healthy {
					level = Unknown
					verified = false
				}
			case counterUnchanged:
			}
		}
	}

	updateBaseline := countersComplete(cur) && verified
	if mapping != nil && mapping.Outcome != device.MappingMapped && mapping.Outcome != device.MappingUnknown {
		if mapping.Outcome == device.MappingMissing || mapping.Outcome == device.MappingUnsupported {
			if level != Critical && level != Warning {
				updateBaseline = false
			}
		} else {
			updateBaseline = false
		}
	}
	return Signal{Key: key, Level: level, Verified: verified, ObservedAt: at}, cur, updateBaseline, clearBaseline, hw
}

func countersComplete(c MemberCounters) bool {
	return c.Read != nil && c.Write != nil && c.Cksum != nil
}
