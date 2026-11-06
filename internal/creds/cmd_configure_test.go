package creds

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestConfigureAWSConfigUpdatesDefaultCredentialRole(t *testing.T) {
	withConfigFile(t, quikstrateConfig{})
	previousConfig, previousBinary, previousRegion := awsConfigFile, binaryPath, awsRegion
	previousDryrun, previousPreserve := configDryrun, configPreserveAWSAuth
	t.Cleanup(func() {
		awsConfigFile, binaryPath, awsRegion = previousConfig, previousBinary, previousRegion
		configDryrun, configPreserveAWSAuth = previousDryrun, previousPreserve
	})
	awsConfigFile = filepath.Join(t.TempDir(), "config")
	binaryPath = "/bin/quikstrate"
	awsRegion = "us-west-2"
	configDryrun = false
	configPreserveAWSAuth = false
	initial := `[default]
# BEGIN QUIKSTRATE MANAGED VALUES
credential_process = /bin/quikstrate credentials -f json
region = us-west-2
# END QUIKSTRATE MANAGED VALUES
`
	if err := os.WriteFile(awsConfigFile, []byte(initial), 0600); err != nil {
		t.Fatal(err)
	}

	if err := configureAWSConfig(nil, nil); err != nil {
		t.Fatal(err)
	}
	contents, err := os.ReadFile(awsConfigFile)
	if err != nil {
		t.Fatal(err)
	}
	want := "credential_process = /bin/quikstrate credentials -e staging -r staging-metdevops -f json"
	if !strings.Contains(string(contents), want) || strings.Contains(string(contents), "credential_process = /bin/quikstrate credentials -f json") {
		t.Fatalf("default credentials process was not updated:\n%s", contents)
	}
}
