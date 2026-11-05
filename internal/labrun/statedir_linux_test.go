//go:build linux

package labrun

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/faridze/pve-zfs-monitor/internal/device"
	"github.com/faridze/pve-zfs-monitor/internal/zfs"
)

func TestValidateLabAuthorizationRejectsPrivateDirWithoutMarker(t *testing.T) {
	d := t.TempDir()
	if err := os.Chmod(d, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := ValidateLabAuthorization(d); err == nil {
		t.Fatal("expected missing marker error")
	}
}

func TestValidateLabAuthorizationRejectsSymlinkDir(t *testing.T) {
	base := t.TempDir()
	target := filepath.Join(base, "target")
	if err := os.Mkdir(target, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := WriteLabMarker(target); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(base, "link")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	if err := ValidateLabAuthorization(link); err == nil {
		t.Fatal("expected symlink rejection")
	}
}

func TestValidateLabAuthorizationAcceptsMarker(t *testing.T) {
	d := labDir(t)
	if err := ValidateLabAuthorization(d); err != nil {
		t.Fatal(err)
	}
}

func TestCollectionFailuresRedactedInReport(t *testing.T) {
	dir := labDir(t)
	secret := "SERIAL-Z999 /dev/sda host-prod.internal"
	c := Collectors{
		ListPools: func(context.Context) ([]zfs.Pool, error) {
			return nil, errors.New(secret)
		},
		CollectTopology: func(context.Context) (*zfs.TopologyReport, error) {
			return nil, errors.New(secret)
		},
		CollectDevices: func(context.Context) (*device.DeviceReport, error) {
			return nil, errors.New(secret)
		},
	}
	report, err := RunOnce(context.Background(), true, dir, DeliveryNone, c, "lab")
	if err != nil {
		t.Fatal(err)
	}
	if len(report.CollectionFailures) != 3 {
		t.Fatalf("categories: %v", report.CollectionFailures)
	}
	raw, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	s := string(raw)
	if strings.Contains(s, "SERIAL") || strings.Contains(s, "/dev/sda") || strings.Contains(s, "host-prod") {
		t.Fatalf("leaked collector detail in JSON: %s", s)
	}
}
