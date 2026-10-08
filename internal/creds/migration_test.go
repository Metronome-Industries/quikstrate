package creds

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

func withConfigFile(t *testing.T, cfg quikstrateConfig) {
	t.Helper()
	old := quikstrateConfigFile
	quikstrateConfigFile = t.TempDir() + "/config.json"
	t.Cleanup(func() { quikstrateConfigFile = old })
	if err := writeQuikstrateConfig(cfg); err != nil {
		t.Fatal(err)
	}
}

func TestCredentialSourcePrecedence(t *testing.T) {
	withConfigFile(t, quikstrateConfig{CredentialSource: string(credentialSourceSubstrate)})
	if got := resolveCredentialSource(); got != credentialSourceSubstrate {
		t.Fatalf("got %s", got)
	}
	t.Setenv("USE_SUBSTRATE", "true")
	if got := resolveCredentialSource(); got != credentialSourceSubstrate {
		t.Fatalf("environment override got %s", got)
	}
}

func TestIDCDefault(t *testing.T) {
	withConfigFile(t, quikstrateConfig{})
	if got := resolveCredentialSource(); got != credentialSourceIDC {
		t.Fatalf("default got %s", got)
	}
}

func TestConfigureIDCPreference(t *testing.T) {
	withConfigFile(t, quikstrateConfig{})
	for _, instance := range []string{"stripe", "metronome"} {
		if err := ConfigureIDCPreference(instance); err != nil {
			t.Fatalf("%s: %v", instance, err)
		}
		if readQuikstrateConfig().PreferredIDCInstance != instance {
			t.Fatalf("%s did not persist preference", instance)
		}
	}
	if err := ConfigureIDCPreference("invalid"); err == nil {
		t.Fatal("expected invalid IDC setting to fail")
	}
}

func TestNewAccounts(t *testing.T) {
	for account, want := range map[[2]string]string{
		{"awsmigration1", "staging"}: "850122837972",
		{"awsmigration2", "staging"}: "719535286314",
		{"network-test", "staging"}:  "566078794007",
	} {
		if got, err := lookupServiceAccountID(account[0], account[1]); err != nil || got != want {
			t.Fatalf("%s/%s: got %q, %v; want %q", account[1], account[0], got, err, want)
		}
	}
}

func TestServiceAccountDomains(t *testing.T) {
	want := []string{"api", "auth", "awsmigration1", "awsmigration2", "druid", "graphql", "ingest", "integrations", "internal-services", "lakehouse", "lambda", "marketplaces", "network-staging", "network-test", "notifications", "static-sites"}
	if !slices.Equal(Domains, want) {
		t.Fatalf("Domains = %v; want %v", Domains, want)
	}
	if slices.Contains(Domains, "admin") {
		t.Fatal("Domains includes account from environment outside EnvironmentMap")
	}
}

func TestPreferredIDCInstances(t *testing.T) {
	withConfigFile(t, quikstrateConfig{})
	first, second := preferredIDCInstances("407752757973")
	if first.Name != "metronome" || second.Name != "stripe" {
		t.Fatalf("unexpected order: %s, %s", first.Name, second.Name)
	}
	if err := ConfigureIDCPreference("stripe"); err != nil {
		t.Fatal(err)
	}
	first, second = preferredIDCInstances("407752757973")
	if first.Name != "stripe" || second.Name != "metronome" {
		t.Fatalf("unexpected order: %s, %s", first.Name, second.Name)
	}
	t.Setenv("QUIKSTRATE_IDC_INSTANCE", "metronome")
	first, _ = preferredIDCInstances("407752757973")
	if first.Name != "metronome" {
		t.Fatalf("environment override got %s", first.Name)
	}
}

func TestStripeAlternateRegion(t *testing.T) {
	withConfigFile(t, quikstrateConfig{PreferredIDCInstance: "stripe"})
	t.Setenv("SC_USE_ALTERNATE_REGION", "true")

	first, second := preferredIDCInstances("407752757973")
	if first != stripeAlternateIDC || second != metronomeIDC {
		t.Fatalf("unexpected alternate-region order: %#v, %#v", first, second)
	}
	name := RoleData{Environment: "staging", Domain: "api", Quality: "alpha", Role: "Administrator"}.GetFilename()
	if !strings.Contains(name, "-stripe-us-east-2-idc.json") {
		t.Fatalf("alternate-region cache name lacks region: %s", name)
	}
}

