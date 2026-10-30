package cmd

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"
)

const homebrewFormula = "metronome-industries/metronome/quikstrate"
const homebrewCheckInterval = 24 * time.Hour
const homebrewCheckTimeout = 3 * time.Second

func notifyHomebrewUpdate(ctx context.Context, cacheFile string, now time.Time, out io.Writer, run func(context.Context) ([]byte, error)) {
	if err := os.MkdirAll(filepath.Dir(cacheFile), 0700); err != nil {
		return
	}
	lock, err := os.OpenFile(cacheFile+".lock", os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return
	}
	defer lock.Close()
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX); err != nil {
		return
	}
	defer syscall.Flock(int(lock.Fd()), syscall.LOCK_UN)
	if info, err := os.Stat(cacheFile); err == nil && info.ModTime().After(now.Add(-homebrewCheckInterval)) {
		return
	}
	// Record attempts as well as successful checks so a broken brew installation
	// cannot slow down every interactive quikstrate command.
	if err := os.WriteFile(cacheFile, nil, 0600); err != nil {
		return
	}

	ctx, cancel := context.WithTimeout(ctx, homebrewCheckTimeout)
	defer cancel()
	result, err := run(ctx)
	if err != nil {
		return
	}
	for _, name := range strings.Split(string(result), "\n") {
		switch strings.TrimSpace(name) {
		case "quikstrate", homebrewFormula:
			fmt.Fprintln(out, "quikstrate: A newer Homebrew version is available. Run: brew update && brew upgrade quikstrate")
			return
		}
	}
}

func runHomebrewOutdated(ctx context.Context) ([]byte, error) {
	return exec.CommandContext(ctx, "brew", "outdated", "--formula", "--quiet", homebrewFormula).Output()
}

func isJSONFormat(cmd *cobra.Command) bool {
	flag := cmd.Flags().Lookup("format")
	return flag != nil && flag.Value.String() == "json"
}

func shouldCheckHomebrewUpdate(cmd *cobra.Command, interactive bool) bool {
	return interactive && cmd.Name() != "clean" && !isJSONFormat(cmd)
}
