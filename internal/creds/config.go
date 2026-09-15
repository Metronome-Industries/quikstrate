package creds

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

var quikstrateConfigFile = filepath.Join(home, ".quikstrate", "config.json")

type quikstrateConfig struct {
	CredentialSource       string    `json:"credential_source,omitempty"`
	MetronomeIDCAccountIDs *[]string `json:"metronome_idc_account_ids,omitempty"`
	StripeIDCAccountIDs    *[]string `json:"stripe_idc_account_ids,omitempty"`
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
	if cfg.MetronomeIDCAccountIDs != nil && cfg.StripeIDCAccountIDs != nil {
		metronomeAccounts := make(map[string]bool)
		for _, id := range *cfg.MetronomeIDCAccountIDs {
			metronomeAccounts[id] = true
		}
		for _, id := range *cfg.StripeIDCAccountIDs {
			if metronomeAccounts[id] {
				return fmt.Errorf("account %s cannot be routed to both Metronome and Stripe IDC", id)
			}
		}
	}
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

func stripeIDCAccountIDs() []string {
	cfg := readQuikstrateConfig()
	if cfg.StripeIDCAccountIDs == nil {
		return []string{}
	}
	return *cfg.StripeIDCAccountIDs
}

// RouteIDC updates only the local IDC routing list. AWS and Kubernetes configuration are unchanged.
func RouteIDC(instance, selection string) error {
	if instance != "stripe" && instance != "metronome" {
		return fmt.Errorf("unknown IDC instance %q; use stripe or metronome", instance)
	}

	ids, err := accountIDsForCutover(selection)
	if err != nil {
		return err
	}

	cfg := readQuikstrateConfig()
	if cfg.MetronomeIDCAccountIDs == nil {
		allIDs := allAccountIDs()
		cfg.MetronomeIDCAccountIDs = &allIDs
	}
	if cfg.StripeIDCAccountIDs == nil {
		empty := []string{}
		cfg.StripeIDCAccountIDs = &empty
	}

	metronomeAccounts := make(map[string]bool)
	for _, id := range *cfg.MetronomeIDCAccountIDs {
		metronomeAccounts[id] = true
	}
	stripeAccounts := make(map[string]bool)
	for _, id := range *cfg.StripeIDCAccountIDs {
		stripeAccounts[id] = true
	}
	for _, id := range ids {
		metronomeAccounts[id] = instance == "metronome"
		stripeAccounts[id] = instance == "stripe"
	}

	updatedMetronome := make([]string, 0, len(metronomeAccounts))
	for id, included := range metronomeAccounts {
		if included {
			updatedMetronome = append(updatedMetronome, id)
		}
	}
	updatedStripe := make([]string, 0, len(stripeAccounts))
	for id, included := range stripeAccounts {
		if included {
			updatedStripe = append(updatedStripe, id)
		}
	}
	sort.Strings(updatedMetronome)
	sort.Strings(updatedStripe)
	cfg.MetronomeIDCAccountIDs = &updatedMetronome
	cfg.StripeIDCAccountIDs = &updatedStripe
	return writeQuikstrateConfig(cfg)
}
