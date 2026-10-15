package device

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// SmartctlPath is the fixed smartmontools binary on Proxmox.
const SmartctlPath = "/usr/sbin/smartctl"

// SmartctlHealthArgs requests JSON health/all attributes read-only.
var SmartctlHealthArgs = func(device string) []string {
	return []string{"--json=o", "-a", "-d", "auto", device}
}

// NVMeDataUnitBytes is the size of one NVMe data unit per NVMe base spec (for lifetime counters).
const NVMeDataUnitBytes = 512000

type smartctlDoc struct {
	SmartStatus                   json.RawMessage `json:"smart_status"`
	Temperature                   json.RawMessage `json:"temperature"`
	ATAAttributes                 json.RawMessage `json:"ata_smart_attributes"`
	NVMeSmartHealthInformationLog json.RawMessage `json:"nvme_smart_health_information_log"`
}

type smartStatusDoc struct {
	Passed *bool `json:"passed"`
	Status *struct {
		String *string `json:"string"`
	} `json:"status"`
}

type temperatureDoc struct {
	Current *int `json:"current"`
}

type ataTableDoc struct {
	Table []struct {
		Name  string `json:"name"`
		Value int    `json:"value"`
		Raw   struct {
			String string `json:"string"`
			Value  int64  `json:"value"`
		} `json:"raw"`
	} `json:"table"`
}

type nvmeLogDoc struct {
	CriticalWarning         *int   `json:"critical_warning"`
	Temperature             *int   `json:"temperature"`
	AvailableSpare          *int   `json:"available_spare"`
	AvailableSpareThreshold *int   `json:"available_spare_threshold"`
	PercentageUsed          *int   `json:"percentage_used"`
	MediaErrors             *int64 `json:"media_errors"`
	NumErrLogEntries        *int64 `json:"num_err_log_entries"`
	PowerOnHours            *int64 `json:"power_on_hours"`
	UnsafeShutdowns         *int64 `json:"unsafe_shutdowns"`
	PowerCycles             *int64 `json:"power_cycles"`
	DataUnitsRead           *int64 `json:"data_units_read"`
	DataUnitsWritten        *int64 `json:"data_units_written"`
}

// ParseSmartctlJSON normalizes smartctl JSON; does not assume healthy on parse success alone.
func ParseSmartctlJSON(data []byte, exitCode int) (DeviceHealth, error) {
	if len(strings.TrimSpace(string(data))) == 0 {
		return DeviceHealth{}, fmt.Errorf("smartctl json empty")
	}
	var doc smartctlDoc
	if err := json.Unmarshal(data, &doc); err != nil {
		return DeviceHealth{}, fmt.Errorf("smartctl json: %w", err)
	}
	h := DeviceHealth{
		RawJSON: string(data),
	}
	if exitCode != 0 {
		code := exitCode
		h.ToolExitCode = &code
		h.DiagnosticFlags = SmartctlExitDiagnosticSummary(exitCode)
	}
	hasEvidence := false

	if len(doc.SmartStatus) > 0 {
		var st smartStatusDoc
		if err := json.Unmarshal(doc.SmartStatus, &st); err == nil {
			if st.Passed != nil {
				h.SmartPassed = st.Passed
				hasEvidence = true
			}
			if st.Status != nil && st.Status.String != nil && *st.Status.String != "" {
				h.SmartStatusString = *st.Status.String
				hasEvidence = true
			}
		}
	}

	if len(doc.NVMeSmartHealthInformationLog) > 0 {
		var nvme nvmeLogDoc
		if err := json.Unmarshal(doc.NVMeSmartHealthInformationLog, &nvme); err == nil {
			if nvme.CriticalWarning != nil {
				if *nvme.CriticalWarning < 0 || *nvme.CriticalWarning > 255 {
					return DeviceHealth{}, fmt.Errorf("critical_warning out of range")
				}
				cw := uint8(*nvme.CriticalWarning)
				h.CriticalWarning = &cw
				hasEvidence = true
			}
			if nvme.Temperature != nil {
				h.Temperature = &SmartNumeric{Value: int64Ptr(int64(*nvme.Temperature)), Unit: "celsius", Normalized: "nvme_smart_health_information_log.temperature"}
				hasEvidence = true
			}
			if nvme.PercentageUsed != nil {
				h.PercentageUsed = &SmartNumeric{Value: int64Ptr(int64(*nvme.PercentageUsed)), Unit: "percent", Normalized: "percentage_used"}
				hasEvidence = true
			}
			if nvme.AvailableSpare != nil {
				h.AvailableSpare = &SmartNumeric{Value: int64Ptr(int64(*nvme.AvailableSpare)), Unit: "percent", Normalized: "available_spare"}
				hasEvidence = true
			}
			if nvme.AvailableSpareThreshold != nil {
				h.SpareThreshold = &SmartNumeric{Value: int64Ptr(int64(*nvme.AvailableSpareThreshold)), Unit: "percent", Normalized: "spare_threshold"}
				hasEvidence = true
			}
			if nvme.MediaErrors != nil {
				h.MediaErrors = &SmartNumeric{Value: nvme.MediaErrors, Normalized: "media_errors"}
				hasEvidence = true
			}
			if nvme.NumErrLogEntries != nil {
				h.ErrorLogEntries = &SmartNumeric{Value: nvme.NumErrLogEntries, Normalized: "num_err_log_entries"}
				hasEvidence = true
			}
			if nvme.PowerOnHours != nil {
				h.PowerOnHours = &SmartNumeric{Value: nvme.PowerOnHours, Unit: "hours", Normalized: "power_on_hours"}
				hasEvidence = true
			}
			if nvme.UnsafeShutdowns != nil {
				h.UnsafeShutdowns = &SmartNumeric{Value: nvme.UnsafeShutdowns, Normalized: "unsafe_shutdowns"}
				hasEvidence = true
			}
			if nvme.PowerCycles != nil {
				h.PowerCycles = &SmartNumeric{Value: nvme.PowerCycles, Normalized: "power_cycles"}
				hasEvidence = true
			}
			if nvme.DataUnitsRead != nil {
				h.ReadLifetime = &SmartNumeric{
					Value:      nvme.DataUnitsRead,
					Unit:       "data_units",
					Normalized: "data_units_read",
					Raw:        fmt.Sprintf("%d units (%d bytes/unit)", *nvme.DataUnitsRead, NVMeDataUnitBytes),
				}
				hasEvidence = true
			}
			if nvme.DataUnitsWritten != nil {
				h.WriteLifetime = &SmartNumeric{
					Value:      nvme.DataUnitsWritten,
					Unit:       "data_units",
					Normalized: "data_units_written",
					Raw:        fmt.Sprintf("%d units (%d bytes/unit)", *nvme.DataUnitsWritten, NVMeDataUnitBytes),
				}
				hasEvidence = true
			}
		}
	}

	if h.Temperature == nil && len(doc.Temperature) > 0 {
		var t temperatureDoc
		if err := json.Unmarshal(doc.Temperature, &t); err == nil && t.Current != nil {
			h.Temperature = &SmartNumeric{Value: int64Ptr(int64(*t.Current)), Unit: "celsius", Normalized: "temperature.current"}
			hasEvidence = true
		}
	}

	if len(doc.ATAAttributes) > 0 {
		var ata ataTableDoc
		if err := json.Unmarshal(doc.ATAAttributes, &ata); err == nil {
			for _, attr := range ata.Table {
				name := strings.ToLower(attr.Name)
				switch name {
				case "temperature_celsius", "airflow_temperature_cel":
					if h.Temperature == nil {
						h.Temperature = parseATATemperature(attr)
						if h.Temperature != nil {
							hasEvidence = true
						}
					}
				case "power_on_hours":
					h.PowerOnHours = &SmartNumeric{Value: int64Ptr(attr.Raw.Value), Raw: attr.Raw.String, Unit: "hours", Normalized: name}
					hasEvidence = true
				case "unsafe_shutdown_count":
					h.UnsafeShutdowns = &SmartNumeric{Value: int64Ptr(attr.Raw.Value), Raw: attr.Raw.String, Normalized: name}
					hasEvidence = true
				case "media_wearout_indicator":
					h.PercentageUsed = &SmartNumeric{Raw: attr.Raw.String, Normalized: name}
					hasEvidence = true
				}
			}
		}
	}

	if SmartctlExitIndicatesHealthFailure(exitCode) {
		if h.SmartPassed == nil {
			f := false
			h.SmartPassed = &f
			hasEvidence = true
		}
	}

	if !hasEvidence {
		return DeviceHealth{}, fmt.Errorf("smartctl json without usable health evidence")
	}
	h = reconcileSmartHealthOutcome(h, exitCode)
	return h, nil
}

