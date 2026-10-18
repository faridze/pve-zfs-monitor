package release

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"strings"
)

const (
	SignatureAlgorithmEd25519 = "ed25519"
)

// VerifyManifestSignature checks a detached ed25519 signature over the raw manifest bytes.
func VerifyManifestSignature(manifestRaw []byte, env *SignatureEnvelope, publicKey ed25519.PublicKey) error {
	if env == nil {
		return fmt.Errorf("missing signature envelope")
	}
	if env.Algorithm != SignatureAlgorithmEd25519 {
		return fmt.Errorf("unsupported signature algorithm %q", env.Algorithm)
	}
	sig, err := base64.StdEncoding.DecodeString(env.Signature)
	if err != nil {
		return fmt.Errorf("signature base64: %w", err)
	}
	if len(publicKey) != ed25519.PublicKeySize {
		return fmt.Errorf("invalid public key length %d", len(publicKey))
	}
	if !ed25519.Verify(publicKey, manifestRaw, sig) {
		return fmt.Errorf("manifest signature verification failed")
	}
	return nil
}

// ParseEd25519PublicKeyBase64 decodes a standard base64-encoded ed25519 public key.
func ParseEd25519PublicKeyBase64(b64 string) (ed25519.PublicKey, error) {
	raw, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return nil, err
	}
	if len(raw) != ed25519.PublicKeySize {
		return nil, fmt.Errorf("expected %d byte public key, got %d", ed25519.PublicKeySize, len(raw))
	}
	return ed25519.PublicKey(raw), nil
}

// FileSHA256 computes hex-encoded SHA-256 of a file.
func FileSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// VerifyArtifactFile checks size and SHA-256 against manifest entry.
func VerifyArtifactFile(path string, art Artifact) error {
	st, err := os.Stat(path)
	if err != nil {
		return err
	}
	if st.Size() != art.Size {
		return fmt.Errorf("artifact size %d != expected %d", st.Size(), art.Size)
	}
	sum, err := FileSHA256(path)
	if err != nil {
		return err
	}
	if !strings.EqualFold(sum, art.SHA256) {
		return fmt.Errorf("artifact sha256 mismatch")
	}
	return nil
}
