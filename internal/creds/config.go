package creds

import (
	"encoding/json"
	"os"
	"path/filepath"
)

var quikstrateConfigFile = filepath.Join(home, ".quikstrate", "config.json")

type quikstrateConfig struct {
	CredentialSource       string    `json:"credential_source,omitempty"`
	MetronomeIDCAccountIDs *[]string `json:"metronome_idc_account_ids,omitempty"`
}

type credentialSource string

const (
	credentialSourceIDC       credentialSource = "identitycenter"
	credentialSourceSubstrate credentialSource = "substrate"
)

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
	if err := os.MkdirAll(filepath.Dir(quikstrateConfigFile), 0700); err != nil {
		return err
	}
	return os.WriteFile(quikstrateConfigFile, data, 0644)
}

// resolveCredentialSource determines the source without silently falling back from IDC.
// Priority: USE_SUBSTRATE=true > persisted selection > Identity Center.
func resolveCredentialSource() credentialSource {
	if os.Getenv("USE_SUBSTRATE") == "true" {
		return credentialSourceSubstrate
	}
	if readQuikstrateConfig().CredentialSource == string(credentialSourceSubstrate) {
		return credentialSourceSubstrate
	}
	return credentialSourceIDC
}

func usingIDC() bool {
	return resolveCredentialSource() == credentialSourceIDC
}

func metronomeIDCAccountIDs() []string {
	cfg := readQuikstrateConfig()
	if cfg.MetronomeIDCAccountIDs == nil {
		return allAccountIDs()
	}
	return *cfg.MetronomeIDCAccountIDs
}
