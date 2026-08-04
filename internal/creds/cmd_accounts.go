package creds

import (
	"encoding/json"
	"fmt"
	"log"
	"os"

	"github.com/jedib0t/go-pretty/v6/table"
	"github.com/spf13/cobra"
)

func AccountsCmd(cmd *cobra.Command, args []string) {
	format := cmd.Flag("format").Value.String()

	accountList, err := getAccountList()
	if err != nil {
		log.Fatal("Unable to retrieve account information:", err.Error())
	}
	accountList.Print(format)
}

func getAccountList() (AccountList, error) {
	return buildStaticAccountList(), nil
}

// Construct AccountList from the hardcoded account maps
func buildStaticAccountList() AccountList {
	var accounts []Account
	for key, id := range staticServiceAccounts {
		domain, environment := key[0], key[1]
		quality := ""
		if env, ok := EnvironmentMap[environment]; ok {
			quality = env.DefaultQuality
		}
		accounts = append(accounts, Account{
			Id:     id,
			Name:   fmt.Sprintf("%s-%s", domain, environment),
			Status: "ACTIVE",
			Tags: map[string]string{
				"Domain":      domain,
				"Environment": environment,
				"Quality":     quality,
			},
		})
	}
	for name, id := range staticSpecialAccounts {
		accounts = append(accounts, Account{
			Id:     id,
			Name:   name,
			Status: "ACTIVE",
			Tags:   map[string]string{},
		})
	}
	return AccountList{Accounts: accounts}
}

type Account struct {
	Arn             string            `json:"Arn"`
	Email           string            `json:"Email"`
	Id              string            `json:"Id"`
	JoinedMethod    string            `json:"JoinedMethod"`
	JoinedTimestamp string            `json:"JoinedTimestamp"`
	Name            string            `json:"Name"`
	Status          string            `json:"Status"`
	Tags            map[string]string `json:"Tags"`
}

type AccountList struct {
	Accounts []Account
}

func (a AccountList) Print(format string) {
	switch format {
	case "json":
		jsonData, _ := json.MarshalIndent(a, "", "  ")
		fmt.Printf("%s\n", jsonData)
	case "text":
		var rows []table.Row
		for _, account := range a.Accounts {
			if account.Status != "ACTIVE" {
				continue
			}
			if env, ok := account.Tags["Environment"]; ok {
				if _, ok := EnvironmentMap[env]; ok {
					rows = append(rows, table.Row{
						account.Tags["Domain"],
						env,
						account.Id,
						fmt.Sprintf("AWS_PROFILE=%s-%s", env, account.Tags["Domain"]),
						fmt.Sprintf("https://gnome.house/accounts?number=%s&role=%s", account.Id, EnvironmentMap[env].DefaultRole),
					})
				}
			}
		}
		t := table.NewWriter()
		t.SetOutputMirror(os.Stdout)
		t.AppendHeader(table.Row{"Domain", "Environment", "Account Number", "AWS_PROFILE", "Console"})
		t.AppendRows(rows)
		t.SortBy([]table.SortBy{
			{Name: "Domain", Mode: table.Asc},
			{Name: "Environment", Mode: table.Asc},
		})
		t.Render()

	default:
		fmt.Println("unknown format")
	}
}
