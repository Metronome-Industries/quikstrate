package creds

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/bitfield/script"
	"github.com/mitchellh/go-ps"
	"github.com/spf13/cobra"
)

var (
	home, _          = os.UserHomeDir()
	CredsDir         = filepath.Join(home, fmt.Sprintf("/.%s", binaryName))
	DefaultCredsFile = filepath.Join(CredsDir, "credentials.json")
	EnvironmentMap   = map[string]Environment{
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
		{
			Name:   "api",
			Domain: "api",
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

func ensureAWSEnvSet() {
	if os.Getenv("AWS_ACCESS_KEY_ID") == "" || os.Getenv("AWS_SECRET_ACCESS_KEY") == "" || os.Getenv("AWS_SESSION_TOKEN") == "" {
		log.Fatal("AWS credentials not set")
	}
}

func PreRunCmd(cmd *cobra.Command, args []string) {
	script.Exec(fmt.Sprintf("mkdir -p %s", CredsDir)).Wait()
}

func getProcess(ppid int) ps.Process {
	process, err := ps.FindProcess(ppid)
	if err != nil {
		log.Fatal(err)
	}
	return process
}

func getShell() string {
	parent := getProcess(os.Getppid())
	if parent.Executable() == "go" {
		parent = getProcess(parent.PPid())
	}
	return parent.Executable()
}
