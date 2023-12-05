package cmd

import (
	"github.com/metronome-industries/quikstrate/internal/creds"
	"github.com/spf13/cobra"
)

var checkCredentialsCmd = &cobra.Command{
	Use:   "check-credentials",
	Long: `Exit with a 0 if the cache is up-to-date, otherwise exit with a 1`,
	Run:    creds.CheckCredentialsCmd,
	PreRun: creds.PreRunCmd,
}

func init() {
	rootCmd.AddCommand(checkCredentialsCmd)
}
