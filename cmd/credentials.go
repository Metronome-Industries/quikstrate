package cmd

import (
	"github.com/metronome-industries/quikstrate/internal/creds"
	"github.com/spf13/cobra"
)

var credentialsCmd = &cobra.Command{
	Use:   "credentials",
	Short: "Return cached credentials, if expired fetch and cache new ones",
	Long: `Fetch and cache environment-scoped credentials for the deploy account.
The default environment is staging. The default role is staging-metdevops for staging
and engineersreadonly for prod.
Use --role admin to fetch the original substrate account credentials. --force fetches new credentials every time.

When using Identity Center, authentication is direct to Identity Center instead of Substrate.

It's recommended to add the following alias to your shell profile (eg. ~/.zshrc):
alias creds="eval \$(quikstrate credentials)"`,
	Run:    creds.CredentialsCmd,
	PreRun: creds.PreRunCmd,
}

func init() {
	credentialsCmd.Flags().StringP("environment", "e", "staging", "credential environment (staging or prod)")
	credentialsCmd.Flags().StringP("role", "r", "", "credential role (default: staging-metdevops or prod engineersreadonly; admin for original behavior)")
	credentialsCmd.Flags().StringP("format", "f", "export", "substrate environment")
	credentialsCmd.Flags().Bool("force", false, "always fetch new credentials")
	credentialsCmd.Flags().Bool("check", false, "check if credentials are expired, exit 0 if up-to-date otherwise exit 1`")
	credentialsCmd.MarkFlagsMutuallyExclusive("force", "check")
	rootCmd.AddCommand(credentialsCmd)
}
