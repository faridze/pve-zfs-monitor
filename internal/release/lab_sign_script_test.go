package release

import (
	"crypto/ed25519"
	"crypto/x509"
	"encoding/pem"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestOfflineSignAndVerifyScriptsIntegration(t *testing.T) {
	if _, err := exec.LookPath("openssl"); err != nil {
		t.Skip("openssl required")
	}
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 required")
	}
	pub, priv := testSigningKeypair()
	dir := t.TempDir()
	bin := []byte{0x7f, 'E', 'L', 'F'}
	name := "pve-zfs-monitor-linux-amd64"
	_ = os.WriteFile(filepath.Join(dir, name), bin, 0o644)

	root := filepath.Clean(filepath.Join("..", ".."))
	manifestPath := filepath.Join(dir, "manifest.json")
	asm := filepath.Join(root, "release", "signing", "assemble-manifest.py")
	if out, err := exec.Command("python3", asm,
		"--channel", "lab",
		"--version", "1.0.0-rc1",
		"--commit", "cafebabe",
		"--release-base", "https://releases.example.test",
		"--artifact-dir", dir,
		"--output", manifestPath,
	).CombinedOutput(); err != nil {
		t.Fatalf("assemble: %v\n%s", err, out)
	}

	privPEM := filepath.Join(dir, "test-signing.pem")
	if err := os.WriteFile(privPEM, marshalPKCS8PEM(priv), 0o600); err != nil {
		t.Fatal(err)
	}
	pubPath := filepath.Join(dir, "trusted-signing-key.pub.b64")
	_ = os.WriteFile(pubPath, []byte(testPubB64), 0o644)

	signScript := filepath.Join(root, "release", "signing", "sign-manifest-offline.sh")
	cmd := exec.Command("bash", signScript, manifestPath)
	cmd.Env = append(os.Environ(), "PVE_ZFS_MONITOR_SIGNING_KEY_PEM="+privPEM)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("sign script: %v\n%s", err, out)
	}

	sigPath := manifestPath + ".sig.json"
	if _, err := os.Stat(sigPath); err != nil {
		t.Fatalf("expected %s, not manifest.sig.json: %v", sigPath, err)
	}
	if _, err := os.Stat(filepath.Join(dir, "manifest.sig.json")); err == nil {
		t.Fatal("incorrect manifest.sig.json must not be created")
	}

	verify := filepath.Join(root, "release", "signing", "verify-bundle-offline.sh")
	if out, err := exec.Command("bash", verify, dir, pubPath).CombinedOutput(); err != nil {
		t.Fatalf("verify script: %v\n%s", err, out)
	}
	_ = pub
}

func marshalPKCS8PEM(priv ed25519.PrivateKey) []byte {
	der, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		panic(err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})
}
