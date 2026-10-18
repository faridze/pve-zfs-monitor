package release

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestLabBundleAssemblyAndOfflineVerify(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 required")
	}
	pub, priv := testSigningKeypair()
	dir := t.TempDir()
	ver := "1.0.0-rc1"
	bin := []byte{0x7f, 'E', 'L', 'F', 'l', 'a', 'b'}
	name := "pve-zfs-monitor-linux-amd64"
	_ = os.WriteFile(filepath.Join(dir, name), bin, 0o644)

	root := filepath.Clean(filepath.Join("..", ".."))
	asm := filepath.Join(root, "release", "signing", "assemble-manifest.py")
	out := filepath.Join(dir, "manifest.json")
	cmd := exec.Command("python3", asm,
		"--channel", "lab",
		"--version", ver,
		"--commit", "deadbeef",
		"--release-base", "https://releases.example.test",
		"--artifact-dir", dir,
		"--output", out,
	)
	if outb, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("assemble: %v\n%s", err, outb)
	}
	raw, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	sigPath := filepath.Join(dir, "manifest.json.sig.json")
	_ = os.WriteFile(sigPath, signManifestJSON(t, raw), 0o644)

	pubPath := filepath.Join(dir, "pub.b64")
	_ = os.WriteFile(pubPath, []byte(testPubB64), 0o644)

	if _, _, err := VerifyLocalBundle(LocalBundleVerifyOptions{
		ManifestPath:    out,
		SignaturePath:   sigPath,
		ArtifactPath:    filepath.Join(dir, name),
		PublicKey:       pub,
		ExpectedChannel: ChannelLab,
		ExpectedVersion: ver,
	}); err != nil {
		t.Fatal(err)
	}

	verify := filepath.Join(root, "release", "signing", "verify-bundle-offline.sh")
	vout, err := exec.Command("bash", verify, dir, pubPath).CombinedOutput()
	if err != nil {
		t.Fatalf("offline verify script: %v\n%s", err, vout)
	}
	_ = priv
	sum := sha256.Sum256(bin)
	m, err := ParseManifest(raw)
	if err != nil {
		t.Fatal(err)
	}
	if m.Channel != ChannelLab || m.Version != ver {
		t.Fatalf("unexpected manifest metadata: %+v", m)
	}
	if m.Artifacts[0].SHA256 != hex.EncodeToString(sum[:]) {
		t.Fatal("manifest sha mismatch")
	}
}

func TestStableAndLabManifestChannelsDiffer(t *testing.T) {
	lab := Manifest{SchemaVersion: 1, Version: "1.0.0", Channel: ChannelLab}
	stable := Manifest{SchemaVersion: 1, Version: "1.0.0", Channel: ChannelStable}
	lb, _ := json.Marshal(lab)
	sb, _ := json.Marshal(stable)
	var labDoc, stableDoc map[string]interface{}
	_ = json.Unmarshal(lb, &labDoc)
	_ = json.Unmarshal(sb, &stableDoc)
	if labDoc["channel"] == stableDoc["channel"] {
		t.Fatal("lab and stable manifests must differ by channel")
	}
}
