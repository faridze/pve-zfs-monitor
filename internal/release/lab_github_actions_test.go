package release

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestLabReleaseWorkflowIsDispatchOnlyOnMain(t *testing.T) {
	root := filepath.Clean(filepath.Join("..", ".."))
	data, err := os.ReadFile(filepath.Join(root, ".github", "workflows", "lab-release.yml"))
	if err != nil {
		t.Fatal(err)
	}
	body := string(data)
	if strings.Contains(body, "pull_request:") {
		t.Fatal("lab-release workflow must not run on pull_request")
	}
	if !strings.Contains(body, "workflow_dispatch:") {
		t.Fatal("lab-release workflow must be workflow_dispatch")
	}
	if !strings.Contains(body, "github.ref == 'refs/heads/main'") {
		t.Fatal("validate job must gate on main ref")
	}
	if !strings.Contains(body, "environment: lab-release-signing") {
		t.Fatal("publish job must use lab-release-signing environment")
	}
	if !strings.Contains(body, "secrets.PVE_ZFS_MONITOR_LAB_SIGNING_KEY_PEM") {
		t.Fatal("must read PVE_ZFS_MONITOR_LAB_SIGNING_KEY_PEM from environment secret")
	}
	if strings.Contains(body, "${{ secrets.GITHUB_TOKEN }}") && strings.Contains(body, "gh-pages") {
		t.Fatal("must not push gh-pages with GITHUB_TOKEN")
	}
}

func TestVerifyLabSigningKeyRejectsWrongFingerprint(t *testing.T) {
	if _, err := exec.LookPath("openssl"); err != nil {
		t.Skip("openssl required")
	}
	_, priv := testSigningKeypair()
	dir := t.TempDir()
	pemPath := filepath.Join(dir, "key.pem")
	if err := os.WriteFile(pemPath, marshalPKCS8PEM(priv), 0o600); err != nil {
		t.Fatal(err)
	}
	root := filepath.Clean(filepath.Join("..", ".."))
	script := filepath.Join(root, "release", "signing", "verify-lab-signing-key.sh")
	cmd := exec.Command("bash", script, pemPath)
	cmd.Env = append(os.Environ(), "EXPECTED_LAB_SIGNING_KEY_FINGERPRINT=sha256:0000000000000000000000000000000000000000000000000000000000000000")
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("expected fingerprint mismatch failure, out=%s", out)
	}
}

func TestSignManifestOfflineMissingKeyFailsClosed(t *testing.T) {
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("bash required")
	}
	dir := t.TempDir()
	manifest := filepath.Join(dir, "manifest.json")
	if err := os.WriteFile(manifest, []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	root := filepath.Clean(filepath.Join("..", ".."))
	script := filepath.Join(root, "release", "signing", "sign-manifest-offline.sh")
	cmd := exec.Command("bash", script, manifest)
	cmd.Env = append(os.Environ(), "PVE_ZFS_MONITOR_SIGNING_KEY_PEM=")
	_, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatal("expected failure when signing key path is empty")
	}
}
