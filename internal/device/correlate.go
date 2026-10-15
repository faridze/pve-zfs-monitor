package device

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/faridze/pve-zfs-monitor/internal/zfs"
)

// CorrelateTopology maps ZFS leaf members to inventory devices.
func CorrelateTopology(topo *zfs.TopologyReport, inv *Inventory) []MemberMapping {
	if topo == nil {
		return nil
	}
	if inv == nil {
		inv = &Inventory{Devices: make(map[string]PhysicalIdentity), ByIDIndex: make(map[string]string), PhysicalIndex: make(map[string][]string)}
	}
	var out []MemberMapping
	for _, pool := range topo.Pools {
		if pool.Root == nil {
			continue
		}
		walkVdevLeaves(pool.Name, pool.Root, func(leaf zfs.VdevNode) {
			out = append(out, correlateLeaf(pool.Name, leaf, inv))
		})
	}
	return out
}

func walkVdevLeaves(poolName string, node *zfs.VdevNode, fn func(zfs.VdevNode)) {
	if node == nil {
		return
	}
	if isDataLeaf(node) {
		fn(*node)
		return
	}
	for i := range node.Children {
		walkVdevLeaves(poolName, &node.Children[i], fn)
	}
}

func isDataLeaf(n *zfs.VdevNode) bool {
	if n == nil || len(n.Children) > 0 {
		return false
	}
	switch n.VdevClass {
	case "section", "spare", "logs", "cache", "special", "pool", "mirror", "raidz", "stripe", "unknown":
		return false
	default:
		return true
	}
}

func correlateLeaf(pool string, leaf zfs.VdevNode, inv *Inventory) MemberMapping {
	m := MemberMapping{
		PoolName:       pool,
		ZFSMemberName:  leaf.Name,
		ZFSState:       leaf.State,
		ZFSStateDetail: leaf.StateDetail,
		Outcome:        MappingUnknown,
	}
	if leaf.State == "UNAVAIL" && IsGUIDMember(leaf.Name) {
		m.Outcome = MappingMissing
		m.Reason = "GUID member unavailable; physical device not inferred"
		return m
	}
	if leaf.State == "UNAVAIL" && strings.Contains(leaf.StateDetail, "was ") {
		m.Outcome = MappingMissing
		m.Reason = "member unavailable; former device path not used for identity"
		return m
	}
	candidates := memberPathCandidates(leaf)
	if len(candidates) == 0 {
		m.Outcome = MappingUnsupported
		m.Reason = "no resolvable device path in member name"
		return m
	}
	var matches []string
	var evidence []MappingEvidence
	for _, c := range candidates {
		path, ev, ok := resolveCandidate(c, inv)
		if !ok {
			continue
		}
		evidence = append(evidence, ev...)
		matches = appendUnique(matches, path)
	}
	m.Evidence = evidence
	switch len(matches) {
	case 0:
		m.Outcome = MappingMissing
		m.Reason = "no inventory match for member paths"
	case 1:
		m.Outcome = MappingMapped
		m.DevicePath = matches[0]
		if phy, ok := inv.Devices[matches[0]]; ok {
			m.Physical = &phy
		}
		// Prefer parent disk identity for partitions when correlating health.
		if m.Physical != nil && m.Physical.Kind == BlockKindPart && m.Physical.ParentPath != "" {
			if parent, ok := inv.Devices[m.Physical.ParentPath]; ok {
				dup := parent
				dup.DevicePath = parent.DevicePath
				m.Physical = &dup
				m.DevicePath = parent.DevicePath
			}
		}
	default:
		if samePhysical(matches, inv) {
			m.Outcome = MappingMapped
			m.DevicePath = matches[0]
			if phy, ok := inv.Devices[matches[0]]; ok {
				m.Physical = &phy
			}
			m.Evidence = append(m.Evidence, MappingEvidence{
				Source: "dedupe", Detail: "multiple aliases same physical key", Strength: "high",
			})
		} else {
			m.Outcome = MappingAmbiguous
			m.Reason = fmt.Sprintf("conflicting device paths: %s", strings.Join(matches, ", "))
		}
	}
	return m
}

func memberPathCandidates(leaf zfs.VdevNode) []string {
	var out []string
	if p := strings.TrimSpace(leaf.DevicePath); p != "" {
		out = appendUnique(out, normalizeMemberPath(p))
	}
	name := strings.TrimSpace(leaf.Name)
	if name == "" {
		return out
	}
	if strings.HasPrefix(name, "/dev/") {
		p := normalizeMemberPath(name)
		out = appendUnique(out, p)
		if byID, ok := ByIDLinkName(p); ok {
			out = appendUnique(out, "by-id:"+byID)
		}
	} else if isKernelBlockName(name) {
		out = appendUnique(out, "/dev/"+name)
	}
	if strings.HasPrefix(name, "nvme-") || strings.HasPrefix(name, "wwn-") || strings.HasPrefix(name, "ata-") || strings.HasPrefix(name, "scsi-") {
		out = appendUnique(out, "by-id:"+name)
	}
	return out
}

func normalizeMemberPath(p string) string {
	p = filepath.Clean(p)
	if strings.HasPrefix(p, "/dev/") {
		return p
	}
	return "/dev/" + strings.TrimPrefix(p, "/dev/")
}

func resolveCandidate(c string, inv *Inventory) (string, []MappingEvidence, bool) {
	if strings.HasPrefix(c, "by-id:") {
		name := strings.TrimPrefix(c, "by-id:")
		return resolveByIDName(name, inv)
	}
	if byID, ok := ByIDLinkName(c); ok {
		return resolveByIDName(byID, inv)
	}
	return resolveToInventoryPath(c, inv, MappingEvidence{Source: "member_path", Detail: c, Strength: "medium"})
}

func resolveByIDName(name string, inv *Inventory) (string, []MappingEvidence, bool) {
	target, ok := inv.ByIDIndex[name]
	if !ok {
		return "", nil, false
	}
	return resolveToInventoryPath(target, inv, MappingEvidence{Source: "by-id", Detail: name, Strength: "high"})
}

func resolveToInventoryPath(path string, inv *Inventory, ev MappingEvidence) (string, []MappingEvidence, bool) {
	if err := ValidateDevicePath(path); err != nil {
		return "", nil, false
	}
	if _, ok := inv.Devices[path]; ok {
		return path, []MappingEvidence{ev}, true
	}
	parent := partitionParentPath(path)
	if parent != "" {
		if _, ok := inv.Devices[parent]; ok {
			return parent, []MappingEvidence{ev, {Source: "partition_parent", Detail: parent, Strength: "high"}}, true
		}
	}
	return "", nil, false
}

func samePhysical(paths []string, inv *Inventory) bool {
	if len(paths) < 2 {
		return true
	}
	keys := make(map[string]bool)
	for _, p := range paths {
		id, ok := inv.Devices[p]
		if !ok {
			return false
		}
		k := physicalKey(id)
		if k == "" {
			return false
		}
		keys[k] = true
	}
	return len(keys) == 1
}
