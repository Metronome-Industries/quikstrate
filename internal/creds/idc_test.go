package creds

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateAWSCLI(t *testing.T) {
	oldLook, oldRun := lookPath, runAWSVersion
	t.Cleanup(func() { lookPath, runAWSVersion = oldLook, oldRun })
	lookPath = func(string) (string, error) { return "/bin/aws", nil }
	tests := []struct{ name, output, want string }{
		{"v1", "aws-cli/1.27.0 Python/3.9", "AWS CLI 1.27.0 is too old; Identity Center requires AWS CLI v2.9.0 or newer. Upgrade instructions: " + awsCLIUpgradeURL + " (temporary fallback: USE_SUBSTRATE=true)"},
		{"old v2", "aws-cli/2.8.9 Python/3.9", "AWS CLI 2.8.9 is too old; Identity Center requires AWS CLI v2.9.0 or newer. Upgrade instructions: " + awsCLIUpgradeURL + " (temporary fallback: USE_SUBSTRATE=true)"},
		{"minimum", "aws-cli/2.9.0 Python/3.9", ""},
		{"new", "aws-cli/2.20.1 Python/3.12", ""},
		{"unparsable", "not aws", "could not parse AWS CLI version from \"not aws\"; Identity Center requires AWS CLI v2.9.0 or newer"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runAWSVersion = func(string) ([]byte, error) { return []byte(tt.output), nil }
			err := validateAWSCLI()
			if tt.want == "" && err != nil {
				t.Fatal(err)
			}
			if tt.want != "" && (err == nil || err.Error() != tt.want) {
				t.Fatalf("got %v\nwant %s", err, tt.want)
			}
		})
	}
}

func TestValidateAWSCLIMissing(t *testing.T) {
	old := lookPath
	t.Cleanup(func() { lookPath = old })
	lookPath = func(string) (string, error) { return "", exec.ErrNotFound }
	want := "AWS CLI v2.9.0 or newer is required for Identity Center, but the aws executable was not found; install or upgrade it: " + awsCLIUpgradeURL + " (temporary fallback: USE_SUBSTRATE=true)"
	if err := validateAWSCLI(); err == nil || err.Error() != want {
		t.Fatalf("got %v", err)
	}
}

func TestRemoveSSOSessionPreservesOtherConfig(t *testing.T) {
	input := "[profile custom]\nregion = us-east-1\n[sso-session stripe]\nsso_start_url = old\n[sso-session other]\nsso_start_url = keep\n"
	got := removeSSOSession(input, "stripe")
	if strings.Contains(got, "sso_start_url = old") || !strings.Contains(got, "[profile custom]") || !strings.Contains(got, "[sso-session other]") {
		t.Fatalf("got:\n%s", got)
	}
}

func TestWriteBothSessionsReplaceInPlace(t *testing.T) {
	oldFile, oldDry := awsConfigFile, configDryrun
	awsConfigFile = filepath.Join(t.TempDir(), "config")
	configDryrun = false
	t.Cleanup(func() { awsConfigFile, configDryrun = oldFile, oldDry })
	if err := writeIDCSessionConfig(metronomeIDC); err != nil {
		t.Fatal(err)
	}
	if err := writeIDCSessionConfig(stripeIDC); err != nil {
		t.Fatal(err)
	}
	if err := writeIDCSessionConfig(stripeIDC); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(awsConfigFile)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(data), "[sso-session stripe]") != 1 || strings.Count(string(data), "[sso-session metronome]") != 1 {
		t.Fatalf("got:\n%s", data)
	}
}
