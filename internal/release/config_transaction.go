package release

import (
	"os"
	"path/filepath"
)

type persistedConfigSnapshot struct {
	configDir  string
	hadConfig  bool
	configData []byte
	hadKey     bool
	keyData    []byte
}

func backupPersistedReleaseConfig() (*persistedConfigSnapshot, error) {
	dir := ConfigDir()
	snap := &persistedConfigSnapshot{configDir: dir}
	confPath := filepath.Join(dir, defaultConfigFile)
	keyPath := filepath.Join(dir, defaultTrustFile)
	if data, err := os.ReadFile(confPath); err == nil {
		snap.hadConfig = true
		snap.configData = data
	}
	if data, err := os.ReadFile(keyPath); err == nil {
		snap.hadKey = true
		snap.keyData = data
	}
	return snap, nil
}

func (s *persistedConfigSnapshot) Restore() error {
	if s == nil {
		return nil
	}
	confPath := filepath.Join(s.configDir, defaultConfigFile)
	keyPath := filepath.Join(s.configDir, defaultTrustFile)
	if !s.hadConfig {
		_ = os.Remove(confPath)
	} else if err := os.WriteFile(confPath, s.configData, 0o644); err != nil {
		return err
	}
	if !s.hadKey {
		_ = os.Remove(keyPath)
	} else if err := os.WriteFile(keyPath, s.keyData, 0o644); err != nil {
		return err
	}
	return nil
}

func shouldSkipConfigRewrite(req InstallReleaseRequest) bool {
	if req.ReleaseBase == "" || req.SourceTrustKeyFile == "" {
		return true
	}
	existing, err := LoadPersistedConfig()
	if err != nil {
		return false
	}
	return existing.Channel == req.Channel &&
		stringsTrimRight(existing.ReleaseBase) == stringsTrimRight(req.ReleaseBase)
}
