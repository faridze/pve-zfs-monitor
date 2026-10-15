package device

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ByIDDir is the standard persistent disk symlink directory.
const ByIDDir = "/dev/disk/by-id"

// ReadByIDIndex reads symlink targets under /dev/disk/by-id.
func ReadByIDIndex(dir string) (map[string]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	index := make(map[string]string, len(entries))
	for _, ent := range entries {
		if ent.IsDir() {
			continue
		}
		name := ent.Name()
		if strings.HasPrefix(name, "wwn-") || strings.HasPrefix(name, "nvme-") ||
			strings.HasPrefix(name, "ata-") || strings.HasPrefix(name, "scsi-") {
			full := filepath.Join(dir, name)
			target, err := os.Readlink(full)
			if err != nil {
				continue
			}
			target = filepath.Clean(filepath.Join(dir, target))
			if !strings.HasPrefix(target, "/dev/") {
				target = "/dev/" + strings.TrimPrefix(target, "../")
			}
			index[name] = target
		}
	}
	return index, nil
}

// ApplyByIDLinks attaches by-id symlink names to inventory devices and builds reverse index.
func ApplyByIDLinks(inv *Inventory, byID map[string]string) {
	if inv == nil {
		return
	}
	if inv.ByIDIndex == nil {
		inv.ByIDIndex = make(map[string]string)
	}
	for name, target := range byID {
		if prev, exists := inv.ByIDIndex[name]; exists && prev != target {
			inv.Issues = append(inv.Issues, fmt.Sprintf("conflicting by-id alias %q: %s vs %s", name, prev, target))
		}
		inv.ByIDIndex[name] = target
		attached := false
		if id, ok := inv.Devices[target]; ok {
			id.ByIDLinks = appendUnique(id.ByIDLinks, name)
			if id.StableHardwareID == "" {
				id.StableHardwareID = "by-id:" + name
			}
			inv.Devices[target] = id
			attached = true
		}
		if parent := partitionParentPath(target); parent != "" {
			if pid, ok := inv.Devices[parent]; ok {
				pid.ByIDLinks = appendUnique(pid.ByIDLinks, name)
				inv.Devices[parent] = pid
				attached = true
			}
		}
		if !attached {
			inv.Issues = append(inv.Issues, fmt.Sprintf("by-id %q -> %q not in inventory", name, target))
		}
	}
}

func partitionParentPath(partPath string) string {
	base := strings.TrimPrefix(partPath, "/dev/")
	if strings.Contains(base, "nvme") && strings.Contains(base, "p") {
		i := strings.LastIndex(base, "p")
		if i > 0 {
			return "/dev/" + base[:i]
		}
	}
	for i := len(base) - 1; i > 0; i-- {
		if base[i] >= '0' && base[i] <= '9' {
			continue
		}
		if i < len(base)-1 {
			return "/dev/" + base[:i+1]
		}
		break
	}
	return ""
}

// ParseByIDLines parses fixture lines "by-id-name -> /dev/target".
func ParseByIDLines(lines []string) (map[string]string, error) {
	out := make(map[string]string)
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "->", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("malformed by-id line: %q", line)
		}
		name := strings.TrimSpace(parts[0])
		target := strings.TrimSpace(parts[1])
		out[name] = target
	}
	return out, nil
}
