/*
Copyright © 2023 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"os"

	"github.com/metronome-industries/quikstrate/internal/creds"
	"github.com/spf13/cobra"
)

var cleanCmd = &cobra.Command{
	Use:   "clean",
	Short: "Removes all credential files",
	Run: func(cmd *cobra.Command, args []string) {
		os.RemoveAll(creds.CredsDir)
	},
}

func init() {
	rootCmd.AddCommand(cleanCmd)
}
