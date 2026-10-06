package creds

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteSSOSessionConfigCreatesAWSConfigDirectory(t *testing.T) {
	previousAWSConfigFile := awsConfigFile
	previousDryrun := configDryrun
	t.Cleanup(func() {
		awsConfigFile = previousAWSConfigFile
		configDryrun = previousDryrun
	})

	awsConfigFile = filepath.Join(t.TempDir(), ".aws", "config")
	configDryrun = false

	if err := writeSSOSessionConfig("metronome", "https://example.com/start", "us-west-2"); err != nil {
		t.Fatalf("writeSSOSessionConfig() error = %v", err)
	}

	contents, err := os.ReadFile(awsConfigFile)
	if err != nil {
		t.Fatalf("reading created AWS config: %v", err)
	}
	if !strings.Contains(string(contents), "[sso-session metronome]") {
		t.Fatalf("AWS config does not contain SSO session block:\n%s", contents)
	}
}

func TestRemoveQuikstrateAWSAuthPreservesOtherAWSConfig(t *testing.T) {
	previousAWSConfigFile := awsConfigFile
	previousDryrun := configDryrun
	t.Cleanup(func() {
		awsConfigFile = previousAWSConfigFile
		configDryrun = previousDryrun
	})

	awsConfigFile = filepath.Join(t.TempDir(), "config")
	configDryrun = false
	contents := `[default]
credential_process = /usr/local/bin/quikstrate credentials -f json
region = us-west-2
[profile staging-api]
credential_process = /usr/local/bin/quikstrate assume -e staging -d api -f json
region = us-west-2
[profile native]
credential_process = /usr/local/bin/native-auth
[sso-session metronome]
sso_start_url = https://example.com/start
sso_region = us-west-2
[sso-session stripe]
sso_start_url = https://stripe.example.com/start
sso_region = us-west-2
`
	if err := os.WriteFile(awsConfigFile, []byte(contents), 0600); err != nil {
		t.Fatal(err)
	}

	if err := removeQuikstrateAWSAuth(); err != nil {
		t.Fatalf("removeQuikstrateAWSAuth() error = %v", err)
	}

	got, err := os.ReadFile(awsConfigFile)
	if err != nil {
		t.Fatal(err)
	}
	for _, unwanted := range []string{"quikstrate credentials", "quikstrate assume", "[sso-session metronome]", "[sso-session stripe]"} {
		if strings.Contains(string(got), unwanted) {
			t.Errorf("AWS config still contains %q:\n%s", unwanted, got)
		}
	}
	for _, wanted := range []string{"[profile staging-api]", "region = us-west-2", "credential_process = /usr/local/bin/native-auth"} {
		if !strings.Contains(string(got), wanted) {
			t.Errorf("AWS config lost %q:\n%s", wanted, got)
		}
	}
}

func TestPreserveAWSAuthEnabled(t *testing.T) {
	t.Run("disabled by default", func(t *testing.T) {
		t.Setenv("QUIKSTRATE_PRESERVE_AWS_AUTH", "")
		got, err := preserveAWSAuthEnabled(false)
		if err != nil || got {
			t.Fatalf("preserveAWSAuthEnabled(false) = %v, %v; want false, nil", got, err)
		}
	})

	t.Run("enabled by environment", func(t *testing.T) {
		t.Setenv("QUIKSTRATE_PRESERVE_AWS_AUTH", "true")
		got, err := preserveAWSAuthEnabled(false)
		if err != nil || !got {
			t.Fatalf("preserveAWSAuthEnabled(false) = %v, %v; want true, nil", got, err)
		}
	})

	t.Run("flag takes precedence", func(t *testing.T) {
		t.Setenv("QUIKSTRATE_PRESERVE_AWS_AUTH", "false")
		got, err := preserveAWSAuthEnabled(true)
		if err != nil || !got {
			t.Fatalf("preserveAWSAuthEnabled(true) = %v, %v; want true, nil", got, err)
		}
	})

	t.Run("invalid environment", func(t *testing.T) {
		t.Setenv("QUIKSTRATE_PRESERVE_AWS_AUTH", "sometimes")
		if _, err := preserveAWSAuthEnabled(false); err == nil {
			t.Fatal("preserveAWSAuthEnabled(false) error = nil; want invalid boolean error")
		}
	})
}
