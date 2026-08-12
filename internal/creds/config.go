package creds

import (
	"encoding/json"
	"os"
	"path/filepath"
)

var quikstrateConfigFile = filepath.Join(home, ".quikstrate", "config.json")

type quikstrateConfig struct {
	CredentialSource string `json:"credential_source"`
}

func readQuikstrateConfig() quikstrateConfig {
	data, err := os.ReadFile(quikstrateConfigFile)
	if err != nil {
		return quikstrateConfig{}
	}
	var cfg quikstrateConfig
	json.Unmarshal(data, &cfg)
	return cfg
}

func writeQuikstrateConfig(cfg quikstrateConfig) error {
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(quikstrateConfigFile, data, 0644)
}

// useIDC reports whether quikstrate should use Metronome IAM Identity Center.
// Priority: USE_SUBSTRATE env var > ~/.quikstrate/config.json > default (Substrate).
func useIDC() bool {
	if os.Getenv("USE_SUBSTRATE") == "true" {
		return false
	}
	return readQuikstrateConfig().CredentialSource == "identitycenter"
}
