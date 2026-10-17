package release

import (
	"crypto/ed25519"
	"encoding/json"
	"fmt"
	"os"
	"runtime"
	"strings"
)

// LocalBundleVerifyOptions validates a downloaded release before any binary execution.
type LocalBundleVerifyOptions struct {
	ManifestPath    string
	SignaturePath   string
	ArtifactPath    string
	PublicKey       ed25519.PublicKey
	ExpectedChannel Channel
	ExpectedVersion string
	ExpectedGOOS    string
	ExpectedGOARCH  string
}

// VerifyLocalBundle authenticates manifest and artifact metadata without executing the artifact.
func VerifyLocalBundle(opts LocalBundleVerifyOptions) (*Manifest, *Artifact, error) {
	raw, err := os.ReadFile(opts.ManifestPath)
	if err != nil {
		return nil, nil, err
	}
	sigRaw, err := os.ReadFile(opts.SignaturePath)
	if err != nil {
		return nil, nil, err
	}
	var env SignatureEnvelope
	if err := json.Unmarshal(sigRaw, &env); err != nil {
		return nil, nil, fmt.Errorf("signature json: %w", err)
	}
	if len(opts.PublicKey) == 0 {
		return nil, nil, fmt.Errorf("missing public key")
	}
	if err := VerifyManifestSignature(raw, &env, opts.PublicKey); err != nil {
		return nil, nil, err
	}
	m, err := ParseManifest(raw)
	if err != nil {
		return nil, nil, err
	}
	if opts.ExpectedChannel != "" && m.Channel != opts.ExpectedChannel {
		return nil, nil, fmt.Errorf("manifest channel %q != expected %q", m.Channel, opts.ExpectedChannel)
	}
	if v := strings.TrimSpace(opts.ExpectedVersion); v != "" && m.Version != v {
		return nil, nil, fmt.Errorf("manifest version %q != expected %q", m.Version, v)
	}
	goos := opts.ExpectedGOOS
	if goos == "" {
		goos = runtime.GOOS
	}
	goarch := opts.ExpectedGOARCH
	if goarch == "" {
		goarch = runtime.GOARCH
	}
	art, err := m.SelectArtifact(goos, goarch)
	if err != nil {
		return nil, nil, err
	}
	if err := ValidateArtifactFilename(art.Filename); err != nil {
		return nil, nil, err
	}
	if err := VerifyArtifactFile(opts.ArtifactPath, *art); err != nil {
		return nil, nil, err
	}
	return m, art, nil
}