func parseATATemperature(attr struct {
	Name  string `json:"name"`
	Value int    `json:"value"`
	Raw   struct {
		String string `json:"string"`
		Value  int64  `json:"value"`
	} `json:"raw"`
}) *SmartNumeric {
	raw := attr.Raw.Value
	rawStr := strings.TrimSpace(attr.Raw.String)
	if temp, ok := parseTemperatureFromRawString(rawStr); ok {
		return &SmartNumeric{Value: int64Ptr(temp), Raw: rawStr, Unit: "celsius", Normalized: "ata_raw_string_temperature"}
	}
	if raw >= 0 && raw <= 150 {
		return &SmartNumeric{Value: int64Ptr(raw), Raw: rawStr, Unit: "celsius", Normalized: "ata_raw_value_temperature"}
	}
	// Never treat normalized attribute Value as degrees Celsius.
	return &SmartNumeric{Raw: rawStr, Normalized: attr.Name, Unit: "unknown"}
}

func parseTemperatureFromRawString(s string) (int64, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, false
	}
	if i := strings.Index(s, "("); i > 0 {
		s = strings.TrimSpace(s[:i])
	}
	v, err := strconv.ParseInt(s, 10, 64)
	if err != nil || v < 0 || v > 150 {
		return 0, false
	}
	return v, true
}

func reconcileSmartHealthOutcome(h DeviceHealth, exitCode int) DeviceHealth {
	if SmartctlExitIndicatesOperationalError(exitCode) {
		h.CollectionStatus = HealthPartial
		if h.CollectionDetail == "" {
			h.CollectionDetail = "operational smartctl exit with partial JSON evidence"
		}
		return h
	}
	if h.SmartPassed != nil && *h.SmartPassed && SmartctlExitIndicatesHealthFailure(exitCode) {
		f := false
		h.SmartPassed = &f
		h.CollectionStatus = HealthConflict
		h.CollectionDetail = "smart_status.passed contradicts smartctl health failure exit bits"
		return h
	}
	if SmartctlExitIndicatesHealthFailure(exitCode) || (h.SmartPassed != nil && !*h.SmartPassed) {
		h.CollectionStatus = HealthCollected
		return h
	}
	if SmartctlExitIndicatesDiagnosticEvidence(exitCode) {
		h.CollectionStatus = HealthCollected
		return h
	}
	h.CollectionStatus = HealthCollected
	return h
}

func int64Ptr(v int64) *int64 {
	return &v
}
