package creds

import (
	"bytes"
	"log"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func setupAWSConfigureTest(t *testing.T) {
	t.Helper()
	withConfigFile(t, quikstrateConfig{})
	t.Setenv("USE_SUBSTRATE", "")
	previousFile, previousBinary, previousRegion := awsConfigFile, binaryPath, awsRegion
	previousDryrun, previousPreserve := configDryrun, configPreserveAWSAuth
	t.Cleanup(func() {
		awsConfigFile, binaryPath, awsRegion = previousFile, previousBinary, previousRegion
		configDryrun, configPreserveAWSAuth = previousDryrun, previousPreserve
	})
	awsConfigFile = filepath.Join(t.TempDir(), "config")
	binaryPath, awsRegion = "/bin/quikstrate", "eu-west-1"
	configDryrun, configPreserveAWSAuth = false, false
}

func readConfiguredAWSFile(t *testing.T) string {
	t.Helper()
	contents, err := os.ReadFile(awsConfigFile)
	if err != nil {
		t.Fatal(err)
	}
	return string(contents)
}

func TestConfigureAWSConfigReadonlyCompanions(t *testing.T) {
	setupAWSConfigureTest(t)
	if err := configureAWSConfig([]string{"prod", "staging"}, []string{"api"}); err != nil {
		t.Fatal(err)
	}
	contents := readConfiguredAWSFile(t)
	sections := parseAWSConfig(contents)
	commands := map[string]string{
		"prod-api":    "/bin/quikstrate assume -e prod -d api -f json",
		"staging-api": "/bin/quikstrate assume -e staging -d api -f json",
		"management":  "/bin/quikstrate assume --management -f json",
		"audit":       "/bin/quikstrate assume --special audit -f json",
		"deploy":      "/bin/quikstrate assume --special deploy -f json",
		"network":     "/bin/quikstrate assume --special network -f json",
	}
	for name, command := range commands {
		for _, readonly := range []bool{false, true} {
			profileName, expectedCommand := name, command
			if readonly {
				profileName += "-engineersreadonly"
				expectedCommand += " -r engineersreadonly"
			}
			index := findAWSSection(sections, "[profile "+profileName+"]")
			if index == -1 {
				t.Fatalf("missing profile %s", profileName)
			}
			values := make(map[string]string)
			for _, line := range sections[index].lines {
				if key, value, ok := awsConfigValue(line); ok {
					values[key] = value
				}
			}
			if values["credential_process"] != expectedCommand || values["region"] != "eu-west-1" {
				t.Errorf("profile %s: got %v; want command %q and region eu-west-1", profileName, values, expectedCommand)
			}
		}
	}
	if !strings.Contains(contents, "credential_process = /bin/quikstrate credentials -f json\n") {
		t.Fatal("default credential process changed")
	}
	if strings.Contains(contents, "[profile default-engineersreadonly]") || strings.Contains(contents, "prod-auth") {
		t.Fatal("generated profiles outside requested scope")
	}
	if err := checkAWSProfiles(contents, []string{"prod", "staging"}, []string{"api"}); err != nil {
		t.Fatal(err)
	}
}

func TestConfigureAWSConfigReadonlyReconciliation(t *testing.T) {
	setupAWSConfigureTest(t)
	personal := "[profile personal]\ncredential_process = native-auth\n# personal comment\nregion = us-east-1\n"
	if err := os.WriteFile(awsConfigFile, []byte(personal), 0600); err != nil {
		t.Fatal(err)
	}
	if err := configureAWSConfig([]string{"prod", "staging"}, []string{"api"}); err != nil {
		t.Fatal(err)
	}
	first := readConfiguredAWSFile(t)
	if err := configureAWSConfig([]string{"prod", "staging"}, []string{"api"}); err != nil {
		t.Fatal(err)
	}
	if second := readConfiguredAWSFile(t); first != second {
		t.Fatal("repeated configure changed the config")
	}
	if err := configureAWSConfig([]string{"staging"}, []string{"api"}); err != nil {
		t.Fatal(err)
	}
	updated := readConfiguredAWSFile(t)
	if strings.Contains(updated, "[profile prod-api") {
		t.Fatal("stale prod profiles were retained")
	}
	if !strings.Contains(updated, personal) || !strings.Contains(updated, "[profile staging-api-engineersreadonly]") {
		t.Fatal("lost unrelated config or requested readonly profile")
	}
}

func TestConfigureAWSConfigPreserveAuthSkipsReadonly(t *testing.T) {
	setupAWSConfigureTest(t)
	configPreserveAWSAuth = true
	if err := os.WriteFile(awsConfigFile, []byte("[profile prod-api]\ncredential_process = native-auth\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := configureAWSConfig([]string{"prod"}, []string{"api"}); err != nil {
		t.Fatal(err)
	}
	contents := readConfiguredAWSFile(t)
	if strings.Contains(contents, "engineersreadonly") || !strings.Contains(contents, "credential_process = native-auth") {
		t.Fatalf("unexpected preserve-auth config:\n%s", contents)
	}
	if err := checkAWSProfiles(contents, []string{"prod"}, []string{"api"}); err != nil {
		t.Fatal(err)
	}
}

func TestConfigureAWSConfigDryrunShowsReadonlyWithoutWriting(t *testing.T) {
	setupAWSConfigureTest(t)
	configDryrun = true
	var output bytes.Buffer
	previousOutput := log.Writer()
	log.SetOutput(&output)
	t.Cleanup(func() { log.SetOutput(previousOutput) })
	if err := configureAWSConfig([]string{"prod"}, []string{"api"}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(awsConfigFile); !os.IsNotExist(err) {
		t.Fatalf("dry-run wrote config: %v", err)
	}
	for _, expected := range []string{
		"[profile prod-api-engineersreadonly]",
		"credential_process = /bin/quikstrate assume -e prod -d api -f json -r engineersreadonly",
		"credential_process = /bin/quikstrate assume --management -f json -r engineersreadonly",
	} {
		if !strings.Contains(output.String(), expected) {
			t.Errorf("dry-run did not show %q", expected)
		}
	}
}

func TestCheckAWSProfilesRequiresReadonlyAndSpecialProfiles(t *testing.T) {
	setupAWSConfigureTest(t)
	if err := configureAWSConfig([]string{"prod"}, []string{"api"}); err != nil {
		t.Fatal(err)
	}
	contents := readConfiguredAWSFile(t)
	for _, missing := range []string{"prod-api-engineersreadonly", "management", "audit-engineersreadonly", "deploy-engineersreadonly", "network-engineersreadonly"} {
		header := "[profile " + missing + "]"
		modified := strings.Replace(contents, header, "[profile removed]", 1)
		if err := checkAWSProfiles(modified, []string{"prod"}, []string{"api"}); err == nil || !strings.Contains(err.Error(), header) {
			t.Errorf("missing %s: got %v", header, err)
		}
	}
	commented := strings.Replace(contents, "[profile prod-api-engineersreadonly]", " [profile prod-api-engineersreadonly] # local comment", 1)
	if err := checkAWSProfiles(commented, []string{"prod"}, []string{"api"}); err != nil {
		t.Fatalf("inline section comment failed validation: %v", err)
	}
	commentOnly := strings.Replace(contents, "[profile prod-api-engineersreadonly]", "# [profile prod-api-engineersreadonly]", 1)
	if err := checkAWSProfiles(commentOnly, []string{"prod"}, []string{"api"}); err == nil {
		t.Fatal("commented-out profile passed validation")
	}
}
