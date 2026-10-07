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
