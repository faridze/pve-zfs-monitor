package release

import "errors"

// ErrUpToDate indicates the installed version matches the latest signed release.
var ErrUpToDate = errors.New("already up to date")
