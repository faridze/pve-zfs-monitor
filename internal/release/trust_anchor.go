package release

import (
	"bytes"
	"errors"
	"os"
	"strings"
)

// ErrTrustKeyMismatch indicates the supplied trust key does not match the persisted anchor.
var ErrTrustKeyMismatch = errors.New("trust signing key does not match persisted anchor (see docs/p1d-lab-key-rotation.md)")

// ErrActivationDurability indicates the binary was activated but post-rename durability sync failed.
var ErrActivationDurability = errors.New("binary activated but durability sync failed")

// AllowTrustKeyRotation documents the explicit operator flag for key rotation.
const AllowTrustKeyRotationEnv = "PVE_ZFS_MONITOR_ALLOW_TRUST_KEY_ROTATION"

func trustKeyBase64Equal(pathA, pathB string) (bool, error) {
	a, err := readTrustKeyBase64(pathA)
	if err != nil {
		return false, err
	}
	b, err := readTrustKeyBase64(pathB)
	if err != nil {
		return false, err
	}
	pubA, err := ParseEd25519PublicKeyBase64(a)
	if err != nil {
		return false, err
	}
	pubB, err := ParseEd25519PublicKeyBase64(b)
	if err != nil {
		return false, err
	}
	return bytes.Equal(pubA, pubB), nil
}

func readTrustKeyBase64(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(data)), nil
}

func validatePersistedTrustAnchor(req InstallReleaseRequest) error {
	if req.SourceTrustKeyFile == "" {
		return nil
	}
	existing, err := LoadPersistedConfig()
	if err != nil {
		return nil
	}
	persistedKey := existing.TrustKeyPath
	if persistedKey == "" {
		return nil
	}
	same, err := trustKeyBase64Equal(persistedKey, req.SourceTrustKeyFile)
	if err != nil {
		return err
	}
	if same {
		return nil
	}
	if req.AllowTrustKeyRotation || strings.TrimSpace(os.Getenv(AllowTrustKeyRotationEnv)) == "1" {
		return nil
	}
	return ErrTrustKeyMismatch
}
