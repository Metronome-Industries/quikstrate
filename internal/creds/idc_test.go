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
