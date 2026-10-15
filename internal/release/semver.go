package release

import (
	"fmt"
	"strings"

	"golang.org/x/mod/semver"
)

// CompareSemver returns -1 if a<b, 0 if equal, 1 if a>b (SemVer 2.0 including prerelease).
func CompareSemver(a, b string) (int, error) {
	ca, err := canonicalSemver(a)
	if err != nil {
		return 0, err
	}
	cb, err := canonicalSemver(b)
	if err != nil {
		return 0, err
	}
	return semver.Compare(ca, cb), nil
}

func canonicalSemver(v string) (string, error) {
	v = strings.TrimSpace(v)
	if v == "" {
		return "", fmt.Errorf("invalid semver %q", v)
	}
	if !strings.HasPrefix(v, "v") {
		v = "v" + strings.TrimPrefix(v, "v")
	}
	if !semver.IsValid(v) {
		return "", fmt.Errorf("invalid semver %q", v)
	}
	return semver.Canonical(v), nil
}
