package cmd

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"
)

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

func TestJSONFormatSkipsHomebrewCheck(t *testing.T) {
	cmd := &cobra.Command{}
	cmd.Flags().StringP("format", "f", "export", "")
	if isJSONFormat(cmd) {
		t.Fatal("export format should allow the update check")
	}
	if err := cmd.Flags().Set("format", "json"); err != nil {
		t.Fatal(err)
	}
	if !isJSONFormat(cmd) {
		t.Fatal("JSON format should skip the update check")
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
