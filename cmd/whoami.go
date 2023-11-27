package cmd

import (
	"github.com/metronome-industries/quikstrate/internal/creds"
	"github.com/spf13/cobra"
)

var whoamiCmd = &cobra.Command{
	Use:   "whoami",
	Short: "Returns the current user",
	Long: `This simply merges the result of "aws sts get-caller-identity" with 
the accounts information from substrate.  If this is not returning what you expect,
double check your AWS_* environment variables.`,
	Run: creds.WhoamiCmd,
}

func init() {
	whoamiCmd.Flags().StringP("format", "f", "text", "output format")
	rootCmd.AddCommand(whoamiCmd)
}
