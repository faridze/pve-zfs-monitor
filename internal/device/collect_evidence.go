package device

// HasSMARTCollectionEvidence reports whether the device report contains independent
// proof of SMART/NVMe collection beyond completeness flags alone.
func HasSMARTCollectionEvidence(report *DeviceReport) bool {
	if report == nil {
		return false
	}
	if len(report.Inventory) == 0 && len(report.DeviceHealth) == 0 {
		return false
	}
	if len(report.DeviceHealth) == 0 {
		return false
	}
	for _, h := range report.DeviceHealth {
		if h.SmartPassed == nil && h.CriticalWarning == nil {
			return false
		}
	}
	if report.InventoryCompleteness == ReportInventoryComplete && len(report.Inventory) > 0 {
		if len(report.DeviceHealth) < len(report.Inventory) {
			return false
		}
	}
	return true
}
