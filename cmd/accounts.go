package cmd

import (
	"github.com/metronome-industries/quikstrate/internal/creds"
	"github.com/spf13/cobra"
)

var accountsCmd = &cobra.Command{
	Use:   "accounts",
	Short: "Caches and returns the results of the 'substrate accounts' command.",
	Long: `The quikstrate accounts default output is slightly different from substrate.  Extraneous information 
like the account email and Administrator role ARN are removed in favor of the gnome.house console URL and AWS_PROFILE snippet.

If other information would be helpful here we can surface it!`,
	Run: creds.AccountsCmd,
}

func init() {
	accountsCmd.Flags().StringP("format", "f", "text", "output format")
	rootCmd.AddCommand(accountsCmd)
}
