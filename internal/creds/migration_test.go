package creds

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func withConfig(t *testing.T, contents string) {
	t.Helper()
	old := quikstrateConfigFile
	quikstrateConfigFile = filepath.Join(t.TempDir(), "config.json")
	t.Cleanup(func() { quikstrateConfigFile = old })
	if contents != "" {
		if err := os.WriteFile(quikstrateConfigFile, []byte(contents), 0600); err != nil {
			t.Fatal(err)
		}
	}
}

func TestCredentialSourcePrecedence(t *testing.T) {
	withConfig(t, `{"credential_source":"substrate"}`)
	t.Setenv("USE_SUBSTRATE", "")
	got, err := resolveCredentialSource()
	if err != nil || got != credentialSourceSubstrate {
		t.Fatalf("got %q, %v", got, err)
	}
	t.Setenv("USE_SUBSTRATE", "true")
	if got, _ = resolveCredentialSource(); got != credentialSourceSubstrate {
		t.Fatalf("got %q", got)
	}
	if err := os.Remove(quikstrateConfigFile); err != nil {
		t.Fatal(err)
	}
	t.Setenv("USE_SUBSTRATE", "")
	if got, _ = resolveCredentialSource(); got != credentialSourceIdentityCenter {
		t.Fatalf("default got %q", got)
	}
}

func TestMetronomeListMissingVersusEmpty(t *testing.T) {
	withConfig(t, `{}`)
	if len(seededQuikstrateConfig().MetronomeIDs()) != len(allAccountIDs()) {
		t.Fatal("missing list was not seeded")
	}
	if err := os.WriteFile(quikstrateConfigFile, []byte(`{"metronome_idc_account_ids":[]}`), 0600); err != nil {
		t.Fatal(err)
	}
	if got := seededQuikstrateConfig().MetronomeIDs(); len(got) != 0 {
		t.Fatalf("empty list reseeded: %v", got)
	}
}

func (c quikstrateConfig) MetronomeIDs() []string {
	if c.MetronomeIDCAccountIDs == nil {
		return nil
	}
	return *c.MetronomeIDCAccountIDs
}

func TestAccountSelectionMutation(t *testing.T) {
	ids := allAccountIDs()
	cfg := quikstrateConfig{MetronomeIDCAccountIDs: &ids}
	staging, _ := accountIDsForSelection("staging")
	if err := updateIDCAccountSelection(&cfg, "staging", false); err != nil {
		t.Fatal(err)
	}
	for _, moved := range staging {
		for _, remaining := range cfg.MetronomeIDs() {
			if moved == remaining {
				t.Fatalf("%s still present", moved)
			}
		}
	}
	if err := updateIDCAccountSelection(&cfg, staging[0], true); err != nil {
		t.Fatal(err)
	}
	if err := updateIDCAccountSelection(&cfg, staging[0], true); err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, id := range cfg.MetronomeIDs() {
		if id == staging[0] {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("rollback not idempotent: %v", cfg.MetronomeIDs())
	}
}

func TestPreferredInstanceAndOverride(t *testing.T) {
	id := staticServiceAccounts[[2]string{"api", "staging"}]
	withConfig(t, `{"metronome_idc_account_ids":["`+id+`"]}`)
	got, _ := preferredIDCInstances(id)
	if got[0].Name != "metronome" || got[1].Name != "stripe" {
		t.Fatalf("got %+v", got)
	}
	other := staticServiceAccounts[[2]string{"api", "prod"}]
	got, _ = preferredIDCInstances(other)
	if got[0].Name != "stripe" {
		t.Fatalf("got %+v", got)
	}
	t.Setenv("QUIKSTRATE_IDC_INSTANCE", "metronome")
	got, _ = preferredIDCInstances(other)
	if !reflect.DeepEqual(got, []idcInstance{metronomeIDC}) {
		t.Fatalf("got %+v", got)
	}
}

func TestIDCFallbackAndCombinedErrors(t *testing.T) {
	id := staticServiceAccounts[[2]string{"api", "prod"}]
	withConfig(t, `{"metronome_idc_account_ids":[]}`)
	old := idcCredentialProvider
	t.Cleanup(func() { idcCredentialProvider = old })
	var calls []string
	idcCredentialProvider = func(instance idcInstance, _, _ string) (Credentials, error) {
		calls = append(calls, instance.Name)
		if instance.Name == "stripe" {
			return Credentials{}, errors.New("missing assignment")
		}
		return Credentials{AccessKeyId: "ok"}, nil
	}
	creds, err := getIDCRoleCredentials(id, "admin", "prod-api")
	if err != nil || creds.AccessKeyId != "ok" || !reflect.DeepEqual(calls, []string{"stripe", "metronome"}) {
		t.Fatalf("%+v %v %v", creds, err, calls)
	}
	idcCredentialProvider = func(instance idcInstance, _, _ string) (Credentials, error) {
		return Credentials{}, errors.New(instance.Name + " failed")
	}
	_, err = getIDCRoleCredentials(id, "admin", "prod-api")
	if err == nil || !strings.Contains(err.Error(), "stripe failed") || !strings.Contains(err.Error(), "metronome failed") {
		t.Fatalf("combined error: %v", err)
	}
}

func TestRolesAndInstanceCache(t *testing.T) {
	withConfig(t, `{"metronome_idc_account_ids":[]}`)
	t.Setenv("USE_SUBSTRATE", "")
	if got := normalizeIDCRole("Administrator"); got != "admin" {
		t.Fatalf("got %q", got)
	}
	if got := normalizeIDCRole("Auditor"); got != "Auditor" {
		t.Fatalf("got %q", got)
	}
	oldDir := CredsDir
	CredsDir = t.TempDir()
	t.Cleanup(func() { CredsDir = oldDir })
	file := (RoleData{Environment: "prod", Domain: "api", Quality: "gamma", Role: "Administrator"}).GetFilename()
	if !strings.HasSuffix(file, "prod-api-gamma-admin-stripe-idc.json") {
		t.Fatalf("got %s", file)
	}
	old := idcCredentialProvider
	called := false
	idcCredentialProvider = func(idcInstance, string, string) (Credentials, error) { called = true; return Credentials{}, nil }
	t.Cleanup(func() { idcCredentialProvider = old })
	_, err := getIDCCredentials(RoleData{Environment: "prod", Domain: "api", Role: "Auditor"})
	if err == nil || !strings.Contains(err.Error(), "--role engineersreadonly") || called {
		t.Fatalf("err=%v called=%v", err, called)
	}
}
