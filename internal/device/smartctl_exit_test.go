package device

import "testing"

func TestSmartctlExitDiagnosticBits(t *testing.T) {
	if !SmartctlExitIndicatesDiagnosticEvidence(0x20) {
		t.Fatal("0x20 past threshold")
	}
	if !SmartctlExitIndicatesDiagnosticEvidence(0x40) {
		t.Fatal("0x40 ata error log")
	}
	if !SmartctlExitIndicatesDiagnosticEvidence(0x80) {
		t.Fatal("0x80 self-test log")
	}
	flags := SmartctlExitDiagnosticSummary(0xE0)
	if len(flags) != 3 {
		t.Fatalf("flags=%v", flags)
	}
}

func TestSmartctlExitCombinations(t *testing.T) {
	cases := []struct {
		code        int
		healthFail  bool
		operational bool
	}{
		{2, false, true},
		{8, true, false},
		{16, true, false},
		{64, false, false},
		{128, false, false},
		{10, true, true}, // open + failing
	}
	for _, tc := range cases {
		if SmartctlExitIndicatesHealthFailure(tc.code) != tc.healthFail {
			t.Fatalf("code %d healthFail", tc.code)
		}
		if SmartctlExitIndicatesOperationalError(tc.code) != tc.operational {
			t.Fatalf("code %d operational", tc.code)
		}
	}
}
