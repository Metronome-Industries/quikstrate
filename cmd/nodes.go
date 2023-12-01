package cmd

import (
	"github.com/metronome-industries/quikstrate/internal/k8s"
	"github.com/spf13/cobra"
)

var nodesCmd = &cobra.Command{
	Use: "nodes",
	Run: k8s.NodesCmd,
}

func init() {
	nodesCmd.Flags().Bool("skip-daemon-sets", true, "skip daemonsets")
	nodesCmd.Flags().StringP("match", "m", "", "fuzzy match pod names")

	// driftCmd.Flags().Bool("skip-karpenter-nodes", true, "skip nodes dedicated to karpenter")
	rootCmd.AddCommand(nodesCmd)
}
