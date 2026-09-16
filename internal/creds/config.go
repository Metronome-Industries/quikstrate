package creds

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

var quikstrateConfigFile = filepath.Join(home, ".quikstrate", "config.json")

type quikstrateConfig struct {
	CredentialSource     string `json:"credential_source,omitempty"`
	PreferredIDCInstance string `json:"preferred_idc_instance,omitempty"`
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

func preferredIDCInstance() string {
	if override := os.Getenv("QUIKSTRATE_IDC_INSTANCE"); override == "stripe" || override == "metronome" {
		return override
	}
	if readQuikstrateConfig().PreferredIDCInstance == "stripe" {
		return "stripe"
	}
	return "metronome"
}

// ConfigureIDCPreference updates only quikstrate's local configuration. AWS and Kubernetes configuration are unchanged.
func ConfigureIDCPreference(instance string) error {
	cfg := readQuikstrateConfig()
	if instance != "stripe" && instance != "metronome" {
		return fmt.Errorf("unknown IDC instance %q; use stripe or metronome", instance)
	}
	cfg.PreferredIDCInstance = instance
	return writeQuikstrateConfig(cfg)
}
