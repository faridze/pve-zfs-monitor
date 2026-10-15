package release

import (
	"context"
	"crypto/ed25519"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

// Updater performs verified download and atomic installation.
type Updater struct {
	Client           HTTPClient
	PublicKey        ed25519.PublicKey
	Current          Info
	Channel          Channel
	VersionPin       string // empty or "latest"
	InstallOpts      InstallOptions
	LockPath         string
	ExecutableGOOS   string
	ExecutableGOARCH string
}

// CheckForUpdate fetches and plans an upgrade without modifying the system.
func (u *Updater) CheckForUpdate(ctx context.Context) (*UpdatePlan, []byte, *SignatureEnvelope, error) {
	urls, err := ResolveChannelURLs(u.Channel, u.VersionPin)
	if err != nil {
		return nil, nil, nil, err
	}
	manifest, raw, sig, err := FetchChannelManifest(ctx, u.Client, urls, u.PublicKey)
	if err != nil {
		return nil, nil, nil, err
	}
	if manifest.Channel != u.Channel {
		return nil, nil, nil, fmt.Errorf("manifest channel %q does not match requested %q", manifest.Channel, u.Channel)
	}
	plan, err := PlanUpdate(u.Current, manifest, u.ExecutableGOOS, u.ExecutableGOARCH, false)
	if err != nil {
		return nil, nil, nil, err
	}
	plan.ManifestRaw = raw
	plan.Signature = *sig
	return plan, raw, sig, nil
}

// ApplyUpdate downloads, verifies, and atomically installs. dryRun stops before install.
func (u *Updater) ApplyUpdate(ctx context.Context, plan *UpdatePlan, dryRun bool) error {
	if plan == nil {
		return fmt.Errorf("missing update plan")
	}
	lock, err := AcquireUpdateLock(u.LockPath)
	if err != nil {
		return err
	}
	defer lock.Release()

	tmpDir, err := os.MkdirTemp(filepath.Dir(u.InstallOpts.TargetPath), "pve-zfs-monitor-dl-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmpDir)

	staged := filepath.Join(tmpDir, plan.Artifact.Filename)
	if err := DownloadArtifact(ctx, u.Client, plan.Artifact, staged); err != nil {
		return err
	}
	if err := VerifyArtifactFile(staged, plan.Artifact); err != nil {
		return err
	}
	if dryRun {
		return nil
	}
	return AtomicInstall(staged, u.InstallOpts)
}

// InstallFromPaths verifies a local manifest/signature and staged binary (bootstrap installer).
func InstallFromPaths(manifestPath, sigPath, binaryPath string, pub ed25519.PublicKey, opts InstallOptions) error {
	raw, err := os.ReadFile(manifestPath)
	if err != nil {
		return err
	}
	sigRaw, err := os.ReadFile(sigPath)
	if err != nil {
		return err
	}
	var env SignatureEnvelope
	if err := json.Unmarshal(sigRaw, &env); err != nil {
		return err
	}
	if err := VerifyManifestSignature(raw, &env, pub); err != nil {
		return err
	}
	m, err := ParseManifest(raw)
	if err != nil {
		return err
	}
	art, err := m.SelectArtifact(runtime.GOOS, runtime.GOARCH)
	if err != nil {
		return err
	}
	if err := VerifyArtifactFile(binaryPath, *art); err != nil {
		return err
	}
	return AtomicInstall(binaryPath, opts)
}
