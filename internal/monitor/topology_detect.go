package monitor

import (
	"sort"
	"time"

	"github.com/faridze/pve-zfs-monitor/internal/device"
	"github.com/faridze/pve-zfs-monitor/internal/zfs"
)

type topologyMemberEval struct {
	poolName   string
	memberName string
	leaf       zfs.VdevNode
	sig        Signal
	counters   MemberCounters
	update     bool
	clear      bool
	hw         string
}

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

	var evals []topologyMemberEval
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
			evals = append(evals, topologyMemberEval{
				poolName: pool.Name, memberName: leaf.Name, leaf: leaf,
				sig: sig, counters: counters, update: update, clear: clear, hw: hw,
			})
		})
	}

	applyHardwareCollisions(evals, at)

	for _, e := range evals {
		if _, exists := byKey[e.sig.Key]; !exists {
			order = append(order, e.sig.Key)
		}
		byKey[e.sig.Key] = e.sig
		if e.update {
			updates[e.sig.Key] = e.counters
		}
		if e.clear {
			clearKeys = append(clearKeys, e.sig.Key)
		}
	}

	signals := make([]Signal, 0, len(order))
	for _, k := range order {
		signals = append(signals, byKey[k])
	}
	return signals, updates, clearKeys, nil
}

func applyHardwareCollisions(evals []topologyMemberEval, at time.Time) {
	hwToIdxs := make(map[string][]int)
	for i, e := range evals {
		if e.hw == "" {
			continue
		}
		hwToIdxs[e.hw] = append(hwToIdxs[e.hw], i)
	}
	for _, idxs := range hwToIdxs {
		if len(idxs) < 2 {
			continue
		}
		sort.Slice(idxs, func(a, b int) bool {
			ea, eb := evals[idxs[a]], evals[idxs[b]]
			if ea.poolName != eb.poolName {
				return ea.poolName < eb.poolName
			}
			return ea.memberName < eb.memberName
		})
		for _, i := range idxs {
			e := &evals[i]
			logicalID := logicalMemberIdentity(e.poolName, e.memberName)
			logKey, _ := Key("device", logicalID)
			if zfsVerifiedCriticalMember(e.leaf) {
				sev, _ := zfsStateSeverity(e.leaf)
				if sev != Critical && sev != Warning {
					sev = Critical
				}
				e.sig = Signal{Key: logKey, Level: sev, Verified: e.leaf.StateObserved, ObservedAt: at}
			} else if e.sig.Level == Healthy {
				e.sig = Signal{Key: logKey, Level: Unknown, Verified: false, ObservedAt: at}
			} else {
				e.sig = Signal{Key: logKey, Level: e.sig.Level, Verified: false, ObservedAt: at}
			}
			e.update = false
			e.clear = true
		}
	}
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

	zfsCritical := zfsVerifiedCriticalMember(leaf) && level == Critical && leaf.StateObserved

	cur := MemberCounters{Read: leaf.ReadErrors, Write: leaf.WriteErrors, Cksum: leaf.CksumErrors}
	clearBaseline := false
	if !countersComplete(cur) {
		if level == Healthy {
			level = Unknown
		}
		if !zfsCritical {
			verified = false
		}
	} else if prev != nil {
		old := prev.Members[key]
		deltas := []counterDelta{
			uint64Delta(old.Read, cur.Read),
			uint64Delta(old.Write, cur.Write),
			uint64Delta(old.Cksum, cur.Cksum),
		}
		if memberCounterDeltasConflict(deltas) {
			if !zfsCritical {
				level = Unknown
				verified = false
				clearBaseline = true
			}
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
				if !zfsCritical {
					level = Unknown
					verified = false
					clearBaseline = true
				}
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
