package monitor

import (
	"time"

	"github.com/faridze/pve-zfs-monitor/internal/device"
	"github.com/faridze/pve-zfs-monitor/internal/zfs"
)

func topologySignals(topo *zfs.TopologyReport, ok bool, report *device.DeviceReport, prev *Baselines, at time.Time) ([]Signal, map[string]MemberCounters, error) {
	hostKey, _ := Key("host", "zpool-topology")
	if !ok || topo == nil || len(topo.Pools) == 0 {
		return []Signal{{Key: hostKey, Level: Unknown, Verified: false, ObservedAt: at}}, nil, nil
	}
	signals := []Signal{{Key: hostKey, Level: Healthy, Verified: true, ObservedAt: at}}
	mappingByMember := indexMappings(report)
	updates := make(map[string]MemberCounters)
	for _, pool := range topo.Pools {
		if pool.Root == nil {
			continue
		}
		walkLeaves(pool.Name, pool.Root, func(leaf zfs.VdevNode) {
			sig, counters, update := evalMemberLeaf(pool.Name, leaf, mappingByMember, prev, at)
			signals = append(signals, sig)
			if update {
				updates[sig.Key] = counters
			}
		})
	}
	return signals, updates, nil
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

func evalMemberLeaf(pool string, leaf zfs.VdevNode, mappings map[string]*device.MemberMapping, prev *Baselines, at time.Time) (Signal, MemberCounters, bool) {
	mapKey := pool + "\x00" + leaf.Name
	mapping := mappings[mapKey]
	id := memberIdentity(pool, leaf.Name, mapping)
	key, err := Key("device", id)
	if err != nil {
		key, _ = Key("device", "invalid-member")
	}
	verified := leaf.StateObserved
	level := zfsMemberLevel(leaf.State, verified)

	if mapping != nil {
		switch mapping.Outcome {
		case device.MappingAmbiguous:
			return Signal{Key: key, Level: Unknown, Verified: false, ObservedAt: at}, MemberCounters{}, false
		case device.MappingMissing, device.MappingUnsupported:
			if level == Healthy {
				level = Unknown
			}
			verified = false
		}
	}

	cur := MemberCounters{Read: leaf.ReadErrors, Write: leaf.WriteErrors, Cksum: leaf.CksumErrors}
	if !countersComplete(cur) {
		if level == Healthy {
			level = Unknown
		}
		verified = false
	} else if prev != nil {
		old := prev.Members[key]
		delta, known := worstMemberCounterDelta(old, cur)
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
		case counterMissing:
			if level == Healthy {
				level = Unknown
				verified = false
			}
		case counterUnchanged:
			// historical nonzero counters without increase do not imply new fault
		}
		_ = known
	}

	updateBaseline := countersComplete(cur) && verified
	if mapping != nil && mapping.Outcome != device.MappingMapped && mapping.Outcome != device.MappingUnknown {
		updateBaseline = false
	}
	return Signal{Key: key, Level: level, Verified: verified, ObservedAt: at}, cur, updateBaseline
}

func countersComplete(c MemberCounters) bool {
	return c.Read != nil && c.Write != nil && c.Cksum != nil
}
