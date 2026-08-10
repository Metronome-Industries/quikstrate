package creds

import (
	"log"
	"os"

	"github.com/spf13/cobra"
)

func AssumeCmd(cmd *cobra.Command, args []string) {
	format := cmd.Flag("format").Value.String()
	force, _ := cmd.Flags().GetBool("force")
	management, _ := cmd.Flags().GetBool("management")
	special, _ := cmd.Flags().GetString("special")
	roleOverride, _ := cmd.Flags().GetString("role")

	var roleData RoleData

	switch {
	case management:
		// Special accounts default to engineersreadonly; pass --role admin for write access.
		roleData = RoleData{SpecialAccount: "management", Role: roleOverride}
	case special != "":
		roleData = RoleData{SpecialAccount: special, Role: roleOverride}
	default:
		env := cmd.Flag("env").Value.String()
		domain := cmd.Flag("domain").Value.String()
		quality := cmd.Flag("quality").Value.String()
		if env == "" || domain == "" {
			cmd.Usage()
			os.Exit(1)
		}
		var ok bool
		roleData, ok = NewRoleData(env, domain, quality, roleOverride)
		if !ok {
			log.Fatalf("unknown environment %q", env)
		}
	}

	var (
		creds Credentials
		err   error
	)
	if force {
		creds, err = getAndWriteCredentials(roleData, roleData.GetFilename())
	} else {
		creds, err = refreshCredentials(roleData, roleData.GetFilename())
	}
	if err != nil {
		log.Fatal(err)
	}
	creds.Print(format)
}

func NewRoleData(environment, domain, quality, role string) (RoleData, bool) {
	if _, ok := EnvironmentMap[environment]; !ok {
		return RoleData{}, false
	}
	if quality == "" {
		quality = EnvironmentMap[environment].DefaultQuality
	}
	return RoleData{
		Environment: environment,
		Domain:      domain,
		Quality:     quality,
		Role:        role,
	}, true
}
