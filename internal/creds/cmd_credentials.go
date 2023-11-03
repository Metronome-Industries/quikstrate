package creds

import (
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
)

func CredentialsCmd(cmd *cobra.Command, args []string) {
	format := cmd.Flag("format").Value.String()
	creds, err := getDefaultCredentials()
	if err != nil {
		log(err)
		os.Exit(1)
	}
	creds.Print(format)
}

func getDefaultCredentials() (Credentials, error) {
	return refreshCredentials(RoleData{}, filepath.Join(CredsDir, "credentials.json"))
}
