package release

import (
	"crypto/ed25519"
	"fmt"
	"os"
	"strings"
)

// TrustedPublicKeyB64 may be set at link time to pin the release signing public key.
var TrustedPublicKeyB64 string

// LoadPublicKeyFromFile reads a base64 ed25519 public key from path.
func LoadPublicKeyFromFile(path string) (ed25519.PublicKey, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return ParseEd25519PublicKeyBase64(strings.TrimSpace(string(data)))
}

// LoadTrustedPublicKey returns the configured ed25519 public key for release verification.
func LoadTrustedPublicKey() (ed25519.PublicKey, error) {
	if b64 := strings.TrimSpace(TrustedPublicKeyB64); b64 != "" {
		return ParseEd25519PublicKeyBase64(b64)
	}
	if cfg, err := LoadPersistedConfig(); err == nil {
		data, err := os.ReadFile(cfg.TrustKeyPath)
		if err != nil {
			return nil, err
		}
		return ParseEd25519PublicKeyBase64(strings.TrimSpace(string(data)))
	}
	if path := strings.TrimSpace(os.Getenv("PVE_ZFS_MONITOR_TRUST_KEY_FILE")); path != "" {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		return ParseEd25519PublicKeyBase64(strings.TrimSpace(string(data)))
	}
	return nil, fmt.Errorf("no trusted release signing public key configured")
}
