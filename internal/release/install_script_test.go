package release

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInstallScriptSelfContainedAndSafe(t *testing.T) {
	root := filepath.Clean(filepath.Join("..", ".."))
	script, err := os.ReadFile(filepath.Join(root, "scripts", "install.sh"))
	if err != nil {
		t.Fatal(err)
	}
	body := string(script)
	for _, forbidden := range []string{
		"verify_release.py",
		"atomic-install.sh",
		"persist-release-config.sh",
		"chmod +x \"$TMP/${ARTIFACT_NAME}\"",
		"\"$TMP/${ARTIFACT_NAME}\" install-release",
		"install-release",
	} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("install.sh must not reference %q", forbidden)
		}
	}
	for _, required := range []string{
		"verify_bundle",
		"atomic_install_file",
		"persist_release_config",
		"flock",
		"--proto '=https'",
		"--proto-redir '=https'",
		"INSTALLER_SHA256",
	} {
		if !strings.Contains(body, required) {
			t.Fatalf("install.sh missing required %q", required)
		}
	}
}

func TestInstallScriptSyntax(t *testing.T) {
	root := filepath.Clean(filepath.Join("..", ".."))
	script := filepath.Join(root, "scripts", "install.sh")
	out, err := runBashSyntaxCheck(script)
	if err != nil {
		t.Fatalf("bash -n: %v\n%s", err, out)
	}
}
