package cmd

import (
	"github.com/metronome-industries/metstrate/internal/creds"
	"github.com/spf13/cobra"
)

var assumeCmd = &cobra.Command{
	Use:    "assume",
	Short:  "Assume a role",
	Run:    creds.AssumeCmd,
	PreRun: creds.PreRunCmd,
}

func init() {
	assumeCmd.Flags().StringP("env", "e", "", "substrate environment")
	assumeCmd.Flags().StringP("domain", "d", "", "substrate domain")
	assumeCmd.Flags().StringP("quality", "q", "", "substrate quality")
	assumeCmd.Flags().StringP("role", "r", "Administrator", "substrate role")
	assumeCmd.Flags().StringP("format", "f", "export", "substrate environment")
	assumeCmd.MarkFlagRequired("env")
	assumeCmd.MarkFlagRequired("domain")
	rootCmd.AddCommand(assumeCmd)
}
