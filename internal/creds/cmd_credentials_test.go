package creds

import (
	"strings"
	"testing"
	"time"
)

func TestCredentialsRoleData(t *testing.T) {
	for _, tc := range []struct {
		name, environment, role, wantRole string
		wantBase                          bool
	}{
		{name: "staging default", environment: "staging", wantRole: "staging-metdevops"},
		{name: "prod default", environment: "prod", wantRole: "prod-metdevops"},
		{name: "explicit role", environment: "prod", role: "custom-role", wantRole: "custom-role"},
		{name: "legacy admin", environment: "prod", role: "Admin", wantBase: true},
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
			if got.SpecialAccount != "substrate" || got.Role != tc.wantRole {
				t.Fatalf("role = %#v; want substrate account role %q", got, tc.wantRole)
			}
		})
	}

	if _, err := credentialsRoleData("unknown", ""); err == nil {
		t.Fatal("unknown environment should fail")
	}
}

func TestEnvironmentCredentialsUseSeparateIDCCache(t *testing.T) {
	withConfigFile(t, quikstrateConfig{})
	staging, _ := credentialsRoleData("staging", "")
	prod, _ := credentialsRoleData("prod", "")
	admin, _ := credentialsRoleData("staging", "Admin")
	stagingFile := staging.GetFilename()
	adminFile := credentialsCacheFile(admin)
	if adminFile != defaultCredsFile() {
		t.Fatalf("admin cache = %q; want original cache %q", adminFile, defaultCredsFile())
	}
	if stagingFile == prod.GetFilename() || stagingFile == adminFile || prod.GetFilename() == adminFile {
		t.Fatalf("credential caches overlap: %q, %q, %q", stagingFile, prod.GetFilename(), adminFile)
	}
	if !strings.Contains(stagingFile, "staging-metdevops") || !strings.Contains(prod.GetFilename(), "prod-metdevops") {
		t.Fatalf("credential caches omit role names: %q, %q", stagingFile, prod.GetFilename())
	}
}

func TestEnvironmentCredentialsRequestRoleInBaseAccount(t *testing.T) {
	withConfigFile(t, quikstrateConfig{})
	previous := getIDCCredentialsForInstance
	t.Cleanup(func() { getIDCCredentialsForInstance = previous })
	getIDCCredentialsForInstance = func(_ idcInstance, accountID, roleName string) (Credentials, error) {
		if accountID != staticSpecialAccounts["substrate"] || roleName != "prod-metdevops" {
			t.Fatalf("requested %s/%s; want substrate account/prod-metdevops", accountID, roleName)
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
		if gotBase.AccessKeyId != "base" || accountID != staticSpecialAccounts["substrate"] || roleName != "staging-metdevops" {
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
