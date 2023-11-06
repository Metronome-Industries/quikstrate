package creds

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/bitfield/script"
	"github.com/spf13/cobra"
)

var (
	home, _        = os.UserHomeDir()
	CredsDir       = filepath.Join(home, fmt.Sprintf("/.metronome/%s", binaryName))
	EnvironmentMap = map[string]Environment{
		"staging": {
			Name:           "staging",
			Aliases:        []string{"staging", "stg"},
			DefaultQuality: "alpha",
			DefaultRole:    "Administrator",
		},
		"prod": {
			Name:           "prod",
			Aliases:        []string{"production", "prod", "prd"},
			DefaultQuality: "gamma",
			DefaultRole:    "Auditor",
		},
	}
	Domains  = []string{"api", "auth", "druid", "graphql", "ingest", "lakehouse", "lambda", "marketplaces", "notifications", "static-sites"}
	Clusters = []ClusterSpec{
		{
			Name:   "graphql",
			Domain: "graphql",
		},
		{
			Name:   "rating",
			Domain: "ingest",
		},
	}
)

type ClusterSpec struct {
	Name   string
	Domain string
}

type Environment struct {
	Name           string
	Aliases        []string
	DefaultQuality string
	DefaultRole    string
}

type RoleData struct {
	Environment string
	Domain      string
	Quality     string
	Role        string
}

func (r RoleData) GetFilename() string {
	return filepath.Join(CredsDir, strings.ToLower(fmt.Sprintf("%s-%s-%s-%s.json", r.Environment, r.Domain, r.Quality, r.Role)))
}

func log(args ...any) {
	fmt.Fprintln(os.Stderr, args...)
}

func logf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format, args...)
}

func ensureAWSEnvSet() {
	if os.Getenv("AWS_ACCESS_KEY_ID") == "" || os.Getenv("AWS_SECRET_ACCESS_KEY") == "" || os.Getenv("AWS_SESSION_TOKEN") == "" {
		log("AWS credentials not set")
		os.Exit(1)
	}
}

func PreRunCmd(cmd *cobra.Command, args []string) {
	script.Exec(fmt.Sprintf("mkdir -p %s", CredsDir)).Wait()
}
