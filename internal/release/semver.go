package release

import (
	"fmt"
	"strconv"
	"strings"
)

// CompareSemver returns -1 if a<b, 0 if equal, 1 if a>b. Pre-release segments are not supported.
func CompareSemver(a, b string) (int, error) {
	av, err := parseSemver(a)
	if err != nil {
		return 0, err
	}
	bv, err := parseSemver(b)
	if err != nil {
		return 0, err
	}
	for i := 0; i < 3; i++ {
		if av[i] < bv[i] {
			return -1, nil
		}
		if av[i] > bv[i] {
			return 1, nil
		}
	}
	return 0, nil
}

func parseSemver(v string) ([]int, error) {
	v = strings.TrimSpace(v)
	if strings.HasPrefix(v, "v") {
		v = v[1:]
	}
	if idx := strings.IndexByte(v, '-'); idx >= 0 {
		v = v[:idx]
	}
	parts := strings.Split(v, ".")
	if len(parts) != 3 {
		return nil, fmt.Errorf("invalid semver %q", v)
	}
	out := make([]int, 3)
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return nil, fmt.Errorf("invalid semver component %q", p)
		}
		out[i] = n
	}
	return out, nil
}
