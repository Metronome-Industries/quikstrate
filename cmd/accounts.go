package cmd

import (
	"github.com/metronome-industries/quikstrate/internal/creds"
	"github.com/spf13/cobra"
)

var accountsCmd = &cobra.Command{
	Use:   "accounts",
	Short: "Caches the results of the 'substrate accounts' command",
	Run:   creds.AccountsCmd,
}

func init() {
	accountsCmd.Flags().StringP("format", "f", "text", "output format")
	rootCmd.AddCommand(accountsCmd)
}
