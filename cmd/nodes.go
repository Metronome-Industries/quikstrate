package cmd

import (
	"github.com/metronome-industries/quikstrate/internal/creds"
	"github.com/spf13/cobra"
)

var nodesCmd = &cobra.Command{
	Use: "nodes",
	Run: creds.NodesCmd,
}

func init() {
	rootCmd.AddCommand(nodesCmd)
}
