package device

import (
	"encoding/json"
	"fmt"
	"strings"
)

// LsblkPath is the fixed lsblk location on Debian/Proxmox.
const LsblkPath = "/usr/bin/lsblk"

// LsblkArgs are read-only machine-readable fields for inventory.
var LsblkArgs = []string{
	"-J", "-b",
	"-o", "NAME,KNAME,PATH,TYPE,SIZE,MODEL,SERIAL,WWN,TRAN,PKNAME",
}

type lsblkNode struct {
	Name     string      `json:"name"`
	KName    string      `json:"kname"`
	Path     string      `json:"path"`
	Type     string      `json:"type"`
	Size     json.Number `json:"size"`
	Model    string      `json:"model"`
	Serial   string      `json:"serial"`
	WWN      string      `json:"wwn"`
	Tran     string      `json:"tran"`
	PKName   string      `json:"pkname"`
	Children []lsblkNode `json:"children"`
}

type lsblkTree struct {
	BlockDevices []lsblkNode `json:"blockdevices"`
}

// ParseLsblkJSON builds an Inventory from lsblk -J output.
func ParseLsblkJSON(data []byte) (*Inventory, error) {
	var tree lsblkTree
	if err := json.Unmarshal(data, &tree); err != nil {
		return nil, fmt.Errorf("lsblk json: %w", err)
	}
	inv := &Inventory{
		Devices:       make(map[string]PhysicalIdentity),
		ByIDIndex:     make(map[string]string),
		PhysicalIndex: make(map[string][]string),
	}
	var walk func(nodes []lsblkNode, parentPath, parentK string)
	walk = func(nodes []lsblkNode, parentPath, parentK string) {
		for _, n := range nodes {
			id := nodeToIdentity(n, parentPath, parentK)
			if id.DevicePath != "" {
				inv.Devices[id.DevicePath] = id
				key := physicalKey(id)
				if key != "" {
					inv.PhysicalIndex[key] = appendUnique(inv.PhysicalIndex[key], id.DevicePath)
				}
			}
			if len(n.Children) > 0 {
				walk(n.Children, id.DevicePath, n.KName)
			}
		}
	}
	walk(tree.BlockDevices, "", "")
	return inv, nil
}

func nodeToIdentity(n lsblkNode, parentPath, parentK string) PhysicalIdentity {
	path := strings.TrimSpace(n.Path)
	if path == "" && n.KName != "" {
		path = "/dev/" + n.KName
	}
	id := PhysicalIdentity{
		KernelName:   n.KName,
		DevicePath:   path,
		ParentKernel: strings.TrimSpace(n.PKName),
		ParentPath:   parentPath,
		Kind:         mapBlockKind(n.Type),
		Transport:    mapTransport(n.Tran, n.KName),
		Model:        strings.TrimSpace(n.Model),
		Serial:       strings.TrimSpace(n.Serial),
		WWN:          strings.TrimSpace(n.WWN),
	}
	if n.Size != "" {
		if v, err := n.Size.Int64(); err == nil && v >= 0 {
			u := uint64(v)
			id.SizeBytes = &u
		}
	}
	if parentK == "" && strings.HasPrefix(n.KName, "nvme") && strings.Contains(n.KName, "n") {
		id.ControllerPath = controllerPathFromNamespace(path)
	}
	if id.ParentPath == "" && id.ParentKernel != "" {
		id.ParentPath = "/dev/" + id.ParentKernel
	}
	return id
}

func controllerPathFromNamespace(path string) string {
	// /dev/nvme0n1 -> /dev/nvme0 ; /dev/nvme0n1p2 -> /dev/nvme0
	base := strings.TrimPrefix(path, "/dev/")
	idx := strings.Index(base, "n")
	if idx <= 0 {
		return ""
	}
	ctrl := base[:idx]
	return "/dev/" + ctrl
}

func mapBlockKind(t string) BlockKind {
	switch strings.ToLower(strings.TrimSpace(t)) {
	case "disk":
		return BlockKindDisk
	case "part":
		return BlockKindPart
	case "nvme":
		return BlockKindNamespace
	case "raid", "mpath", "dm":
		return BlockKindVirtual
	default:
		return BlockKindUnknown
	}
}

func mapTransport(tran, kname string) TransportKind {
	switch strings.ToLower(strings.TrimSpace(tran)) {
	case "nvme":
		return TransportNVMe
	case "sata", "ata":
		return TransportSATA
	case "sas":
		return TransportSAS
	case "virtio":
		return TransportVirtIO
	}
	if strings.HasPrefix(kname, "nvme") {
		return TransportNVMe
	}
	if strings.HasPrefix(kname, "sd") {
		return TransportSATA
	}
	return TransportUnknown
}

func physicalKey(id PhysicalIdentity) string {
	if id.WWN != "" {
		return "wwn:" + strings.ToLower(id.WWN)
	}
	if id.Serial != "" && id.Model != "" {
		return "serial:" + strings.ToLower(id.Serial) + "|" + strings.ToLower(id.Model)
	}
	if id.Serial != "" {
		return "serial:" + strings.ToLower(id.Serial)
	}
	return ""
}

func appendUnique(list []string, v string) []string {
	for _, existing := range list {
		if existing == v {
			return list
		}
	}
	return append(list, v)
}
