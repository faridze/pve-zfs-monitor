package outbox

import "errors"

var (
	errInvalidEntry    = errors.New("invalid outbox entry")
	errInvalidSnapshot = errors.New("invalid outbox snapshot")
	errFull            = errors.New("outbox full")
)
