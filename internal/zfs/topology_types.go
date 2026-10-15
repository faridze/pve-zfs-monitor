package zfs

const (
	ListHealthVerifiedConsistent   = "verified_consistent"
	ListHealthVerifiedInconsistent = "verified_inconsistent"
	ListHealthUnverified           = "unverified"

	ReportListComparisonUnverified   = "list_comparison_unverified"
	ReportListComparisonIncomplete   = "list_comparison_incomplete"
	ReportListComparisonInconsistent = "list_comparison_inconsistent"
	ReportListComparisonComplete     = "list_comparison_complete"
)

// TopologyReport is the normalized output of zpool status parsing.
type TopologyReport struct {
	Pools                []PoolTopology `json:"pools"`
	ReportIssues         []string       `json:"report_issues,omitempty"`
	ListComparisonStatus string         `json:"list_comparison_status,omitempty"`
}

// PoolTopology describes one pool from zpool status.
type PoolTopology struct {
	Name             string    `json:"name"`
	State            string    `json:"state"`
	StatusText       []string  `json:"status_text,omitempty"`
	ActionText       []string  `json:"action_text,omitempty"`
	SeeText          []string  `json:"see_text,omitempty"`
	Scan             *ScanInfo `json:"scan,omitempty"`
	Root             *VdevNode `json:"root_vdev,omitempty"`
	ErrorsLine       string    `json:"errors_line,omitempty"`
	ListHealth       string    `json:"list_health,omitempty"`
	ListHealthStatus string    `json:"list_health_status,omitempty"`
	ParseIssues      []string  `json:"parse_issues,omitempty"`
}

// ScanInfo captures scrub/resilver scan metadata from the scan/scrub line.
type ScanInfo struct {
	Raw       string  `json:"raw"`
	Requested bool    `json:"requested"`
	Phase     string  `json:"phase"`     // none, in_progress, completed, cancelled, unknown
	Operation string  `json:"operation"` // scrub, resilver, unknown, none
	Errors    *uint64 `json:"errors,omitempty"`
}

// VdevNode is one row in the zpool status config tree.
type VdevNode struct {
	Name          string     `json:"name"`
	VdevClass     string     `json:"vdev_class"` // pool, mirror, raidz, stripe, disk, spare, special, logs, cache, section, unknown
	State         string     `json:"state,omitempty"`
	StateObserved bool       `json:"state_observed"`
	StateDetail   string     `json:"state_detail,omitempty"`
	ReadErrors    *uint64    `json:"read_errors"`
	WriteErrors   *uint64    `json:"write_errors"`
	CksumErrors   *uint64    `json:"checksum_errors"`
	DevicePath    string     `json:"device_path,omitempty"`
	Children      []VdevNode `json:"children,omitempty"`
}
