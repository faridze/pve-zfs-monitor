//go:build !linux

package delivery

import (
	"errors"
	"time"

	"github.com/faridze/pve-zfs-monitor/internal/monitor"
	"github.com/faridze/pve-zfs-monitor/internal/outbox"
)

// Store is unavailable on non-Linux builds.
type Store struct {
	Directory string
}

// Coordinator is unavailable on non-Linux builds.
type Coordinator struct {
	Store Store
}

func (c Coordinator) AdvanceDetection(monitor.DetectionInput, time.Time, monitor.Options, monitor.DetectionOptions) (monitor.State, []monitor.Event, error) {
	return monitor.State{}, nil, errors.New("delivery store requires linux")
}

func (c Coordinator) DeliverNotifications(string, outbox.Transport, time.Time) (int, error) {
	return 0, errors.New("delivery store requires linux")
}

func (c Coordinator) PendingNotifications() (int, error) {
	return 0, errors.New("delivery store requires linux")
}
