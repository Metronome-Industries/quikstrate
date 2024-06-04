package cmd

import (
	"github.com/metronome-industries/quikstrate/internal/terraform"
	"github.com/spf13/cobra"
)

var driftCmd = &cobra.Command{
	Use:   "drift",
	Short: "Calculates terraform drift (run in metronome-substrate, or specify terraform directory with -p)",
	Run:   terraform.DriftCmd,
}

func init() {
	driftCmd.Flags().BoolP("verbose", "v", false, "verbose logging")
	driftCmd.Flags().String("terraform-version", terraform.TerraformVersion, "terraform version to use")
	driftCmd.Flags().StringP("path", "p", "", "path to terraform directory (defaults to \"<current git root>/root-modules\")")
	driftCmd.Flags().StringArrayP("match", "m", terraform.DefaultMatchPatterns, "optional filters to match module directories, eg. \"-m api -m prod\"")
	driftCmd.Flags().StringArrayP("skip", "s", terraform.DefaultSkipPatterns, "optional filters to skip module directories, eg. \"-s us-east-2\"")
	driftCmd.Flags().IntP("concurrency", "c", terraform.DefaultConcurrency, "the number of concurrent terraform processes")

	rootCmd.AddCommand(driftCmd)
}
