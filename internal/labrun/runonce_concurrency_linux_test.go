//go:build linux

package labrun

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/faridze/pve-zfs-monitor/internal/device"
	"github.com/faridze/pve-zfs-monitor/internal/zfs"
)

func TestConcurrentRunOnceRespectsStateLock(t *testing.T) {
	dir := labDir(t)
	c := Collectors{
		ListPools: func(context.Context) ([]zfs.Pool, error) {
			time.Sleep(50 * time.Millisecond)
			return []zfs.Pool{{Name: "p1", Health: "ONLINE"}}, nil
		},
		CollectTopology: func(context.Context) (*zfs.TopologyReport, error) { return nil, nil },
		CollectDevices:  func(context.Context) (*device.DeviceReport, error) { return nil, nil },
		PoolTimeout:     time.Second,
	}
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := RunOnce(context.Background(), true, dir, DeliveryNone, c, "lab")
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)
	success := 0
	for err := range errs {
		if err == nil {
			success++
			continue
		}
		if !strings.Contains(strings.ToLower(err.Error()), "lock") {
			t.Fatalf("unexpected err: %v", err)
		}
	}
	if success == 0 {
		t.Fatal("expected at least one successful run")
	}
}
