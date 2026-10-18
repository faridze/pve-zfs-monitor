package release

import (
    "os"
    "os/exec"
    "path/filepath"
    "strings"
    "testing"
)

func TestLabBuildUsesAbsoluteActionsOutputDir(t *testing.T) {
    for _, name := range []string{"bash", "git", "python3", "tar"} {
        if _, err := exec.LookPath(name); err != nil {
            t.Skipf("%s required", name)
        }
    }
    root := filepath.Clean(filepath.Join("..", ".."))
    shaRaw, err := exec.Command("git", "-C", root, "rev-parse", "HEAD").Output()
    if err != nil {
        t.Fatalf("git HEAD: %v", err)
    }
    sha := strings.TrimSpace(string(shaRaw))

    // Mock only Go compilation; exercise real git archive and manifest assembly.
    fakeBin := filepath.Join(t.TempDir(), "fake-bin")
    if err := os.MkdirAll(fakeBin, 0700); err != nil {
        t.Fatal(err)
    }
    fakeGo := "#!/bin/sh\nset -eu\nwhile [ \"$#\" -gt 0 ]; do\n" +
        "  if [ \"$1\" = '-o' ]; then\n    shift\n" +
        "    printf 'fixture-%s\\n' \"$GOARCH\" > \"$1\"\n    exit 0\n  fi\n" +
        "  shift\ndone\nexit 2\n"
    if err := os.WriteFile(filepath.Join(fakeBin, "go"), []byte(fakeGo), 0700); err != nil {
        t.Fatal(err)
    }

    outputDir := filepath.Join(t.TempDir(), "absolute-build-output")
    version := "1.0.0-rc2"
    script := filepath.Join(root, "release", "signing", "build-lab-artifacts.sh")
    cmd := exec.Command("bash", script)
    cmd.Env = append(os.Environ(),
        "GIT_SHA="+sha,
        "VERSION="+version,
        "RELEASE_BASE=https://example.invalid/release",
        "OUT_DIR="+outputDir,
        "PATH="+fakeBin+":"+os.Getenv("PATH"),
    )
    if out, err := cmd.CombinedOutput(); err != nil {
        t.Fatalf("build with absolute OUT_DIR: %v\n%s", err, out)
    }
    dir := filepath.Join(outputDir, version)
    for _, f := range []string{
        "pve-zfs-monitor-linux-amd64",
        "pve-zfs-monitor-linux-arm64",
        "manifest.json",
    } {
        if info, err := os.Stat(filepath.Join(dir, f)); err != nil || info.Size() == 0 {
            t.Fatalf("missing/empty output %s at absolute OUT_DIR: %v", f, err)
        }
    }
}

func TestLabWorkflowUsesRunnerProvidedSigningKeyPath(t *testing.T) {
    root := filepath.Clean(filepath.Join("..", ".."))
    body, err := os.ReadFile(filepath.Join(root, ".github", "workflows", "lab-release.yml"))
    if err != nil {
        t.Fatal(err)
    }
    workflow := string(body)
    if strings.Contains(workflow, "env.SIGNING_KEY_PATH") {
        t.Fatal("GITHUB_ENV runner variables must not be referenced via expression env context")
    }
    if !strings.Contains(workflow, "export PVE_ZFS_MONITOR_SIGNING_KEY_PEM=\"$SIGNING_KEY_PATH\"") {
        t.Fatal("signing step must pass runner signing key path to local signer")
    }
}
