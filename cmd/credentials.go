package cmd

import (
	"github.com/hbowron/creds/internal/creds"
	"github.com/spf13/cobra"
)

var credentialsCmd = &cobra.Command{
	Use:    "credentials",
	Short:  "A brief description of your command",
	Run:    creds.CredentialsCmd,
	PreRun: creds.PreRunCmd,
}

func init() {
	credentialsCmd.Flags().StringP("format", "f", "export", "substrate environment")
	rootCmd.AddCommand(credentialsCmd)
}
