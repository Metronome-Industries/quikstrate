package creds

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

var quikstrateConfigFile = filepath.Join(home, ".quikstrate", "config.json")

type credentialSource string

const (
	credentialSourceIdentityCenter credentialSource = "identitycenter"
	credentialSourceSubstrate      credentialSource = "substrate"
)

type quikstrateConfig struct {
	CredentialSource       string    `json:"credential_source,omitempty"`
	MetronomeIDCAccountIDs *[]string `json:"metronome_idc_account_ids,omitempty"`
}

func readQuikstrateConfig() quikstrateConfig {
	data, err := os.ReadFile(quikstrateConfigFile)
	if err != nil {
		return quikstrateConfig{}
	}
	var cfg quikstrateConfig
	_ = json.Unmarshal(data, &cfg)
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

// resolveCredentialSource applies the supported precedence: the temporary
// environment rollback, persisted configuration, then Identity Center.
func resolveCredentialSource() (credentialSource, error) {
	if os.Getenv("USE_SUBSTRATE") == "true" {
		return credentialSourceSubstrate, nil
	}
	source := readQuikstrateConfig().CredentialSource
	switch source {
	case "", string(credentialSourceIdentityCenter):
		return credentialSourceIdentityCenter, nil
	case string(credentialSourceSubstrate):
		return credentialSourceSubstrate, nil
	default:
		return "", fmt.Errorf("unsupported credential_source %q; expected identitycenter or substrate", source)
	}
}

func useIDC() bool {
	source, err := resolveCredentialSource()
	return err == nil && source == credentialSourceIdentityCenter
}

func seededQuikstrateConfig() quikstrateConfig {
	cfg := readQuikstrateConfig()
	if cfg.MetronomeIDCAccountIDs == nil {
		ids := allAccountIDs()
		cfg.MetronomeIDCAccountIDs = &ids
	}
	return cfg
}

func metronomeIDCAccountIDs() []string {
	cfg := readQuikstrateConfig()
	if cfg.MetronomeIDCAccountIDs == nil {
		return allAccountIDs()
	}
	return *cfg.MetronomeIDCAccountIDs
}
