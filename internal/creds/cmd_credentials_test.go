package creds

import (
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"
)

func TestCredentialsRoleData(t *testing.T) {
	for _, tc := range []struct {
		name, environment, role, wantRole string
		wantBase                          bool
	}{
		{name: "staging default", environment: "staging", wantRole: "staging-metdevops"},
		{name: "prod default", environment: "prod", wantRole: "engineersreadonly"},
		{name: "explicit prod metdevops", environment: "prod", role: "prod-metdevops", wantRole: "prod-metdevops"},
		{name: "explicit role", environment: "prod", role: "custom-role", wantRole: "custom-role"},
		{name: "legacy admin", environment: "prod", role: "admin", wantBase: true},
		{name: "legacy Admin spelling", environment: "staging", role: "Admin", wantBase: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := credentialsRoleData(tc.environment, tc.role)
			if err != nil {
				t.Fatal(err)
			}
			if tc.wantBase {
				if got != (RoleData{}) {
					t.Fatalf("legacy admin role = %#v; want base credentials", got)
				}
				return
			}
			if got.SpecialAccount != "deploy" || got.Role != tc.wantRole {
				t.Fatalf("role = %#v; want deploy account role %q", got, tc.wantRole)
			}
		})
	}

	if _, err := credentialsRoleData("unknown", ""); err == nil {
		t.Fatal("unknown environment should fail")
	}
}

func TestSubstrateCredentialsWithoutRoleFlagsUseBaseCredentials(t *testing.T) {
	withConfigFile(t, quikstrateConfig{})
	t.Setenv("USE_SUBSTRATE", "true")
	for _, tc := range []struct {
		name string
		args []string
		want RoleData
	}{
		{name: "no role flags", want: RoleData{}},
		{name: "explicit default environment", args: []string{"-e", "staging"}, want: RoleData{SpecialAccount: "deploy", Role: "staging-metdevops"}},
		{name: "explicit role", args: []string{"-r", "prod-metdevops"}, want: RoleData{SpecialAccount: "deploy", Role: "prod-metdevops"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cmd := &cobra.Command{}
			cmd.Flags().StringP("environment", "e", "staging", "")
			cmd.Flags().StringP("role", "r", "", "")
			if err := cmd.ParseFlags(tc.args); err != nil {
				t.Fatal(err)
			}
			got, err := credentialsRoleDataFromCommand(cmd)
			if err != nil || got != tc.want {
				t.Fatalf("role = %#v, %v; want %#v", got, err, tc.want)
			}
			if tc.name == "no role flags" && credentialsCacheFile(got) != DefaultCredsFile {
				t.Fatalf("base credentials use cache %q; want %q", credentialsCacheFile(got), DefaultCredsFile)
			}
		})
	}
}

func TestIDCCredentialsWithoutRoleFlagsUseStagingDeployRole(t *testing.T) {
	withConfigFile(t, quikstrateConfig{})
	t.Setenv("USE_SUBSTRATE", "")
	cmd := &cobra.Command{}
	cmd.Flags().StringP("environment", "e", "staging", "")
	cmd.Flags().StringP("role", "r", "", "")
	got, err := credentialsRoleDataFromCommand(cmd)
	want := RoleData{SpecialAccount: "deploy", Role: "staging-metdevops"}
	if err != nil || got != want {
		t.Fatalf("role = %#v, %v; want %#v", got, err, want)
	}
}

func TestEnvironmentCredentialsUseSeparateIDCCache(t *testing.T) {
	withConfigFile(t, quikstrateConfig{})
	staging, _ := credentialsRoleData("staging", "")
	prod, _ := credentialsRoleData("prod", "")
	admin, _ := credentialsRoleData("staging", "admin")
	stagingFile := staging.GetFilename()
	adminFile := credentialsCacheFile(admin)
	if adminFile != defaultCredsFile() {
		t.Fatalf("admin cache = %q; want original cache %q", adminFile, defaultCredsFile())
	}
	if stagingFile == prod.GetFilename() || stagingFile == adminFile || prod.GetFilename() == adminFile {
		t.Fatalf("credential caches overlap: %q, %q, %q", stagingFile, prod.GetFilename(), adminFile)
	}
	if !strings.Contains(stagingFile, "staging-metdevops") || !strings.Contains(prod.GetFilename(), "engineersreadonly") {
		t.Fatalf("credential caches omit role names: %q, %q", stagingFile, prod.GetFilename())
	}
}

func TestEnvironmentCredentialsRequestRoleInDeployAccount(t *testing.T) {
	withConfigFile(t, quikstrateConfig{})
	previous := getIDCCredentialsForInstance
	t.Cleanup(func() { getIDCCredentialsForInstance = previous })
	getIDCCredentialsForInstance = func(_ idcInstance, accountID, roleName string) (Credentials, error) {
		if accountID != staticSpecialAccounts["deploy"] || roleName != "engineersreadonly" {
			t.Fatalf("requested %s/%s; want deploy account/engineersreadonly", accountID, roleName)
		}
		return Credentials{AccessKeyId: "environment"}, nil
	}
	role, _ := credentialsRoleData("prod", "")
	got, _, err := getIDCCredentialsWithInstance(role)
	if err != nil || got.AccessKeyId != "environment" {
		t.Fatalf("environment credentials = %#v, %v", got, err)
	}
}

func TestSubstrateEnvironmentCredentialsAssumeRoleFromBase(t *testing.T) {
	withConfigFile(t, quikstrateConfig{CredentialSource: string(credentialSourceSubstrate)})
	previousFile := DefaultCredsFile
	previousAssume := assumeIAMRoleCredentials
	t.Cleanup(func() {
		DefaultCredsFile = previousFile
		assumeIAMRoleCredentials = previousAssume
	})
	DefaultCredsFile = t.TempDir() + "/credentials.json"
	base := Credentials{AccessKeyId: "base", Expiration: time.Now().Add(time.Hour)}
	if err := base.Write(DefaultCredsFile); err != nil {
		t.Fatal(err)
	}
	assumeIAMRoleCredentials = func(gotBase Credentials, accountID, roleName string) (Credentials, error) {
		if gotBase.AccessKeyId != "base" || accountID != staticSpecialAccounts["deploy"] || roleName != "staging-metdevops" {
			t.Fatalf("assume inputs = %q, %q, %q", gotBase.AccessKeyId, accountID, roleName)
		}
		return Credentials{AccessKeyId: "assumed"}, nil
	}
	role, _ := credentialsRoleData("staging", "")
	got, err := getSubstrateCredentials(role)
	if err != nil || got.AccessKeyId != "assumed" {
		t.Fatalf("substrate credentials = %#v, %v", got, err)
	}
}
