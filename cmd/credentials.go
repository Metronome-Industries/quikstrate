package cmd

import (
	"github.com/metronome-industries/quikstrate/internal/creds"
	"github.com/spf13/cobra"
)

var credentialsCmd = &cobra.Command{
	Use:   "credentials",
	Short: "Return cached credentials, if expired fetch and cache new ones",
	Long: `The quikstrate credentials command maps 1:1 to the substrate credentials command.
The only difference in usage is the "--force" flag, which will make quikstrate fetch and cache new credentials everytime.

It's recommended to add the following alias to your shell profile (eg. ~/.zshrc):
alias creds="eval \$(quikstrate credentials)"`,
	Run:    creds.CredentialsCmd,
	PreRun: creds.PreRunCmd,
}

func init() {
	credentialsCmd.Flags().StringP("format", "f", "export", "substrate environment")
	credentialsCmd.Flags().Bool("force", false, "always fetch new credentials")
	credentialsCmd.Flags().Bool("check", false, "check if credentials are expired, exit 0 if up-to-date otherwise exit 1`")
	credentialsCmd.MarkFlagsMutuallyExclusive("force", "check")
	rootCmd.AddCommand(credentialsCmd)
}
