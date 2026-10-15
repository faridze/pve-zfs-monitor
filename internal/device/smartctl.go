package device

import (
	"encoding/json"
	"fmt"
	"strings"
)

// SmartctlPath is the fixed smartmontools binary on Proxmox.
const SmartctlPath = "/usr/sbin/smartctl"

// SmartctlHealthArgs requests JSON health/all attributes read-only.
var SmartctlHealthArgs = func(device string) []string {
	return []string{"--json=o", "-a", "-d", "auto", device}
}

type smartctlDoc struct {
	SmartStatus struct {
		Passed bool `json:"passed"`
		Status struct {
			String string `json:"string"`
		} `json:"status"`
	} `json:"smart_status"`
	Temperature struct {
		Current int `json:"current"`
	} `json:"temperature"`
	ATAAttributes struct {
		Table []struct {
			Name  string `json:"name"`
			Value int    `json:"value"`
			Raw   struct {
				String string `json:"string"`
				Value  int64  `json:"value"`
			} `json:"raw"`
		} `json:"table"`
	} `json:"ata_smart_attributes"`
	NVMeSmartHealthInformationLog struct {
		CriticalWarning         int   `json:"critical_warning"`
		Temperature             int   `json:"temperature"`
		AvailableSpare          int   `json:"available_spare"`
		AvailableSpareThreshold int   `json:"available_spare_threshold"`
		PercentageUsed          int   `json:"percentage_used"`
		MediaErrors             int64 `json:"media_errors"`
		NumErrLogEntries        int64 `json:"num_err_log_entries"`
		PowerOnHours            int64 `json:"power_on_hours"`
		UnsafeShutdowns         int64 `json:"unsafe_shutdowns"`
		PowerCycles             int64 `json:"power_cycles"`
	} `json:"nvme_smart_health_information_log"`
}

// ParseSmartctlJSON normalizes smartctl JSON; does not assume healthy on parse success alone.
func ParseSmartctlJSON(data []byte, exitCode int) (DeviceHealth, error) {
	var doc smartctlDoc
	if err := json.Unmarshal(data, &doc); err != nil {
		return DeviceHealth{}, fmt.Errorf("smartctl json: %w", err)
	}
	h := DeviceHealth{
		CollectionStatus: HealthCollected,
		RawJSON:          string(data),
	}
	if exitCode != 0 {
		code := exitCode
		h.ToolExitCode = &code
	}
	if doc.SmartStatus.Passed || doc.SmartStatus.Status.String != "" {
		passed := doc.SmartStatus.Passed
		h.SmartPassed = &passed
		h.SmartStatusString = doc.SmartStatus.Status.String
	}
	nvme := doc.NVMeSmartHealthInformationLog
	if nvme.Temperature > 0 || nvme.PercentageUsed > 0 || nvme.AvailableSpare > 0 {
		cw := uint8(nvme.CriticalWarning)
		h.CriticalWarning = &cw
		h.Temperature = &SmartNumeric{Value: int64Ptr(int64(nvme.Temperature)), Unit: "celsius", Normalized: "nvme_smart_health_information_log.temperature"}
		h.PercentageUsed = &SmartNumeric{Value: int64Ptr(int64(nvme.PercentageUsed)), Unit: "percent", Normalized: "percentage_used"}
		h.AvailableSpare = &SmartNumeric{Value: int64Ptr(int64(nvme.AvailableSpare)), Unit: "percent", Normalized: "available_spare"}
		h.SpareThreshold = &SmartNumeric{Value: int64Ptr(int64(nvme.AvailableSpareThreshold)), Unit: "percent", Normalized: "spare_threshold"}
		h.MediaErrors = &SmartNumeric{Value: int64Ptr(nvme.MediaErrors), Normalized: "media_errors"}
		h.ErrorLogEntries = &SmartNumeric{Value: int64Ptr(nvme.NumErrLogEntries), Normalized: "num_err_log_entries"}
		h.PowerOnHours = &SmartNumeric{Value: int64Ptr(nvme.PowerOnHours), Unit: "hours", Normalized: "power_on_hours"}
		h.UnsafeShutdowns = &SmartNumeric{Value: int64Ptr(nvme.UnsafeShutdowns), Normalized: "unsafe_shutdowns"}
		h.PowerCycles = &SmartNumeric{Value: int64Ptr(nvme.PowerCycles), Normalized: "power_cycles"}
	} else if doc.Temperature.Current > 0 {
		h.Temperature = &SmartNumeric{Value: int64Ptr(int64(doc.Temperature.Current)), Unit: "celsius", Normalized: "temperature.current"}
	}
	for _, attr := range doc.ATAAttributes.Table {
		name := strings.ToLower(attr.Name)
		switch name {
		case "temperature_celsius", "airflow_temperature_cel":
			if h.Temperature == nil {
				h.Temperature = &SmartNumeric{Value: int64Ptr(int64(attr.Value)), Raw: attr.Raw.String, Unit: "celsius", Normalized: name}
			}
		case "power_on_hours":
			h.PowerOnHours = &SmartNumeric{Value: int64Ptr(attr.Raw.Value), Raw: attr.Raw.String, Unit: "hours", Normalized: name}
		case "unsafe_shutdown_count":
			h.UnsafeShutdowns = &SmartNumeric{Value: int64Ptr(attr.Raw.Value), Raw: attr.Raw.String, Normalized: name}
		case "media_wearout_indicator":
			// Do not invent percentage_used for SATA; record raw only.
			h.PercentageUsed = &SmartNumeric{Raw: attr.Raw.String, Normalized: name}
		}
	}
	return h, nil
}

func int64Ptr(v int64) *int64 {
	return &v
}

// SmartctlExitIndicatesFailure returns true when exit code has failing SMART bits but JSON may still be valid.
func SmartctlExitIndicatesFailure(exitCode int) bool {
	return exitCode&0x01 != 0 || exitCode&0x02 != 0 || exitCode&0x04 != 0
}
