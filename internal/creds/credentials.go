package creds

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
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
	var creds Credentials
	var err error
	if usingIDC() {
		var instance idcInstance
		creds, instance, err = getIDCCredentialsWithInstance(role)
		if err == nil {
			file = role.getIDCInstanceFilename(instance.Name)
		}
	} else {
		creds, err = getSubstrateCredentials(role)
	}
	if err != nil {
		return Credentials{}, err
	}
	log.Printf("writing credentials to %s (expiring in %s)\n", file, creds.Expiration.Sub(time.Now()).Round(time.Minute).String())
	creds.Write(file)
	return creds, err
}

func getIDCCredentialsWithInstance(role RoleData) (Credentials, idcInstance, error) {
	if role == (RoleData{}) {
		return getIDCRoleCredentialsWithInstance(staticSpecialAccounts["substrate"], idcRoleAdmin, "admin")
	}

	if role.SpecialAccount != "" {
		roleName, err := normalizeIDCRole(role.Role)
		if err != nil {
			return Credentials{}, idcInstance{}, err
		}
		accountID, err := lookupSpecialAccountID(role.SpecialAccount)
		if err != nil {
			return Credentials{}, idcInstance{}, err
		}
		return getIDCRoleCredentialsWithInstance(accountID, roleName, role.SpecialAccount)
	}

	accountID, err := lookupServiceAccountID(role.Domain, role.Environment)
	if err != nil {
		return Credentials{}, idcInstance{}, err
	}
	roleName, err := normalizeIDCRole(role.Role)
	if err != nil {
		return Credentials{}, idcInstance{}, err
	}
	return getIDCRoleCredentialsWithInstance(accountID, roleName, fmt.Sprintf("%s-%s", role.Environment, role.Domain))
}

func getIDCRoleCredentials(accountID, roleName, label string) (Credentials, error) {
	creds, _, err := getIDCRoleCredentialsWithInstance(accountID, roleName, label)
	return creds, err
}

func getIDCRoleCredentialsWithInstance(accountID, roleName, label string) (Credentials, idcInstance, error) {
	primary, secondary := preferredIDCInstances(accountID)
	creds, err := getIDCCredentialsForInstance(primary, accountID, roleName)
	if err == nil {
		return creds, primary, nil
	}
	log.Printf("%s IDC could not provide %q credentials for %s; trying %s IDC\n", primary.Name, roleName, label, secondary.Name)
	secondaryCreds, secondaryErr := getIDCCredentialsForInstance(secondary, accountID, roleName)
	if secondaryErr == nil {
		return secondaryCreds, secondary, nil
	}
	return Credentials{}, idcInstance{}, fmt.Errorf("getting %q credentials for %s from %s IDC: %w; %s IDC also failed: %v", roleName, label, primary.Name, err, secondary.Name, secondaryErr)
}

func preferredIDCInstances(_ string) (idcInstance, idcInstance) {
	stripe := activeStripeIDC()
	if preferredIDCInstance() == "metronome" {
		return metronomeIDC, stripe
	}
	return stripe, metronomeIDC
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

func normalizeIDCRole(role string) (string, error) {
	switch role {
	case substrateRoleReadOnly:
		return "", fmt.Errorf("Auditor is not an IAM Identity Center permission set; use --role %s (or prefix command with USE_SUBSTRATE=true)", idcRoleReadOnly)
	case substrateRoleAdmin:
		return idcRoleAdmin, nil
	default:
		return role, nil
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
	if usingIDC() {
		return RoleData{}.getIDCInstanceFilename(preferredIDCInstanceName(staticSpecialAccounts["substrate"]))
	}
	return DefaultCredsFile
}

func getDefaultCredentials() (Credentials, error) {
	return refreshCredentials(RoleData{}, defaultCredsFile())
}
