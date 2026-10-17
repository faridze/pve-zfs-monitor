package release

import (
	"context"
	"crypto/ed25519"
	"fmt"
	"os"
	"path/filepath"
)

// Updater performs verified download and atomic installation.
type Updater struct {
	Secure           SecureHTTPClient
	Client           HTTPClient // optional test override for manifest and artifact fetches
	PublicKey        ed25519.PublicKey
	Current          Info
	Channel          Channel
	VersionPin       string
	InstallOpts      InstallOptions
	LockPath         string
	ExecutableGOOS   string
	ExecutableGOARCH string
}

func (u *Updater) lockPath() string {
	if u.LockPath != "" {
		return u.LockPath
	}
	return installLockPath()
}

// CheckForUpdate fetches and plans an upgrade without modifying the system.
func (u *Updater) CheckForUpdate(ctx context.Context) (*UpdatePlan, []byte, *SignatureEnvelope, error) {
	urls, err := ResolveChannelURLs(u.Channel, u.VersionPin)
	if err != nil {
		return nil, nil, nil, err
	}
	manifest, raw, sig, err := FetchChannelManifest(ctx, u.httpClient(MaxManifestBodyBytes), urls, u.PublicKey)
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

// ApplyUpdate downloads, verifies, and atomically installs. dryRun verifies artifact without replacing binary.
func (u *Updater) ApplyUpdate(ctx context.Context, plan *UpdatePlan, dryRun bool) error {
	if plan == nil {
		return fmt.Errorf("missing update plan")
	}
	lock, err := AcquireUpdateLock(u.lockPath())
	if err != nil {
		return err
	}
	defer lock.Release()

	if err := ValidateArtifactFilename(plan.Artifact.Filename); err != nil {
		return err
	}
	if plan.Artifact.URL != "" {
		if err := ValidateHTTPSURL(plan.Artifact.URL); err != nil {
			return err
		}
	}

	tmpDir, err := os.MkdirTemp(filepath.Dir(u.InstallOpts.TargetPath), "pve-zfs-monitor-dl-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmpDir)

	staged := filepath.Join(tmpDir, plan.Artifact.Filename)
	if err := DownloadArtifact(ctx, u.httpClient(MaxArtifactBodyBytes), plan.Artifact, staged); err != nil {
		return err
	}
	if err := VerifyArtifactFile(staged, plan.Artifact); err != nil {
		return err
	}
	if dryRun {
		return nil
	}
	_, err = atomicInstallUnlocked(staged, u.InstallOpts)
	return err
}

// InstallReleaseRequest describes a verified local install/bootstrap.
type InstallReleaseRequest struct {
	ManifestPath          string
	SignaturePath         string
	ArtifactPath          string
	PublicKey             ed25519.PublicKey
	Opts                  InstallOptions
	ReleaseBase           string
	Channel               Channel
	ExpectedVersion       string
	SourceTrustKeyFile    string
	AllowTrustKeyRotation bool
}

// InstallRelease verifies a local bundle, installs atomically, and persists release configuration.
func InstallRelease(req InstallReleaseRequest) error {
	if _, _, err := VerifyLocalBundle(LocalBundleVerifyOptions{
		ManifestPath:    req.ManifestPath,
		SignaturePath:   req.SignaturePath,
		ArtifactPath:    req.ArtifactPath,
		PublicKey:       req.PublicKey,
		ExpectedChannel: req.Channel,
		ExpectedVersion: req.ExpectedVersion,
	}); err != nil {
		return err
	}
	lock, err := AcquireUpdateLock(installLockPath())
	if err != nil {
		return err
	}
	defer lock.Release()

	if err := validatePersistedTrustAnchor(req); err != nil {
		return err
	}

	var snap *persistedConfigSnapshot
	if req.ReleaseBase != "" && req.SourceTrustKeyFile != "" && !shouldSkipConfigRewrite(req) {
		snap, err = backupPersistedReleaseConfig()
		if err != nil {
			return err
		}
		if err := WritePersistedConfig(PersistedConfig{
			ReleaseBase: stringsTrimRight(req.ReleaseBase),
			Channel:     req.Channel,
		}, req.SourceTrustKeyFile); err != nil {
			_ = snap.Restore()
			return err
		}
	}
	activated, err := atomicInstallUnlocked(req.ArtifactPath, req.Opts)
	if err != nil {
		if !activated && snap != nil {
			_ = snap.Restore()
		}
		return err
	}
	return nil
}

func stringsTrimRight(s string) string {
	for len(s) > 0 && s[len(s)-1] == '/' {
		s = s[:len(s)-1]
	}
	return s
}

func (u *Updater) httpClient(max int64) HTTPClient {
	if u.Client != nil {
		return u.Client
	}
	return boundedClient{u.Secure, max}
}

// InstallFromPaths verifies a local manifest/signature and staged binary.
func InstallFromPaths(manifestPath, sigPath, binaryPath string, pub ed25519.PublicKey, opts InstallOptions) error {
	_, _, err := VerifyLocalBundle(LocalBundleVerifyOptions{
		ManifestPath:  manifestPath,
		SignaturePath: sigPath,
		ArtifactPath:  binaryPath,
		PublicKey:     pub,
	})
	if err != nil {
		return err
	}
	return AtomicInstall(binaryPath, opts)
}
