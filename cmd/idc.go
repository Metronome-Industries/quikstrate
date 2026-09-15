package cmd

import (
	"github.com/metronome-industries/quikstrate/internal/creds"
	"github.com/spf13/cobra"
)

var idcCmd = &cobra.Command{
	Use:   "idc <stripe|metronome> <staging|prod|admin|account-id>",
	Short: "Route AWS accounts to an Identity Center instance",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		return creds.RouteIDC(args[0], args[1])
	},
}

func init() {
	rootCmd.AddCommand(idcCmd)
}
