package device

// smartctl exit status bits (smartmontools smartctl.h).
const (
	smartctlExitSyntax        = 0x01 // command line syntax error
	smartctlExitOpen          = 0x02 // device open failed
	smartctlExitSmartCmd      = 0x04 // SMART command failed
	smartctlExitFailingNow    = 0x08 // SMART FAILING NOW
	smartctlExitPrefail       = 0x10 // prefail threshold exceeded
	smartctlExitPastThreshold = 0x20 // FAILAGE: threshold exceeded in the past
	smartctlExitATAErrorLog   = 0x40 // FAILERR: ATA error log contains records
	smartctlExitSelfTestLog   = 0x80 // FAILLOG: self-test log contains records
)

// SmartctlExitIndicatesHealthFailure reports device-health failure bits in the exit status.
func SmartctlExitIndicatesHealthFailure(exitCode int) bool {
	return exitCode&smartctlExitFailingNow != 0 || exitCode&smartctlExitPrefail != 0
}

// SmartctlExitIndicatesOperationalError reports command/access errors distinct from health failure.
func SmartctlExitIndicatesOperationalError(exitCode int) bool {
	return exitCode&smartctlExitSyntax != 0 || exitCode&smartctlExitOpen != 0 || exitCode&smartctlExitSmartCmd != 0
}

// SmartctlExitIndicatesDiagnosticEvidence reports ATA error/self-test/past-threshold log bits.
func SmartctlExitIndicatesDiagnosticEvidence(exitCode int) bool {
	return exitCode&smartctlExitPastThreshold != 0 || exitCode&smartctlExitATAErrorLog != 0 || exitCode&smartctlExitSelfTestLog != 0
}

// SmartctlExitDiagnosticSummary returns human-readable diagnostic flags from exit bits.
func SmartctlExitDiagnosticSummary(exitCode int) []string {
	var out []string
	if exitCode&smartctlExitPastThreshold != 0 {
		out = append(out, "past_threshold")
	}
	if exitCode&smartctlExitATAErrorLog != 0 {
		out = append(out, "ata_error_log")
	}
	if exitCode&smartctlExitSelfTestLog != 0 {
		out = append(out, "self_test_log")
	}
	return out
}
