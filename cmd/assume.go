package cmd

import (
	"github.com/metronome-industries/quikstrate/internal/creds"
	"github.com/spf13/cobra"
)

var assumeCmd = &cobra.Command{
	Use:   "assume",
	Short: "A stripped down version of the 'substrate assume-role' command.",
	Long: `This command uses the default credentials to fetch and cache role specific credentials.  This is used extensively in ~/.aws/config profiles (and 
kubectl through that).  The --env, --domain, --quality, and --role flags specify which credentials, and --format specifies the output.

Similarly to "quikstrate credentials", the --force flag will always fetch new credentials.

Note that role-specific credentials expire in 1 hour, not 12 hours like the default credentials. Just an FYI, nothing to worry about.`,
	Run:    creds.AssumeCmd,
	PreRun: creds.PreRunCmd,
}

func init() {
	assumeCmd.Flags().StringP("env", "e", "", "substrate environment")
	assumeCmd.Flags().StringP("domain", "d", "", "substrate domain")
	assumeCmd.Flags().StringP("quality", "q", "", "substrate quality")
	assumeCmd.Flags().StringP("role", "r", "Administrator", "substrate role")
	assumeCmd.Flags().StringP("format", "f", "export", "substrate environment")
	assumeCmd.Flags().Bool("force", false, "always fetch new credentials")
	assumeCmd.MarkFlagRequired("env")
	assumeCmd.MarkFlagRequired("domain")
	rootCmd.AddCommand(assumeCmd)
}
