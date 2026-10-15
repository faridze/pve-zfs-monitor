package release

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// HTTPClient fetches release metadata and artifacts.
type HTTPClient interface {
	Get(ctx context.Context, url string) ([]byte, error)
}

// NetHTTPClient uses net/http with bounded body size.
type NetHTTPClient struct {
	Client    *http.Client
	MaxBytes  int64
	UserAgent string
}

func (c NetHTTPClient) Get(ctx context.Context, url string) ([]byte, error) {
	if c.Client == nil {
		c.Client = &http.Client{Timeout: 60 * time.Second}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	if c.UserAgent != "" {
		req.Header.Set("User-Agent", c.UserAgent)
	}
	resp, err := c.Client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET %s: HTTP %d", url, resp.StatusCode)
	}
	max := c.MaxBytes
	if max <= 0 {
		max = 32 << 20
	}
	lim := io.LimitReader(resp.Body, max+1)
	data, err := io.ReadAll(lim)
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > max {
		return nil, fmt.Errorf("response exceeds limit")
	}
	return data, nil
}

// ChannelURLs resolves manifest and signature URLs for a channel.
type ChannelURLs struct {
	ManifestURL  string
	SignatureURL string
}

// UpdatePlan describes a pending upgrade.
type UpdatePlan struct {
	Current     Info
	Available   Manifest
	Artifact    Artifact
	ManifestRaw []byte
	Signature   SignatureEnvelope
}

// FetchChannelManifest downloads and verifies a signed manifest.
func FetchChannelManifest(ctx context.Context, client HTTPClient, urls ChannelURLs, pub ed25519.PublicKey) (*Manifest, []byte, *SignatureEnvelope, error) {
	raw, err := client.Get(ctx, urls.ManifestURL)
	if err != nil {
		return nil, nil, nil, err
	}
	sigRaw, err := client.Get(ctx, urls.SignatureURL)
	if err != nil {
		return nil, nil, nil, err
	}
	var env SignatureEnvelope
	if err := json.Unmarshal(sigRaw, &env); err != nil {
		return nil, nil, nil, fmt.Errorf("signature json: %w", err)
	}
	if err := VerifyManifestSignature(raw, &env, pub); err != nil {
		return nil, nil, nil, err
	}
	m, err := ParseManifest(raw)
	if err != nil {
		return nil, nil, nil, err
	}
	return m, raw, &env, nil
}

// PlanUpdate compares installed version with a verified remote manifest.
func PlanUpdate(current Info, remote *Manifest, goos, goarch string, allowDowngrade bool) (*UpdatePlan, error) {
	if remote == nil {
		return nil, fmt.Errorf("missing manifest")
	}
	if current.Channel != "" && string(remote.Channel) != current.Channel {
		return nil, fmt.Errorf("channel mismatch: installed %q manifest %q", current.Channel, remote.Channel)
	}
	cmp, err := CompareSemver(remote.Version, current.Version)
	if err != nil {
		return nil, err
	}
	if cmp < 0 && !allowDowngrade {
		return nil, fmt.Errorf("refusing downgrade from %s to %s", current.Version, remote.Version)
	}
	if cmp == 0 {
		return nil, fmt.Errorf("already at version %s", current.Version)
	}
	art, err := remote.SelectArtifact(goos, goarch)
	if err != nil {
		return nil, err
	}
	return &UpdatePlan{
		Current:   current,
		Available: *remote,
		Artifact:  *art,
	}, nil
}

// UpdateLock guards concurrent updates.
type UpdateLock struct {
	Path string
	f    *os.File
}

// AcquireUpdateLock creates an exclusive lock file.
func AcquireUpdateLock(path string) (*UpdateLock, error) {
	if path == "" {
		path = installLockPath()
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	if err := tryLock(f); err != nil {
		f.Close()
		return nil, fmt.Errorf("another update is in progress")
	}
	return &UpdateLock{Path: path, f: f}, nil
}

func (l *UpdateLock) Release() error {
	if l.f == nil {
		return nil
	}
	err := l.f.Close()
	l.f = nil
	return err
}

// tryLock uses a simple non-blocking flock on Unix.
func tryLock(f *os.File) error {
	return fileLock(f)
}

// DownloadArtifact writes a verified artifact to destPath (caller verifies after).
func DownloadArtifact(ctx context.Context, client HTTPClient, art Artifact, destPath string) error {
	if err := ValidateHTTPSURL(art.URL); err != nil {
		return err
	}
	if err := ValidateArtifactFilename(art.Filename); err != nil {
		return err
	}
	data, err := client.Get(ctx, art.URL)
	if err != nil {
		return err
	}
	if int64(len(data)) != art.Size {
		return fmt.Errorf("download size %d != expected %d", len(data), art.Size)
	}
	sum := sha256Hex(data)
	if !strings.EqualFold(sum, art.SHA256) {
		return fmt.Errorf("download checksum mismatch")
	}
	if err := os.WriteFile(destPath, data, 0o600); err != nil {
		return err
	}
	return nil
}

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
