package cmd

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/metronome-industries/quikstrate/internal/creds"
	"github.com/spf13/cobra"
)

func TestRootCommandReportsHomebrewUpdate(t *testing.T) {
	oldCredsDir, oldTerminal, oldOutdated, oldStderr := creds.CredsDir, stderrIsTerminal, homebrewOutdated, os.Stderr
	defer func() {
		creds.CredsDir, stderrIsTerminal, homebrewOutdated, os.Stderr = oldCredsDir, oldTerminal, oldOutdated, oldStderr
	}()
	creds.CredsDir = t.TempDir()
	stderrIsTerminal = func() bool { return true }
	homebrewOutdated = func(context.Context) ([]byte, error) { return []byte("quikstrate\n"), nil }
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	os.Stderr = writer
	cmd := &cobra.Command{Use: "credentials"}
	cmd.SetContext(context.Background())
	cmd.Flags().StringP("format", "f", "export", "")
	rootCmd.PersistentPostRun(cmd, nil)
	writer.Close()
	output, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(output), "brew upgrade quikstrate") {
		t.Fatalf("missing update notice: %q", output)
	}
}

func TestHomebrewUpdateNoticeChecksOncePerDay(t *testing.T) {
	cacheFile := filepath.Join(t.TempDir(), "quikstrate", "homebrew-update-check")
	now := time.Now()
	var output bytes.Buffer
	calls := 0
	run := func(context.Context) ([]byte, error) {
		calls++
		return []byte("quikstrate\n"), nil
	}

	notifyHomebrewUpdate(context.Background(), cacheFile, now, &output, run)
	if calls != 1 || !strings.Contains(output.String(), "brew update && brew upgrade quikstrate") {
		t.Fatalf("first check: calls=%d, notice=%q", calls, output.String())
	}

	output.Reset()
	notifyHomebrewUpdate(context.Background(), cacheFile, now.Add(time.Hour), &output, run)
	if calls != 1 || output.Len() != 0 {
		t.Fatalf("cached check: calls=%d, notice=%q", calls, output.String())
	}

	if err := os.Chtimes(cacheFile, now.Add(-25*time.Hour), now.Add(-25*time.Hour)); err != nil {
		t.Fatal(err)
	}
	notifyHomebrewUpdate(context.Background(), cacheFile, now, &output, run)
	if calls != 2 || output.Len() == 0 {
		t.Fatalf("expired check: calls=%d, notice=%q", calls, output.String())
	}
}

func TestHomebrewCheckEligibility(t *testing.T) {
	for _, tc := range []struct {
		name        string
		command     string
		format      string
		interactive bool
		want        bool
	}{
		{name: "interactive command", command: "credentials", format: "export", interactive: true, want: true},
		{name: "JSON credentials", command: "credentials", format: "json", interactive: true},
		{name: "automated command", command: "credentials", format: "export"},
		{name: "clean command", command: "clean", interactive: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cmd := &cobra.Command{Use: tc.command}
			if tc.format != "" {
				cmd.Flags().StringP("format", "f", tc.format, "")
			}
			if got := shouldCheckHomebrewUpdate(cmd, tc.interactive); got != tc.want {
				t.Fatalf("shouldCheckHomebrewUpdate() = %t, want %t", got, tc.want)
			}
		})
	}
}

func TestConcurrentHomebrewChecksRunOnce(t *testing.T) {
	cacheFile := filepath.Join(t.TempDir(), "homebrew-update-check")
	now := time.Now()
	started := make(chan struct{})
	release := make(chan struct{})
	var wg sync.WaitGroup
	var output bytes.Buffer
	calls := 0
	run := func(context.Context) ([]byte, error) {
		calls++
		close(started)
		<-release
		return []byte("quikstrate\n"), nil
	}

	wg.Add(2)
	go func() {
		defer wg.Done()
		notifyHomebrewUpdate(context.Background(), cacheFile, now, &output, run)
	}()
	<-started
	go func() {
		defer wg.Done()
		notifyHomebrewUpdate(context.Background(), cacheFile, now, &output, run)
	}()
	close(release)
	wg.Wait()
	if calls != 1 || strings.Count(output.String(), "A newer Homebrew version") != 1 {
		t.Fatalf("concurrent checks: calls=%d, notice=%q", calls, output.String())
	}
}

func TestHomebrewCheckSkipsHeldLock(t *testing.T) {
	cacheFile := filepath.Join(t.TempDir(), "homebrew-update-check")
	lock, err := os.OpenFile(cacheFile+".lock", os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Close()
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX); err != nil {
		t.Fatal(err)
	}

	finished := make(chan struct{})
	go func() {
		defer close(finished)
		notifyHomebrewUpdate(context.Background(), cacheFile, time.Now(), io.Discard, func(context.Context) ([]byte, error) {
			return []byte("quikstrate\n"), nil
		})
	}()
	select {
	case <-finished:
	case <-time.After(200 * time.Millisecond):
		syscall.Flock(int(lock.Fd()), syscall.LOCK_UN)
		<-finished
		t.Fatal("update check waited for a held lock")
	}
}

func TestHomebrewUpdateNoticeIgnoresCurrentAndFailedChecks(t *testing.T) {
	for _, tc := range []struct {
		name   string
		result []byte
		err    error
	}{
		{name: "current formula", result: []byte("\n")},
		{name: "other formula", result: []byte("other-formula\n")},
		{name: "qualified formula", result: []byte("metronome-industries/metronome/quikstrate\n")},
		{name: "brew unavailable", err: errors.New("brew failed")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cacheFile := filepath.Join(t.TempDir(), "homebrew-update-check")
			var output bytes.Buffer
			notifyHomebrewUpdate(context.Background(), cacheFile, time.Now(), &output, func(context.Context) ([]byte, error) {
				return tc.result, tc.err
			})
			if tc.name == "qualified formula" {
				if output.Len() == 0 {
					t.Fatal("qualified formula should produce an update notice")
				}
				return
			}
			if output.Len() != 0 {
				t.Fatalf("unexpected notice: %q", output.String())
			}
		})
	}
}
