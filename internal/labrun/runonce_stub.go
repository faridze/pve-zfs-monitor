//go:build !linux

package labrun

import (
	"context"
	"errors"
)

// RunOnce is only available on Linux (durable delivery store).
func RunOnce(context.Context, bool, string, DeliveryMode, Collectors, string) (RunOnceReport, error) {
	return RunOnceReport{}, errors.New("lab run-once requires linux")
}
