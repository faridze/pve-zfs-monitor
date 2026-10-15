package device

import "testing"

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
