package creds

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"

	"github.com/bitfield/script"
	"github.com/jedib0t/go-pretty/v6/table"
	"github.com/spf13/cobra"
)

var accountsFile = filepath.Join(CredsDir, "accounts.json")

func AccountsCmd(cmd *cobra.Command, args []string) {
	format := cmd.Flag("format").Value.String()

	accountList, err := getAccountList()
	if err != nil {
		log.Fatal("Unable to retrieve account information:", err.Error())
	}
	accountList.Print(format)
}

func getAccountList() (accountList AccountList, err error) {
	accountList, err = readAccountsFile(accountsFile)
	if err != nil {
		log.Fatal("unable to read cached file, calling substrate...")
		accountList, err = refreshAccounts(accountsFile)
	}
	return
}

func refreshAccounts(file string) (accountList AccountList, err error) {
	defaultCreds, err := getDefaultCredentials()
	if err != nil {
		return
	}
	defaultCreds.SetEnv()

	byteValue, err := script.NewPipe().WithStderr(os.Stderr).Exec("substrate accounts -format json").Bytes()
	if err != nil {
		return
	}

	if err = json.Unmarshal(byteValue, &accountList.Accounts); err != nil {
		return
	}

	err = writeAccountsFile(file, accountList)
	return
}

func readAccountsFile(file string) (accountList AccountList, err error) {
	jsonFile, err := os.Open(file)
	if err != nil {
		return
	}
	defer jsonFile.Close()

	byteValue, err := io.ReadAll(jsonFile)
	if err != nil {
		return
	}

	err = json.Unmarshal(byteValue, &accountList)
	return
}

func writeAccountsFile(file string, accountList AccountList) error {
	jsonData, err := json.MarshalIndent(accountList, "", "  ")
	if err != nil {
		return err
	}

	err = os.WriteFile(file, jsonData, 0644)
	if err != nil {
		return err
	}

	return nil
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
		t.AppendHeader(table.Row{"Domain", "Envionment", "Account Number", "AWS_PROFILE", "Console"})
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
