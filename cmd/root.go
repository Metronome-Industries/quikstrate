package cmd

import (
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/metronome-industries/quikstrate/internal/creds"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

var rootCmd = &cobra.Command{
	Use:   "quikstrate -h",
	Short: "A substrate & identity center wrapper",
	CompletionOptions: cobra.CompletionOptions{
		DisableDefaultCmd: true,
	},
	PersistentPostRun: func(cmd *cobra.Command, _ []string) {
		if term.IsTerminal(int(os.Stderr.Fd())) && !isJSONFormat(cmd) {
			notifyHomebrewUpdate(cmd.Context(), filepath.Join(creds.CredsDir, "homebrew-update-check"), time.Now(), os.Stderr, runHomebrewOutdated)
		}
	},
}

func Execute() {
	err := rootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}

func init() {
	log.SetFlags(0)
}
