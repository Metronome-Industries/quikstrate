package creds

import (
	"log"
	"os"

	"github.com/spf13/cobra"
)

func CredentialsCmd(cmd *cobra.Command, args []string) {
	format := cmd.Flag("format").Value.String()
	force := cmd.Flag("force").Value.String()
	check := cmd.Flag("check").Value.String()

	if check == "true" {
		checkCredentials()
	}

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

func checkCredentials() {
	creds, err := getCredsFromFile(DefaultCredsFile)
	if err != nil || creds.needsRefresh() {
		os.Exit(1)
	}
	os.Exit(0)
}
