package creds

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/bitfield/script"
)

const defaultRefreshTrigger = 5 * time.Minute

type Credentials struct {
	AccessKeyId     string    `json:"AccessKeyId"`
	SecretAccessKey string    `json:"SecretAccessKey"`
	SessionToken    string    `json:"SessionToken"`
	Expiration      time.Time `json:"Expiration"`
	Version         int       `json:"Version"`
}

func (c Credentials) Print(format string) {
	switch format {
	case "json":
		jsonData, _ := json.MarshalIndent(c, "", "  ")
		fmt.Printf("%s\n", jsonData)
	case "export":
		switch getShell() {
		case "fish":
			fmt.Printf(" set -x AWS_ACCESS_KEY_ID \"%s\"; set -x AWS_SECRET_ACCESS_KEY \"%s\"; set -x AWS_SESSION_TOKEN \"%s\"\n", c.AccessKeyId, c.SecretAccessKey, c.SessionToken)
		default:
			fmt.Printf(" export AWS_ACCESS_KEY_ID=\"%s\" AWS_SECRET_ACCESS_KEY=\"%s\" AWS_SESSION_TOKEN=\"%s\"\n", c.AccessKeyId, c.SecretAccessKey, c.SessionToken)
		}
	default:
		fmt.Printf("format %s is unsupported...", format)
		os.Exit(1)
	}
}

func (c Credentials) Write(file string) error {
	if c == (Credentials{}) {
		return errors.New("cannot write empty credentials")
	}
	jsonData, _ := json.MarshalIndent(c, "", "  ")
	return os.WriteFile(file, jsonData, 0644)
}

func (c Credentials) SetEnv() error {
	if c == (Credentials{}) {
		return errors.New("cannot set empty credentials to env")
	}
	os.Setenv("AWS_ACCESS_KEY_ID", c.AccessKeyId)
	os.Setenv("AWS_SECRET_ACCESS_KEY", c.SecretAccessKey)
	os.Setenv("AWS_SESSION_TOKEN", c.SessionToken)
	return nil
}

func (c Credentials) needsRefresh() bool {
	if time.Now().Add(defaultRefreshTrigger).After(c.Expiration) {
		return true
	}
	return false
}

func getCredsFromFile(file string) (Credentials, error) {
	jsonFile, err := os.Open(file)
	if err != nil {
		return Credentials{}, err
	}
	defer jsonFile.Close()

	byteValue, err := io.ReadAll(jsonFile)
	if err != nil {
		return Credentials{}, err
	}

	var creds Credentials
	err = json.Unmarshal(byteValue, &creds)
	return creds, err
}

func refreshCredentials(role RoleData, file string) (Credentials, error) {
	creds, _ := getCredsFromFile(file)
	if creds.needsRefresh() {
		return getAndWriteCredentials(role, file)
	}
	return creds, nil
}

func getAndWriteCredentials(role RoleData, file string) (Credentials, error) {
	creds, err := getCredentials(role)
	if err != nil {
		return Credentials{}, err
	}
	log.Printf("writing credentials to %s (expiring in %s)\n", file, creds.Expiration.Sub(time.Now()).Round(time.Minute).String())
	creds.Write(file)
	return creds, err
}

// getCredentials dispatches to the IDC or Substrate credential path based on the
// engineer's configured credential source (set via qt configure --use-identitycenter
// or overridden per-session with USE_SUBSTRATE=true/false).
func getCredentials(role RoleData) (Credentials, error) {
	if useIDC() {
		return getIDCCredentials(role)
	}
	return getSubstrateCredentials(role)
}

func getIDCCredentials(role RoleData) (Credentials, error) {
	if role == (RoleData{}) {
		// No role specified: return Substrate account credentials (engineers expect 666642175330 as the default).
		return getMetronomeSSORoleCredentials(staticSpecialAccounts["substrate"], "engineersreadonly")
	}

	if role.SpecialAccount != "" {
		roleName := normalizeIDCRole(role.Role, "prod")
		accountID, err := lookupSpecialAccountID(role.SpecialAccount)
		if err != nil {
			return Credentials{}, err
		}
		return getIDCRoleCredentials(accountID, roleName, role.Role == "", role.SpecialAccount)
	}

	accountID, err := lookupServiceAccountID(role.Domain, role.Environment)
	if err != nil {
		return Credentials{}, err
	}
	roleName := normalizeIDCRole(role.Role, role.Environment)
	return getIDCRoleCredentials(accountID, roleName, role.Role == "", fmt.Sprintf("%s-%s", role.Environment, role.Domain))
}

