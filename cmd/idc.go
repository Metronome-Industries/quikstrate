package cmd

import (
	"github.com/metronome-industries/quikstrate/internal/creds"
	"github.com/spf13/cobra"
)

var idcCmd = &cobra.Command{
	Use:   "idc <stripe|metronome>",
	Short: "Configure the preferred Identity Center instance",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return creds.ConfigureIDCPreference(args[0])
	},
}

func init() {
	rootCmd.AddCommand(idcCmd)
}
