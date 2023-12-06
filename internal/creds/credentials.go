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

func getCredentials(role RoleData) (creds Credentials, err error) {
	var cmd string
	if (role == RoleData{}) {
		cmd = "substrate credentials -format json"
	} else {
		ensureAWSEnvSet()
		cmd = fmt.Sprintf("substrate assume-role -environment %s -domain %s -quality %s -role %s -format json", role.Environment, role.Domain, role.Quality, role.Role)
	}
	log.Print("running", cmd)
	byteValue, err := script.NewPipe().WithStderr(os.Stderr).Exec(cmd).Bytes()
	if err != nil {
		return
	}

	err = json.Unmarshal(byteValue, &creds)
	return
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
