package release

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	defaultConfigDir  = "/etc/pve-zfs-monitor"
	defaultConfigFile = "release.conf"
	defaultTrustFile  = "trusted-signing-key.pub.b64"
)

// PersistedConfig is root-owned release trust settings (never taken from unsigned manifests).
type PersistedConfig struct {
	ReleaseBase  string  `json:"release_base"`
	Channel      Channel `json:"channel"`
	TrustKeyPath string  `json:"trust_key_path"`
}

// ConfigDir returns the configuration directory (overridable in tests).
func ConfigDir() string {
	if d := strings.TrimSpace(os.Getenv("PVE_ZFS_MONITOR_CONFIG_DIR")); d != "" {
		return d
	}
	return defaultConfigDir
}

func configFilePath() string {
	return filepath.Join(ConfigDir(), defaultConfigFile)
}

// LoadPersistedConfig reads validated release configuration from disk.
func LoadPersistedConfig() (*PersistedConfig, error) {
	path := configFilePath()
	if err := assertSecureConfigPath(path); err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cfg PersistedConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("release config: %w", err)
	}
	if err := validatePersistedConfig(&cfg); err != nil {
		return nil, err
	}
	if strings.TrimSpace(cfg.TrustKeyPath) == "" {
		return nil, fmt.Errorf("missing trust_key_path")
	}
	if err := assertSecureConfigPath(cfg.TrustKeyPath); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func validatePersistedConfig(cfg *PersistedConfig) error {
	if cfg == nil {
		return fmt.Errorf("missing config")
	}
	if err := ValidateHTTPSURL(cfg.ReleaseBase); err != nil {
		return fmt.Errorf("release_base: %w", err)
	}
	if cfg.Channel != ChannelStable && cfg.Channel != ChannelLab {
		return fmt.Errorf("invalid channel %q", cfg.Channel)
	}
	if cfg.TrustKeyPath != "" {
		if filepath.Clean(cfg.TrustKeyPath) != cfg.TrustKeyPath {
			return fmt.Errorf("invalid trust_key_path")
		}
	}
	return nil
}

// WritePersistedConfig stores release settings and copies the trust key into the config dir.
func WritePersistedConfig(cfg PersistedConfig, sourceTrustKeyFile string) error {
	if err := validatePersistedConfig(&cfg); err != nil {
		return err
	}
	if err := rejectSymlink(sourceTrustKeyFile); err != nil {
		return err
	}
	dir := ConfigDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	if err := assertSecureParentDir(dir); err != nil {
		return err
	}
	destKey := filepath.Join(dir, defaultTrustFile)
	if err := copyFile(sourceTrustKeyFile, destKey); err != nil {
		return err
	}
	if err := finalizeConfigFileMode(destKey); err != nil {
		return err
	}
	cfg.TrustKeyPath = destKey
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	tmp := filepath.Join(dir, ".release.conf.tmp")
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	if err := finalizeConfigFileMode(tmp); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, configFilePath()); err != nil {
		return err
	}
	return finalizeConfigFileMode(configFilePath())
}

// ResolveReleaseBase returns configured HTTPS release base URL.
func ResolveReleaseBase() (string, error) {
	if base := strings.TrimRight(strings.TrimSpace(DefaultReleaseBaseURL), "/"); base != "" {
		if err := ValidateHTTPSURL(base); err != nil {
			return "", err
		}
		return base, nil
	}
	if cfg, err := LoadPersistedConfig(); err == nil {
		return strings.TrimRight(cfg.ReleaseBase, "/"), nil
	}
	if base := strings.TrimRight(strings.TrimSpace(os.Getenv("PVE_ZFS_MONITOR_RELEASE_BASE")), "/"); base != "" {
		if err := ValidateHTTPSURL(base); err != nil {
			return "", err
		}
		return base, nil
	}
	return "", fmt.Errorf("release base URL not configured")
}
