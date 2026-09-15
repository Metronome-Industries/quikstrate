package creds

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
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

func TestIDCDefaultAndEmptyExceptionList(t *testing.T) {
	empty := []string{}
	withConfigFile(t, quikstrateConfig{MetronomeIDCAccountIDs: &empty, StripeIDCAccountIDs: &empty})
	if got := resolveCredentialSource(); got != credentialSourceIDC {
		t.Fatalf("default got %s", got)
	}
	if got := metronomeIDCAccountIDs(); len(got) != 0 {
		t.Fatalf("empty exception list was reseeded: %v", got)
	}
	if got := stripeIDCAccountIDs(); len(got) != 0 {
		t.Fatalf("unexpected explicit Stripe accounts: %v", got)
	}
}

func TestCutoverSelections(t *testing.T) {
	staging, err := accountIDsForCutover("staging")
	if err != nil || len(staging) == 0 {
		t.Fatalf("staging: %v, %v", staging, err)
	}
	admin, err := accountIDsForCutover("admin")
	if err != nil || !reflect.DeepEqual(admin, []string{"420073272039", "465454680116", "703712742941", "814412579886", "666642175330"}) {
		t.Fatalf("admin: %v, %v", admin, err)
	}
	if _, err := accountIDsForCutover("nope"); err == nil {
		t.Fatal("expected invalid cutover selection to fail")
	}
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

func TestRouteIDC(t *testing.T) {
	ids := []string{"407752757973"}
	withConfigFile(t, quikstrateConfig{MetronomeIDCAccountIDs: &ids})

	if err := RouteIDC("stripe", "407752757973"); err != nil {
		t.Fatal(err)
	}
	if got := metronomeIDCAccountIDs(); len(got) != 0 {
		t.Fatalf("Stripe route did not remove account: %v", got)
	}
	if got := stripeIDCAccountIDs(); !reflect.DeepEqual(got, ids) {
		t.Fatalf("Stripe route did not add account: %v", got)
	}

	if err := RouteIDC("metronome", "407752757973"); err != nil {
		t.Fatal(err)
	}
	if got := metronomeIDCAccountIDs(); !reflect.DeepEqual(got, ids) {
		t.Fatalf("Metronome route did not add account: %v", got)
	}
	if got := stripeIDCAccountIDs(); len(got) != 0 {
		t.Fatalf("Metronome route did not remove Stripe account: %v", got)
	}

	if err := RouteIDC("other", "staging"); err == nil {
		t.Fatal("expected invalid IDC instance to fail")
	}
}

func TestConfigRejectsAccountInBothIDCLists(t *testing.T) {
	ids := []string{"407752757973"}
	old := quikstrateConfigFile
	quikstrateConfigFile = t.TempDir() + "/config.json"
	t.Cleanup(func() { quikstrateConfigFile = old })
	if err := writeQuikstrateConfig(quikstrateConfig{
		MetronomeIDCAccountIDs: &ids,
		StripeIDCAccountIDs:    &ids,
	}); err == nil {
		t.Fatal("expected overlapping IDC routing lists to fail")
	}
}

func TestPreferredIDCInstances(t *testing.T) {
	ids := []string{"407752757973"}
	withConfigFile(t, quikstrateConfig{MetronomeIDCAccountIDs: &ids})
	first, second := preferredIDCInstances("407752757973")
	if first.Name != "metronome" || second.Name != "stripe" {
		t.Fatalf("unexpected order: %s, %s", first.Name, second.Name)
	}
	first, second = preferredIDCInstances("477056945755")
	if first.Name != "stripe" || second.Name != "metronome" {
		t.Fatalf("unexpected order: %s, %s", first.Name, second.Name)
	}
}

func TestStripeAlternateRegion(t *testing.T) {
	empty := []string{}
	withConfigFile(t, quikstrateConfig{MetronomeIDCAccountIDs: &empty})
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
	ids := []string{"407752757973"}
	withConfigFile(t, quikstrateConfig{MetronomeIDCAccountIDs: &ids})
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
	empty := []string{}
	withConfigFile(t, quikstrateConfig{MetronomeIDCAccountIDs: &empty})
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

func TestFallbackCacheUsesSuccessfulInstanceName(t *testing.T) {
	empty := []string{}
	withConfigFile(t, quikstrateConfig{MetronomeIDCAccountIDs: &empty})
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
	empty := []string{}
	withConfigFile(t, quikstrateConfig{MetronomeIDCAccountIDs: &empty})
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
