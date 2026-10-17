package release

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
)

// Manifest describes one published release and its per-architecture artifacts.
type Manifest struct {
	SchemaVersion int               `json:"schema_version"`
	Version       string            `json:"version"`
	Channel       Channel           `json:"channel"`
	Commit        string            `json:"commit"`
	PublishedAt   string            `json:"published_at"`
	Artifacts     []Artifact        `json:"artifacts"`
	Metadata      map[string]string `json:"metadata,omitempty"`
}

// Artifact is one downloadable binary for a platform.
type Artifact struct {
	Filename string `json:"filename"`
	GOOS     string `json:"goos"`
	GOARCH   string `json:"goarch"`
	SHA256   string `json:"sha256"`
	Size     int64  `json:"size"`
	URL      string `json:"url"`
}

// SignatureEnvelope carries detached signature metadata for a manifest.
type SignatureEnvelope struct {
	Algorithm   string `json:"algorithm"`
	PublicKeyID string `json:"public_key_id"`
	Signature   string `json:"signature"` // base64 raw ed25519
}

// ParseManifest decodes and minimally validates a release manifest.
func ParseManifest(data []byte) (*Manifest, error) {
	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("manifest json: %w", err)
	}
	if m.SchemaVersion != 1 {
		return nil, fmt.Errorf("unsupported manifest schema %d", m.SchemaVersion)
	}
	if strings.TrimSpace(m.Version) == "" {
		return nil, fmt.Errorf("manifest missing version")
	}
	if m.Channel != ChannelStable && m.Channel != ChannelLab {
		return nil, fmt.Errorf("invalid channel %q", m.Channel)
	}
	if len(m.Artifacts) == 0 {
		return nil, fmt.Errorf("manifest has no artifacts")
	}
	for i, a := range m.Artifacts {
		if a.Filename == "" || a.GOOS == "" || a.GOARCH == "" || a.SHA256 == "" || a.Size <= 0 {
			return nil, fmt.Errorf("artifact %d incomplete", i)
		}
		if err := ValidateArtifactFilename(a.Filename); err != nil {
			return nil, fmt.Errorf("artifact %d: %w", i, err)
		}
		if a.URL != "" {
			if err := ValidateHTTPSURL(a.URL); err != nil {
				return nil, fmt.Errorf("artifact %d url: %w", i, err)
			}
		}
	}
	return &m, nil
}

var safeArtifactName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._+-]*$`)

// ValidateArtifactFilename rejects path traversal and unsafe names.
func ValidateArtifactFilename(name string) error {
	if name == "" || name != filepath.Base(name) || strings.Contains(name, "..") {
		return fmt.Errorf("invalid artifact filename %q", name)
	}
	if !safeArtifactName.MatchString(name) {
		return fmt.Errorf("unsafe artifact filename %q", name)
	}
	return nil
}

// SelectArtifact picks the artifact matching goos/goarch.
func (m *Manifest) SelectArtifact(goos, goarch string) (*Artifact, error) {
	for _, a := range m.Artifacts {
		if a.GOOS == goos && a.GOARCH == goarch {
			return &a, nil
		}
	}
	return nil, fmt.Errorf("no artifact for %s/%s in manifest %s", goos, goarch, m.Version)
}
