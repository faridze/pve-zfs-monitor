package device

// smartctl exit status bits (smartmontools smartctl.h).
const (
	smartctlExitSyntax     = 0x01 // command line syntax error
	smartctlExitOpen       = 0x02 // device open failed
	smartctlExitSmartCmd   = 0x04 // SMART command failed
	smartctlExitFailingNow = 0x08 // SMART status: disk failing
	smartctlExitPrefail    = 0x10 // prefail threshold exceeded
	smartctlExitErrorLog   = 0x20 // error log contains records
	smartctlExitSelfTest   = 0x40 // self-test log contains records
)

// SmartctlExitIndicatesHealthFailure reports device-health failure bits in the exit status.
func SmartctlExitIndicatesHealthFailure(exitCode int) bool {
	return exitCode&smartctlExitFailingNow != 0 || exitCode&smartctlExitPrefail != 0
}

// SmartctlExitIndicatesOperationalError reports command/access errors distinct from health failure.
func SmartctlExitIndicatesOperationalError(exitCode int) bool {
	return exitCode&smartctlExitSyntax != 0 || exitCode&smartctlExitOpen != 0 || exitCode&smartctlExitSmartCmd != 0
}
