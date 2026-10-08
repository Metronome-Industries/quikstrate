package creds

import (
	"strings"
	"testing"
)

func TestUpdateManagedAWSConfigPreservesUnrelatedConfiguration(t *testing.T) {
	input := `# personal comment
[default]
output = json
credential_process = old-provider
region = eu-west-1

[profile staging-api]
role_arn = arn:aws:iam::123456789012:role/local
credential_process = old-provider
region = eu-west-1

[profile personal]
region = ap-southeast-2
`
	managed := []awsManagedSection{
		{header: "[default]", values: [][2]string{{"credential_process", "/bin/quikstrate credentials -f json"}, {"region", "us-west-2"}}},
		{header: "[profile staging-api]", values: [][2]string{{"credential_process", "/bin/quikstrate assume -e staging -d api -f json"}, {"region", "us-west-2"}}},
	}

	got := updateManagedAWSConfig(input, managed, true)
	for _, wanted := range []string{
		"# personal comment",
		"output = json",
		"role_arn = arn:aws:iam::123456789012:role/local",
		"[profile personal]\nregion = ap-southeast-2",
		awsManagedStart,
		"credential_process = /bin/quikstrate credentials -f json",
	} {
		if !strings.Contains(got, wanted) {
			t.Errorf("updated config lost %q:\n%s", wanted, got)
		}
	}
	if strings.Contains(got, "old-provider") || strings.Contains(got, "eu-west-1") {
		t.Errorf("updated config retained replaced values:\n%s", got)
	}
}

func TestUpdateManagedAWSConfigPrunesOnlyStaleManagedValues(t *testing.T) {
	input := `[profile old-api]
description = keep me
# BEGIN QUIKSTRATE MANAGED VALUES
credential_process = /bin/quikstrate assume -e old -d api -f json
region = us-west-2
# END QUIKSTRATE MANAGED VALUES

[profile personal]
credential_process = native-auth
region = eu-central-1
`

	got := updateManagedAWSConfig(input, nil, true)
	for _, wanted := range []string{"[profile old-api]", "description = keep me", "[profile personal]", "native-auth", "eu-central-1"} {
		if !strings.Contains(got, wanted) {
			t.Errorf("updated config lost %q:\n%s", wanted, got)
		}
	}
	for _, unwanted := range []string{awsManagedStart, "quikstrate assume", "region = us-west-2"} {
		if strings.Contains(got, unwanted) {
			t.Errorf("updated config retained %q:\n%s", unwanted, got)
		}
	}
}

func TestUpdateManagedAWSConfigMigratesLegacyQuikstrateValues(t *testing.T) {
	input := `[profile old-api]
credential_process = /usr/local/bin/quikstrate assume -e old -d api -f json
region = us-west-2

[sso-session stripe]
sso_start_url = https://example.com/start
sso_region = us-west-2
sso_registration_scopes = sso:account:access

[profile personal]
region = us-east-1
`

	got := updateManagedAWSConfig(input, nil, true)
	if strings.Contains(got, "[profile old-api]") || strings.Contains(got, "[sso-session stripe]") {
		t.Errorf("legacy quikstrate sections were not pruned:\n%s", got)
	}
	if !strings.Contains(got, "[profile personal]\nregion = us-east-1") {
		t.Errorf("unrelated profile was not preserved:\n%s", got)
	}
}

func TestUpdateManagedAWSConfigCanPreserveNativeAuthentication(t *testing.T) {
	input := `[default]
credential_process = native-auth
region = eu-west-1

[profile staging-api]
credential_process = native-auth --profile staging-api
role_arn = arn:aws:iam::123456789012:role/local
region = eu-west-1
`
	managed := []awsManagedSection{
		{header: "[default]", values: [][2]string{{"region", "us-west-2"}}},
		{header: "[profile staging-api]", values: [][2]string{{"region", "us-west-2"}}},
	}

	got := updateManagedAWSConfig(input, managed, true)
	for _, wanted := range []string{"credential_process = native-auth", "credential_process = native-auth --profile staging-api", "role_arn = arn:aws:iam::123456789012:role/local"} {
		if !strings.Contains(got, wanted) {
			t.Errorf("preserve-auth update lost %q:\n%s", wanted, got)
		}
	}
	if strings.Count(got, "region = us-west-2") != 2 {
		t.Errorf("preserve-auth update did not manage both regions:\n%s", got)
	}
}

func TestUpdateManagedAWSConfigIsIdempotent(t *testing.T) {
	managed := []awsManagedSection{
		{header: "[profile staging-api]", values: [][2]string{{"region", "us-west-2"}}},
	}
	first := updateManagedAWSConfig("[profile personal]\nregion = us-east-1\n", managed, true)
	second := updateManagedAWSConfig(first, managed, true)
	if first != second {
		t.Fatalf("managed config is not idempotent:\nfirst:\n%s\nsecond:\n%s", first, second)
	}
}

func TestUpdateManagedAWSConfigRecognizesInlineSectionComments(t *testing.T) {
	input := `[profile staging-api]
credential_process = /bin/quikstrate assume -e staging -d api -f json
region = us-west-2

[profile mainland-engineersreadonly]; this is a valid comment
sso_session    = SpaceCommander
sso_account_id = 030465607062
sso_role_name  = engineersreadonly
region         = us-west-2
output         = json
`
	managed := []awsManagedSection{
		{header: "[profile staging-api]", values: [][2]string{{"credential_process", "/bin/quikstrate assume -e staging -d api -f json"}, {"region", "us-west-2"}}},
	}

	got := updateManagedAWSConfig(input, managed, true)
	stripeProfile := `[profile mainland-engineersreadonly]; this is a valid comment
sso_session    = SpaceCommander
sso_account_id = 030465607062
sso_role_name  = engineersreadonly
region         = us-west-2
output         = json`
	if !strings.Contains(got, stripeProfile) {
		t.Errorf("profile with an inline section comment was modified:\n%s", got)
	}
	if strings.Index(got, awsManagedStart) > strings.Index(got, "[profile mainland-engineersreadonly]") {
		t.Errorf("managed values were moved into the following profile:\n%s", got)
	}
}

func TestUpdateManagedAWSConfigPreservesCommentOnManagedSection(t *testing.T) {
	input := `[profile staging-api] # configured for local development
credential_process = old-provider
region = eu-west-1
`
	managed := []awsManagedSection{
		{header: "[profile staging-api]", values: [][2]string{{"credential_process", "/bin/quikstrate assume -e staging -d api -f json"}, {"region", "us-west-2"}}},
	}

	got := updateManagedAWSConfig(input, managed, true)
	if !strings.Contains(got, "[profile staging-api] # configured for local development") {
		t.Errorf("managed section lost its inline comment:\n%s", got)
	}
	if strings.Count(got, "[profile staging-api]") != 1 {
		t.Errorf("managed section with an inline comment was duplicated:\n%s", got)
	}
}