func TestStripeAlternateRegionDoesNotChangeMetronome(t *testing.T) {
	withConfigFile(t, quikstrateConfig{})
	t.Setenv("SC_USE_ALTERNATE_REGION", "true")

	first, second := preferredIDCInstances("407752757973")
	if first != metronomeIDC || second != stripeAlternateIDC {
		t.Fatalf("unexpected Metronome order during regional failover: %#v, %#v", first, second)
	}
}

func TestStripeAlternateRegionRejectsInvalidValue(t *testing.T) {
	t.Setenv("SC_USE_ALTERNATE_REGION", "sometimes")
	defer func() {
		if recover() == nil {
			t.Fatal("expected invalid SC_USE_ALTERNATE_REGION to panic")
		}
	}()
	activeStripeIDC()
}

func TestIDCFallbackAndCombinedError(t *testing.T) {
	withConfigFile(t, quikstrateConfig{PreferredIDCInstance: "stripe"})
	old := getIDCCredentialsForInstance
	t.Cleanup(func() { getIDCCredentialsForInstance = old })
	getIDCCredentialsForInstance = func(instance idcInstance, _, _ string) (Credentials, error) {
		if instance.Name == "stripe" {
			return Credentials{}, errors.New("stripe unavailable")
		}
		return Credentials{AccessKeyId: "fallback"}, nil
	}
	creds, instance, err := getIDCRoleCredentialsWithInstance("407752757973", "admin", "staging-api")
	if err != nil || creds.AccessKeyId != "fallback" || instance.Name != "metronome" {
		t.Fatalf("fallback: %#v, %s, %v", creds, instance.Name, err)
	}
	getIDCCredentialsForInstance = func(instance idcInstance, _, _ string) (Credentials, error) {
		return Credentials{}, errors.New(instance.Name + " failed")
	}
	_, err = getIDCRoleCredentials("407752757973", "admin", "staging-api")
	if err == nil || !strings.Contains(err.Error(), "stripe IDC") || !strings.Contains(err.Error(), "metronome IDC") {
		t.Fatalf("combined error missing context: %v", err)
	}
}

func TestCustomIDCRoleFallsBackToSTSAssumeRole(t *testing.T) {
	withConfigFile(t, quikstrateConfig{PreferredIDCInstance: "stripe"})
	oldGet := getIDCCredentialsForInstance
	oldAssume := assumeIAMRoleCredentials
	t.Cleanup(func() {
		getIDCCredentialsForInstance = oldGet
		assumeIAMRoleCredentials = oldAssume
	})

	getIDCCredentialsForInstance = func(instance idcInstance, accountID, roleName string) (Credentials, error) {
		if accountID == staticSpecialAccounts["substrate"] && roleName == idcRoleAdmin {
			return Credentials{AccessKeyId: "base-" + instance.Name}, nil
		}
		return Credentials{}, fmt.Errorf("%w: no %q permission set for account %s", errPermissionSetNotAvailable, roleName, accountID)
	}
	assumeIAMRoleCredentials = func(base Credentials, accountID, roleName string) (Credentials, error) {
		if base.AccessKeyId != "base-stripe" {
			t.Fatalf("base credentials = %q; want Stripe admin credentials", base.AccessKeyId)
		}
		if accountID != "008444403661" || roleName != "devbox-role-us-west-2" {
			t.Fatalf("assume target = %s/%s", accountID, roleName)
		}
		return Credentials{AccessKeyId: "assumed"}, nil
	}

	role := RoleData{Environment: "staging", Domain: "graphql", Quality: "alpha", Role: "devbox-role-us-west-2"}
	creds, instance, err := getIDCCredentialsWithInstance(role)
	if err != nil || creds.AccessKeyId != "assumed" || instance.Name != "stripe" {
		t.Fatalf("fallback: %#v, %s, %v", creds, instance.Name, err)
	}
}

