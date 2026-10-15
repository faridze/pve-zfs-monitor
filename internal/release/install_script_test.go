package release

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInstallScriptNeverExecutesDownloadedBinary(t *testing.T) {
	root := filepath.Clean(filepath.Join("..", ".."))
	script, err := os.ReadFile(filepath.Join(root, "scripts", "install.sh"))
	if err != nil {
		t.Fatal(err)
	}
	body := string(script)
	if strings.Contains(body, "chmod +x \"$TMP/${ARTIFACT_NAME}\"") {
		t.Fatal("install.sh must not chmod+execute downloaded artifact before verification")
	}
	if strings.Contains(body, "\"$TMP/${ARTIFACT_NAME}\" install-release") {
		t.Fatal("install.sh must not execute downloaded artifact")
	}
	if !strings.Contains(body, "verify_release.py") {
		t.Fatal("install.sh must call verify_release.py before install")
	}
}
