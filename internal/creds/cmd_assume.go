package creds

import (
	"log"
	"os"

	"github.com/spf13/cobra"
)

func AssumeCmd(cmd *cobra.Command, args []string) {
	format := cmd.Flag("format").Value.String()
	force := cmd.Flag("force").Value.String()

	roleData, ok := NewRoleData(cmd.Flag("env").Value.String(), cmd.Flag("domain").Value.String(), cmd.Flag("quality").Value.String(), cmd.Flag("role").Value.String())
	if !ok {
		cmd.Usage()
		os.Exit(1)
	}

	defaultCreds, err := getDefaultCredentials()
	if err != nil {
		log.Fatal(err)
	}

	defaultCreds.SetEnv()

	var creds Credentials
	if force == "true" {
		creds, err = getAndWriteCredentials(roleData, roleData.GetFilename())
	} else {
		creds, err = refreshCredentials(roleData, roleData.GetFilename())
	}
	if err != nil {
		log.Fatal(err)
	}

	creds.Print(format)
}

func NewRoleData(environment, domain, quality, role string) (RoleData, bool) {
	if _, ok := EnvironmentMap[environment]; !ok {
		return RoleData{}, false
	}
	if quality == "" {
		quality = EnvironmentMap[environment].DefaultQuality
	}

	return RoleData{
		Environment: environment,
		Domain:      domain,
		Quality:     quality,
		Role:        role,
	}, true
}