func TestCustomSpecialRoleFallsBackToSTSAssumeRole(t *testing.T) {
	withConfigFile(t, quikstrateConfig{PreferredIDCInstance: "stripe"})
	oldGet := getIDCCredentialsForInstance
	oldAssume := assumeIAMRoleCredentials
	t.Cleanup(func() {
		getIDCCredentialsForInstance = oldGet
		assumeIAMRoleCredentials = oldAssume
	})

	getIDCCredentialsForInstance = func(instance idcInstance, accountID, roleName string) (Credentials, error) {
		if roleName == idcRoleAdmin {
			return Credentials{AccessKeyId: "base-" + instance.Name}, nil
		}
		return Credentials{}, fmt.Errorf("%w: no %q permission set for account %s", errPermissionSetNotAvailable, roleName, accountID)
	}
	assumeIAMRoleCredentials = func(base Credentials, accountID, roleName string) (Credentials, error) {
		if base.AccessKeyId != "base-stripe" {
			t.Fatalf("base credentials = %q; want Stripe admin credentials", base.AccessKeyId)
		}
		if accountID != staticSpecialAccounts["substrate"] || roleName != "devbox-role-us-west-2" {
			t.Fatalf("assume target = %s/%s", accountID, roleName)
		}
		return Credentials{AccessKeyId: "assumed-special"}, nil
	}

	role := RoleData{SpecialAccount: "substrate", Role: "devbox-role-us-west-2"}
	creds, instance, err := getIDCCredentialsWithInstance(role)
	if err != nil || creds.AccessKeyId != "assumed-special" || instance.Name != "stripe" {
		t.Fatalf("fallback: %#v, %s, %v", creds, instance.Name, err)
	}
}

func TestFallbackCacheUsesSuccessfulInstanceName(t *testing.T) {
	withConfigFile(t, quikstrateConfig{PreferredIDCInstance: "stripe"})
	oldGet := getIDCCredentialsForInstance
	oldCredsDir := CredsDir
	t.Cleanup(func() {
		getIDCCredentialsForInstance = oldGet
		CredsDir = oldCredsDir
	})
	CredsDir = t.TempDir()
	getIDCCredentialsForInstance = func(instance idcInstance, _, _ string) (Credentials, error) {
		if instance.Name == "stripe" {
			return Credentials{}, errors.New("not assigned")
		}
		return Credentials{AccessKeyId: "fallback", Expiration: time.Now().Add(time.Hour)}, nil
	}

	role := RoleData{Environment: "staging", Domain: "api", Quality: "alpha", Role: "Administrator"}
	if _, err := getAndWriteCredentials(role, role.GetFilename()); err != nil {
		t.Fatal(err)
	}
	metronomeCache := filepath.Join(CredsDir, "staging-api-alpha-admin-metronome-idc.json")
	if _, err := getCredsFromFile(metronomeCache); err != nil {
		t.Fatalf("fallback credentials not written to Metronome cache: %v", err)
	}
	stripeCache := filepath.Join(CredsDir, "staging-api-alpha-admin-stripe-idc.json")
	if _, err := getCredsFromFile(stripeCache); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("unexpected Stripe cache after Metronome fallback: %v", err)
	}
}

func TestIDCRolesAndCacheNames(t *testing.T) {
	withConfigFile(t, quikstrateConfig{PreferredIDCInstance: "stripe"})
	if role, err := normalizeIDCRole("Administrator"); err != nil || role != "admin" {
		t.Fatalf("administrator: %q, %v", role, err)
	}
	if _, err := normalizeIDCRole("Auditor"); err == nil || !strings.Contains(err.Error(), "engineersreadonly") {
		t.Fatalf("Auditor error: %v", err)
	}
	name := RoleData{Environment: "prod", Domain: "api", Quality: "gamma", Role: "Administrator"}.GetFilename()
	if !strings.Contains(name, "-stripe-idc.json") {
		t.Fatalf("cache name lacks instance: %s", name)
	}
}

func TestAWSCLIVersionPreflight(t *testing.T) {
	old := awsVersionOutput
	t.Cleanup(func() { awsVersionOutput = old })
	cases := []struct {
		output string
		err    error
		want   string
	}{
		{"", exec.ErrNotFound, "executable not found"},
		{"aws-cli/1.32.0 Python", nil, "could not parse AWS CLI v2 version"},
		{"aws-cli/2.8.9 Python", nil, "too old"},
		{"aws-cli/2.9.0 Python", nil, ""},
		{"aws-cli/2.16.3 Python", nil, ""},
		{"not aws", nil, "could not parse AWS CLI version"},
	}
	for _, tc := range cases {
		awsVersionOutput = func() (string, error) { return tc.output, tc.err }
		err := checkAWSCLIVersion()
		if tc.want == "" && err != nil {
			t.Fatalf("%q: %v", tc.output, err)
		}
		if tc.want != "" && (err == nil || !strings.Contains(err.Error(), tc.want)) {
			t.Fatalf("%q: got %v, want %q", tc.output, err, tc.want)
		}
	}
}
