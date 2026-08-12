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
	Domains = []string{"api", "auth", "druid", "graphql", "ingest", "integrations", "lakehouse", "lambda", "marketplaces", "network-staging", "notifications", "static-sites", "internal-services"}
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
			Name:   "dagster",
			Domain: "lakehouse",
		},
		{
			Name:         "internal-services",
			Domain:       "internal-services",
			Environments: []string{"prod"}, // Only exists in prod
		},
	}
)

type ClusterSpec struct {
	Name         string
	Domain       string
	Environments []string // If empty, cluster exists in all environments
}

type Environment struct {
	Name           string
	Aliases        []string
	DefaultQuality string
	DefaultRole    string
}

type RoleData struct {
	Environment    string
	Domain         string
	Quality        string
	Role           string
	SpecialAccount string // non-empty when assuming into a special account (management, audit, deploy, network)
}

func (r RoleData) GetFilename() string {
	suffix := ".json"
	if useIDC() {
		suffix = "-idc.json"
	}
	if r.SpecialAccount != "" {
		role := r.Role
		if useIDC() {
			role = normalizeIDCRole(r.Role, "prod")
		}
		return filepath.Join(CredsDir, fmt.Sprintf("special-%s-%s%s", r.SpecialAccount, role, suffix))
	}
	role := r.Role
	if useIDC() {
		role = normalizeIDCRole(r.Role, r.Environment)
	} else if role == "" {
		role = idcRoleForEnvironment(r.Environment)
	}
	return filepath.Join(CredsDir, strings.ToLower(fmt.Sprintf("%s-%s-%s-%s%s", r.Environment, r.Domain, r.Quality, role, suffix)))
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
