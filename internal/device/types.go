// Package device provides read-only physical disk inventory, ZFS correlation and SMART collection.
package device

// TransportKind classifies block device transport where known.
type TransportKind string

const (
	TransportNVMe    TransportKind = "nvme"
	TransportSATA    TransportKind = "sata"
	TransportSAS     TransportKind = "sas"
	TransportVirtIO  TransportKind = "virtio"
	TransportOther   TransportKind = "other"
	TransportUnknown TransportKind = "unknown"
)

// BlockKind distinguishes partitions, whole disks, namespaces, and virtual nodes.
type BlockKind string

const (
	BlockKindDisk       BlockKind = "disk"
	BlockKindPart       BlockKind = "part"
	BlockKindNamespace  BlockKind = "namespace"
	BlockKindController BlockKind = "controller"
	BlockKindVirtual    BlockKind = "virtual"
	BlockKindUnknown    BlockKind = "unknown"
)

// MappingOutcome describes ZFS-member to physical device correlation.
type MappingOutcome string

const (
	MappingMapped      MappingOutcome = "mapped"
	MappingAmbiguous   MappingOutcome = "ambiguous"
	MappingMissing     MappingOutcome = "missing"
	MappingUnsupported MappingOutcome = "unsupported"
	MappingUnknown     MappingOutcome = "unknown"
)

// HealthCollectionStatus describes SMART/NVMe data collection state.
type HealthCollectionStatus string

const (
	HealthCollected        HealthCollectionStatus = "collected"
	HealthUnsupported      HealthCollectionStatus = "unsupported"
	HealthPermissionDenied HealthCollectionStatus = "permission_denied"
	HealthFailed           HealthCollectionStatus = "failed"
	HealthMissingTool      HealthCollectionStatus = "missing_tool"
	HealthTimeout          HealthCollectionStatus = "timeout"
	HealthNotAttempted     HealthCollectionStatus = "not_attempted"
)

// PhysicalIdentity is a normalized view of one block device from host evidence.
type PhysicalIdentity struct {
	KernelName     string        `json:"kernel_name,omitempty"`
	DevicePath     string        `json:"device_path,omitempty"`
	ParentKernel   string        `json:"parent_kernel,omitempty"`
	ParentPath     string        `json:"parent_path,omitempty"`
	ControllerPath string        `json:"controller_path,omitempty"`
	Kind           BlockKind     `json:"kind"`
	Transport      TransportKind `json:"transport"`
	Model          string        `json:"model,omitempty"`
	Serial         string        `json:"serial,omitempty"`
	WWN            string        `json:"wwn,omitempty"`
	NVMeCtrlID     string        `json:"nvme_controller_id,omitempty"`
	ByIDLinks      []string      `json:"by_id_links,omitempty"`
	SizeBytes      *uint64       `json:"size_bytes,omitempty"`
}

// Inventory holds discovered block devices keyed by canonical device path.
type Inventory struct {
	Devices       map[string]PhysicalIdentity `json:"devices"`
	ByIDIndex     map[string]string           `json:"-"` // by-id name -> device path
	PhysicalIndex map[string][]string         `json:"-"` // stable physical key -> paths
	Issues        []string                    `json:"issues,omitempty"`
}

// MappingEvidence records why a correlation was chosen.
type MappingEvidence struct {
	Source   string `json:"source"`
	Detail   string `json:"detail,omitempty"`
	Strength string `json:"strength,omitempty"`
}

// MemberMapping links one ZFS topology leaf to physical identity.
type MemberMapping struct {
	PoolName       string            `json:"pool_name"`
	ZFSMemberName  string            `json:"zfs_member_name"`
	ZFSState       string            `json:"zfs_state,omitempty"`
	ZFSStateDetail string            `json:"zfs_state_detail,omitempty"`
	Outcome        MappingOutcome    `json:"outcome"`
	Reason         string            `json:"reason,omitempty"`
	DevicePath     string            `json:"device_path,omitempty"`
	Physical       *PhysicalIdentity `json:"physical,omitempty"`
	Evidence       []MappingEvidence `json:"evidence,omitempty"`
}

// SmartNumeric preserves raw and normalized SMART values without inventing units.
type SmartNumeric struct {
	Value      *int64 `json:"value,omitempty"`
	Raw        string `json:"raw,omitempty"`
	Unit       string `json:"unit,omitempty"`
	Normalized string `json:"normalized,omitempty"`
}

// DeviceHealth holds normalized SMART / NVMe health for one physical device.
type DeviceHealth struct {
	DevicePath          string                 `json:"device_path"`
	CollectionStatus    HealthCollectionStatus `json:"collection_status"`
	CollectionDetail    string                 `json:"collection_detail,omitempty"`
	SmartPassed         *bool                  `json:"smart_passed,omitempty"`
	SmartStatusString   string                 `json:"smart_status_string,omitempty"`
	CriticalWarning     *uint8                 `json:"critical_warning,omitempty"`
	Temperature         *SmartNumeric          `json:"temperature,omitempty"`
	PercentageUsed      *SmartNumeric          `json:"percentage_used,omitempty"`
	AvailableSpare      *SmartNumeric          `json:"available_spare,omitempty"`
	SpareThreshold      *SmartNumeric          `json:"spare_threshold,omitempty"`
	MediaErrors         *SmartNumeric          `json:"media_errors,omitempty"`
	DataIntegrityErrors *SmartNumeric          `json:"data_integrity_errors,omitempty"`
	ErrorLogEntries     *SmartNumeric          `json:"error_log_entries,omitempty"`
	PowerOnHours        *SmartNumeric          `json:"power_on_hours,omitempty"`
	UnsafeShutdowns     *SmartNumeric          `json:"unsafe_shutdowns,omitempty"`
	PowerCycles         *SmartNumeric          `json:"power_cycles,omitempty"`
	ReadLifetime        *SmartNumeric          `json:"read_lifetime,omitempty"`
	WriteLifetime       *SmartNumeric          `json:"write_lifetime,omitempty"`
	RawJSON             string                 `json:"raw_json,omitempty"`
	ToolExitCode        *int                   `json:"tool_exit_code,omitempty"`
}

// DeviceReport is the top-level devices diagnostic payload.
type DeviceReport struct {
	CollectedAt     string             `json:"collected_at"`
	TopologySource  string             `json:"topology_source,omitempty"`
	InventoryIssues []string           `json:"inventory_issues,omitempty"`
	Inventory       []PhysicalIdentity `json:"inventory,omitempty"`
	MemberMappings  []MemberMapping    `json:"member_mappings"`
	DeviceHealth    []DeviceHealth     `json:"device_health,omitempty"`
	ReportIssues    []string           `json:"report_issues,omitempty"`
}
