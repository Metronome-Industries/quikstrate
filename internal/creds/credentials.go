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

const (
	idcRoleAdmin          = "admin"
	idcRoleReadOnly       = "engineersreadonly"
	substrateRoleAdmin    = "Administrator"
	substrateRoleReadOnly = "Auditor"
)

type Credentials struct {
	AccessKeyId     string    `json:"AccessKeyId"`
	SecretAccessKey string    `json:"SecretAccessKey"`
	SessionToken    string    `json:"SessionToken"`
	Expiration      time.Time `json:"Expiration"`
	Version         int       `json:"Version"`
	IDCInstance     string    `json:"-"`
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
	if creds.IDCInstance != "" {
		file = role.idcFilename(creds.IDCInstance)
	}
	log.Printf("writing credentials to %s (expiring in %s)\n", file, creds.Expiration.Sub(time.Now()).Round(time.Minute).String())
	creds.Write(file)
	return creds, err
}

func getCredentials(role RoleData) (Credentials, error) {
	source, err := resolveCredentialSource()
	if err != nil {
		return Credentials{}, err
	}
	if source == credentialSourceIdentityCenter {
		return getIDCCredentials(role)
	}
	return getSubstrateCredentials(role)
}

func getIDCCredentials(role RoleData) (Credentials, error) {
	if role.Role == substrateRoleReadOnly {
		return Credentials{}, fmt.Errorf("role %q is a Substrate role and is not available through Identity Center; use --role %s", substrateRoleReadOnly, idcRoleReadOnly)
	}
	if role == (RoleData{}) {
		return getIDCRoleCredentials(staticSpecialAccounts["substrate"], idcRoleAdmin, "admin")
	}

	if role.SpecialAccount != "" {
		roleName := normalizeIDCRole(role.Role)
		accountID, err := lookupSpecialAccountID(role.SpecialAccount)
		if err != nil {
			return Credentials{}, err
		}
		return getIDCRoleCredentials(accountID, roleName, role.SpecialAccount)
	}

	accountID, err := lookupServiceAccountID(role.Domain, role.Environment)
	if err != nil {
		return Credentials{}, err
	}
	roleName := normalizeIDCRole(role.Role)
	return getIDCRoleCredentials(accountID, roleName, fmt.Sprintf("%s-%s", role.Environment, role.Domain))
}

func preferredIDCInstances(accountID string) ([]idcInstance, error) {
	if override := os.Getenv("QUIKSTRATE_IDC_INSTANCE"); override != "" {
		switch override {
		case metronomeIDC.Name:
			return []idcInstance{metronomeIDC}, nil
		case stripeIDC.Name:
			return []idcInstance{stripeIDC}, nil
		default:
			return nil, fmt.Errorf("invalid QUIKSTRATE_IDC_INSTANCE %q; expected metronome or stripe", override)
		}
	}
	for _, id := range metronomeIDCAccountIDs() {
		if id == accountID {
			return []idcInstance{metronomeIDC, stripeIDC}, nil
		}
	}
	return []idcInstance{stripeIDC, metronomeIDC}, nil
}

func getIDCRoleCredentials(accountID, roleName, label string) (Credentials, error) {
	instances, err := preferredIDCInstances(accountID)
	if err != nil {
		return Credentials{}, err
	}
	attemptErrors := make([]error, 0, len(instances))
	for _, instance := range instances {
		creds, err := idcCredentialProvider(instance, accountID, roleName)
		if err == nil {
			creds.IDCInstance = instance.Name
			return creds, nil
		}
		attemptErrors = append(attemptErrors, fmt.Errorf("%s Identity Center: %w", instance.Name, err))
	}
	return Credentials{}, fmt.Errorf("unable to get %q credentials for %s: %w", roleName, label, errors.Join(attemptErrors...))
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

// normalizeIDCRole converts the public Administrator default to the IDC name.
func normalizeIDCRole(role string) string {
	switch role {
	case substrateRoleAdmin:
		return idcRoleAdmin
	default:
		return role
	}
}

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
	baseCreds, err := refreshCredentials(RoleData{}, DefaultCredsFile)
	if err != nil {
		return Credentials{}, err
	}
	baseCreds.SetEnv()
	cmd := fmt.Sprintf("substrate assume-role --environment %s --domain %s --quality %s --role %s --format json",
		role.Environment, role.Domain, role.Quality, role.Role)
	log.Print("running: ", cmd)
	byteValue, err := script.NewPipe().WithStderr(os.Stderr).Exec(cmd).Bytes()
	if err != nil {
		return Credentials{}, err
	}
	var creds Credentials
	return creds, json.Unmarshal(byteValue, &creds)
}

func substrateSpecialCredentials(name string) (Credentials, error) {
	baseCreds, err := refreshCredentials(RoleData{}, DefaultCredsFile)
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

// defaultCredsFile returns separate cache paths for IDC vs Substrate so
// switching sources always fetches fresh credentials.
func defaultCredsFile() string {
	if useIDC() {
		instances, _ := preferredIDCInstances(staticSpecialAccounts["substrate"])
		instanceName := stripeIDC.Name
		if len(instances) > 0 {
			instanceName = instances[0].Name
		}
		return filepath.Join(CredsDir, "credentials-"+instanceName+"-idc.json")
	}
	return DefaultCredsFile
}

func getDefaultCredentials() (Credentials, error) {
	return refreshCredentials(RoleData{}, defaultCredsFile())
}
