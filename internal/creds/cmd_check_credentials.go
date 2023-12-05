package creds

import (
	"os"

	"github.com/spf13/cobra"
)

func CheckCredentialsCmd(cmd *cobra.Command, args []string) {
	creds, _ := getCredsFromFile(DefaultCredsFile)

	if creds.needsRefresh() {
		os.Exit(1)
	} else {
		os.Exit(0)
	}
}