package creds

import (
	"log"

	"github.com/spf13/cobra"
)

func CredentialsCmd(cmd *cobra.Command, args []string) {
	format := cmd.Flag("format").Value.String()
	force := cmd.Flag("force").Value.String()

	var creds Credentials
	var err error
	if force == "true" {
		creds, err = getAndWriteCredentials(RoleData{}, DefaultCredsFile)
	} else {
		creds, err = getDefaultCredentials()
	}
	if err != nil {
		log.Fatal(err)
	}
	creds.Print(format)
}

func getDefaultCredentials() (Credentials, error) {
	return refreshCredentials(RoleData{}, DefaultCredsFile)
}
