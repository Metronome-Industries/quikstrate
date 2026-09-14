package creds

import (
	"errors"
	"os/exec"
	"reflect"
	"strings"
	"testing"
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
	withConfigFile(t, quikstrateConfig{MetronomeIDCAccountIDs: &empty})
	if got := resolveCredentialSource(); got != credentialSourceIDC {
		t.Fatalf("default got %s", got)
	}
	if got := metronomeIDCAccountIDs(); len(got) != 0 {
		t.Fatalf("empty exception list was reseeded: %v", got)
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
	creds, err := getIDCRoleCredentials("407752757973", "admin", "staging-api")
	if err != nil || creds.AccessKeyId != "fallback" {
		t.Fatalf("fallback: %#v, %v", creds, err)
	}
	getIDCCredentialsForInstance = func(instance idcInstance, _, _ string) (Credentials, error) {
		return Credentials{}, errors.New(instance.Name + " failed")
	}
	_, err = getIDCRoleCredentials("407752757973", "admin", "staging-api")
	if err == nil || !strings.Contains(err.Error(), "stripe IDC") || !strings.Contains(err.Error(), "metronome IDC") {
		t.Fatalf("combined error missing context: %v", err)
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
