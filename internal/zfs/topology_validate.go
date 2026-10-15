package zfs

import (
	"fmt"
	"strings"
)

func validateTopologyReport(report *TopologyReport) error {
	seen := make(map[string]bool, len(report.Pools))
	for _, pool := range report.Pools {
		if seen[pool.Name] {
			return fmt.Errorf("duplicate pool block %q", pool.Name)
		}
		seen[pool.Name] = true
		if err := validatePoolStructure(pool.Name, pool.Root); err != nil {
			return fmt.Errorf("pool %q: %w", pool.Name, err)
		}
	}
	return nil
}

func validatePoolStructure(poolName string, root *VdevNode) error {
	if root == nil {
		return fmt.Errorf("missing root vdev")
	}
	if root.Name != poolName {
		return fmt.Errorf("root vdev %q does not match pool name %q", root.Name, poolName)
	}
	return validateVdevTree(root)
}

func validateVdevTree(node *VdevNode) error {
	if node.VdevClass == "mirror" && len(node.Children) == 0 {
		return fmt.Errorf("mirror %q has no members", node.Name)
	}
	if err := validateMemberCounters(node); err != nil {
		return err
	}
	for i := range node.Children {
		if err := validateVdevTree(&node.Children[i]); err != nil {
			return err
		}
	}
	return nil
}

func validateMemberCounters(node *VdevNode) error {
	if !node.StateObserved {
		return nil
	}
	if node.State == "AVAIL" || node.State == "INUSE" {
		return nil
	}
	if node.VdevClass == "section" {
		return nil
	}
	if node.ReadErrors == nil || node.WriteErrors == nil || node.CksumErrors == nil {
		return fmt.Errorf("incomplete error counters for vdev %q", node.Name)
	}
	return nil
}

func countMirrorMembers(node *VdevNode) int {
	if node == nil {
		return 0
	}
	if node.VdevClass == "mirror" {
		return len(node.Children)
	}
	total := 0
	for i := range node.Children {
		total += countMirrorMembers(&node.Children[i])
	}
	return total
}

func isAuxSectionHeader(trim string) (string, bool) {
	switch strings.ToLower(trim) {
	case "logs", "cache", "spares", "special":
		return strings.ToLower(trim), true
	default:
		return "", false
	}
}

func auxSectionClass(name string) string {
	switch name {
	case "logs":
		return "logs"
	case "cache":
		return "cache"
	case "spares":
		return "spare"
	case "special":
		return "special"
	default:
		return "section"
	}
}