// getIDCRoleCredentials fetches IDC credentials, automatically falling back to
// engineersreadonly if admin was the environment default and isn't available.
func getIDCRoleCredentials(accountID, roleName string, autoFallback bool, label string) (Credentials, error) {
	creds, err := getMetronomeSSORoleCredentials(accountID, roleName)
	if err == nil {
		return creds, nil
	}
	if autoFallback && roleName == "admin" && errors.Is(err, errPermissionSetNotAvailable) {
		log.Printf("[quikstrate] admin not available for %s, using engineersreadonly\n", label)
		if creds, err = getMetronomeSSORoleCredentials(accountID, "engineersreadonly"); err == nil {
			return creds, nil
		}
	}
	if errors.Is(err, errPermissionSetNotAvailable) {
		return Credentials{}, fmt.Errorf("no %q permission set for %s", roleName, label)
	}
	return Credentials{}, err
}

func getSubstrateCredentials(role RoleData) (Credentials, error) {
	if role == (RoleData{}) {
		return substrateBaseCredentials()
	}
	if role.SpecialAccount != "" {
		return substrateSpecialCredentials(role.SpecialAccount)
	}
	return substrateAssumeRole(role)
}

// idcRoleForEnvironment returns the default IAM Identity Center permission set name.
// Staging defaults to admin (engineers deploy there); prod defaults to read-only.
func idcRoleForEnvironment(environment string) string {
	if environment == "staging" {
		return "admin"
	}
	return "engineersreadonly"
}

// normalizeIDCRole converts old Substrate role names to IDC permission set names,
// and fills in the environment default when no role is specified.
// This ensures existing scripts that pass --role Administrator keep working.
func normalizeIDCRole(role, environment string) string {
	switch role {
	case "", "Auditor":
		return idcRoleForEnvironment(environment)
	case "Administrator":
		return "admin"
	default:
		// Assume it's already an IDC permission set name (admin, engineersreadonly, etc.)
		return role
	}
}

// ---- Substrate fallback helpers ----
//
// These are used when Metronome IAM Identity Center is unavailable, typically
// because the engineer hasn't joined sso-metronome-identitycenter yet.

func substrateBaseCredentials() (Credentials, error) {
	cmd := "substrate credentials --format json --force"
	log.Print("running: ", cmd)
	byteValue, err := script.NewPipe().WithStderr(os.Stderr).Exec(cmd).Bytes()
	if err != nil {
		return Credentials{}, err
	}
	var creds Credentials
	return creds, json.Unmarshal(byteValue, &creds)
}

func substrateAssumeRole(role RoleData) (Credentials, error) {
	// substrate assume-role requires base credentials in the environment.
	baseCreds, err := substrateBaseCredentials()
	if err != nil {
		return Credentials{}, err
	}
	baseCreds.SetEnv()

	subRole := substrateRoleName(role.Role, role.Environment)
	cmd := fmt.Sprintf("substrate assume-role --environment %s --domain %s --quality %s --role %s --format json",
		role.Environment, role.Domain, role.Quality, subRole)
	log.Print("running: ", cmd)
	byteValue, err := script.NewPipe().WithStderr(os.Stderr).Exec(cmd).Bytes()
	if err != nil {
		return Credentials{}, err
	}
	var creds Credentials
	return creds, json.Unmarshal(byteValue, &creds)
}

func substrateSpecialCredentials(name string) (Credentials, error) {
	// substrate assume-role requires base credentials in the environment.
	baseCreds, err := substrateBaseCredentials()
	if err != nil {
		return Credentials{}, err
	}
	baseCreds.SetEnv()

	var cmd string
	if name == "management" {
		cmd = "substrate assume-role --management --format json"
	} else {
		cmd = fmt.Sprintf("substrate assume-role --special %s --format json", name)
	}
	log.Print("running: ", cmd)
	byteValue, err := script.NewPipe().WithStderr(os.Stderr).Exec(cmd).Bytes()
	if err != nil {
		return Credentials{}, err
	}
	var creds Credentials
	return creds, json.Unmarshal(byteValue, &creds)
}

// substrateRoleName translates an IDC permission set name (or empty string) to the
// equivalent Substrate role name for the fallback path.
func substrateRoleName(role, environment string) string {
	switch role {
	case "admin", "Administrator":
		return "Administrator"
	case "engineersreadonly", "Auditor", "":
		if environment == "staging" {
			return "Administrator"
		}
		return "Auditor"
	default:
		return role
	}
}

// defaultCredsFile returns the cache path for base (no-role) credentials.
// IDC and Substrate have separate files so switching sources via USE_SUBSTRATE
// or config always fetches fresh credentials from the right provider.
func defaultCredsFile() string {
	if useIDC() {
		return filepath.Join(CredsDir, "credentials-idc.json")
	}
	return DefaultCredsFile
}

func getDefaultCredentials() (Credentials, error) {
	return refreshCredentials(RoleData{}, defaultCredsFile())
}
