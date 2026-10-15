package release

import "testing"

func TestSemverPrereleaseOrdering(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"1.0.0-rc1", "1.0.0-rc2", -1},
		{"1.0.0-rc2", "1.0.0", -1},
		{"1.0.0", "1.0.0-rc1", 1},
		{"1.0.0+build1", "1.0.0+build2", 0},
	}
	for _, tc := range cases {
		got, err := CompareSemver(tc.a, tc.b)
		if err != nil {
			t.Fatalf("%s vs %s: %v", tc.a, tc.b, err)
		}
		if got != tc.want {
			t.Fatalf("%s vs %s: got %d want %d", tc.a, tc.b, got, tc.want)
		}
	}
}

func TestSemverInvalidRejected(t *testing.T) {
	if _, err := CompareSemver("not-a-version", "1.0.0"); err == nil {
		t.Fatal("expected invalid version error")
	}
}
