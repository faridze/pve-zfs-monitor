VERSION ?= 0.0.0-dev
COMMIT ?= $(shell git rev-parse HEAD 2>/dev/null || echo unknown)
BUILD_TIME ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
CHANNEL ?= lab
TRUSTED ?= false
TRUST_KEY_B64 ?=
RELEASE_BASE ?=

LDFLAGS = -s -w \
  -X github.com/faridze/pve-zfs-monitor/internal/release.Version=$(VERSION) \
  -X github.com/faridze/pve-zfs-monitor/internal/release.Commit=$(COMMIT) \
  -X github.com/faridze/pve-zfs-monitor/internal/release.BuildTime=$(BUILD_TIME) \
  -X github.com/faridze/pve-zfs-monitor/internal/release.ChannelName=$(CHANNEL) \
  -X github.com/faridze/pve-zfs-monitor/internal/release.TrustedRelease=$(TRUSTED) \
  -X github.com/faridze/pve-zfs-monitor/internal/release.TrustedPublicKeyB64=$(TRUST_KEY_B64) \
  -X github.com/faridze/pve-zfs-monitor/internal/release.DefaultReleaseBaseURL=$(RELEASE_BASE)

.PHONY: build
build:
	CGO_ENABLED=0 go build -trimpath -ldflags "$(LDFLAGS)" -o bin/pve-zfs-monitor ./cmd/pve-zfs-monitor

.PHONY: build-linux-amd64 build-linux-arm64
build-linux-amd64:
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags "$(LDFLAGS)" -o bin/pve-zfs-monitor-linux-amd64 ./cmd/pve-zfs-monitor

build-linux-arm64:
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -trimpath -ldflags "$(LDFLAGS)" -o bin/pve-zfs-monitor-linux-arm64 ./cmd/pve-zfs-monitor
