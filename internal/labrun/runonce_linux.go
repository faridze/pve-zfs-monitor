//go:build linux

package labrun

import (
	"context"
	"errors"
	"fmt"

	"github.com/faridze/pve-zfs-monitor/internal/delivery"
	"github.com/faridze/pve-zfs-monitor/internal/outbox"
)

const atLeastOnceNote = "Simulated delivery uses at-least-once semantics; crash after transport success but before durable ack may duplicate simulated sends on retry."

// RunOnce executes one lab monitoring cycle with durable state on Linux.
func RunOnce(ctx context.Context, labMode bool, stateDir string, mode DeliveryMode, collectors Collectors, hostLabel string) (RunOnceReport, error) {
	if !labMode {
		return RunOnceReport{}, errors.New("lab mode must be explicitly enabled")
	}
	if err := ValidateLabStateDir(stateDir); err != nil {
		return RunOnceReport{}, err
	}
	if mode == "" {
		return RunOnceReport{}, errors.New("delivery mode required")
	}
	if hostLabel == "" {
		hostLabel = "lab"
	}
	if collectors.ListPools == nil || collectors.CollectTopology == nil || collectors.CollectDevices == nil {
		return RunOnceReport{}, errors.New("collectors not configured")
	}

	in, failures, err := BuildDetectionInput(ctx, collectors)
	if err != nil {
		return RunOnceReport{}, err
	}
	now := evalNow(collectors)
	opts, detectOpts := DefaultMonitorOptions()

	coord := delivery.Coordinator{Store: delivery.Store{Directory: stateDir}}
	state, events, err := coord.AdvanceDetection(in, now, opts, detectOpts)
	if err != nil {
		return RunOnceReport{
			Lab: true, DeliveryMode: string(mode), CollectionFailures: failures,
			StatePersisted: false,
		}, fmt.Errorf("advance detection: %w", err)
	}
	signals, err := SignalSummary(in, state.Baselines, now, detectOpts)
	if err != nil {
		return RunOnceReport{}, err
	}
	overall, crit, warn, unk, healthy := summarizeSignals(signals)

	pending, err := coord.PendingNotifications()
	if err != nil {
		return RunOnceReport{}, err
	}

	report := RunOnceReport{
		Lab:                  true,
		DeliveryMode:         string(mode),
		Overall:              overall,
		VerifiedCritical:     crit,
		VerifiedWarning:      warn,
		UnknownSignals:       unk,
		HealthySignals:       healthy,
		EventsEmitted:        safeEvents(events),
		EventsEmittedCount:   len(events),
		PendingNotifications: pending,
		CollectionFailures:   failures,
		StatePersisted:       true,
	}

	switch mode {
	case DeliveryNone:
		return report, nil
	case DeliveryFake:
		report.AtLeastOnceLimitation = atLeastOnceNote
		n, err := coord.DeliverNotifications(hostLabel, &outbox.FakeTransport{}, now)
		if err != nil {
			return report, fmt.Errorf("simulated delivery: %w", err)
		}
		report.SimulatedDeliveries = n
		report.SimulatedAck = n > 0 || pending == 0
		pendingAfter, _ := coord.PendingNotifications()
		report.PendingNotifications = pendingAfter
		return report, nil
	default:
		return RunOnceReport{}, errors.New("unsupported delivery mode")
	}
}
