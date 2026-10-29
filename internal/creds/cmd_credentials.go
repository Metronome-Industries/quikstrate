package creds

import (
	"fmt"
	"log"
	"os"

	"github.com/spf13/cobra"
)

func CredentialsCmd(cmd *cobra.Command, args []string) {
	format := cmd.Flag("format").Value.String()
	force, _ := cmd.Flags().GetBool("force")
	check, _ := cmd.Flags().GetBool("check")
	environment, _ := cmd.Flags().GetString("environment")
	role, _ := cmd.Flags().GetString("role")
	roleData, err := credentialsRoleData(environment, role)
	if err != nil {
		log.Fatal(err)
	}
	file := credentialsCacheFile(roleData)

	if check {
		checkCredentials(file)
	}

	var creds Credentials
	if force {
		creds, err = getAndWriteCredentials(roleData, file)
	} else {
		creds, err = refreshCredentials(roleData, file)
	}
	if err != nil {
		log.Fatal(err)
	}
	creds.Print(format)
}

func credentialsRoleData(environment, role string) (RoleData, error) {
	if _, ok := EnvironmentMap[environment]; !ok {
		return RoleData{}, fmt.Errorf("unknown environment %q", environment)
	}
	if role == "Admin" {
		return RoleData{}, nil
	}
	if role == "" {
		role = environment + "-metdevops"
	}
	return RoleData{SpecialAccount: "substrate", Role: role}, nil
}

func credentialsCacheFile(role RoleData) string {
	if role == (RoleData{}) {
		return defaultCredsFile()
	}
	return role.GetFilename()
}

func checkCredentials(file string) {
	creds, err := getCredsFromFile(file)
	if err != nil || creds.needsRefresh() {
		os.Exit(1)
	}
	os.Exit(0)
}
