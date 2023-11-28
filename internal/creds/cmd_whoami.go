package creds

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"regexp"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/jedib0t/go-pretty/v6/table"
	"github.com/spf13/cobra"
)

var assumedRoleRegex = regexp.MustCompile("arn:aws:sts::([a-z0-9]+):assumed-role/(.+?)/(.+)")

func WhoamiCmd(cmd *cobra.Command, args []string) {
	format := cmd.Flag("format").Value.String()

	accountList, err := getAccountList()
	if err != nil {
		log("Unable to retrieve account information:", err.Error())
		os.Exit(1)
	}

	callerIdentity, err := getCallerIdentity(context.TODO())
	if err != nil {
		log("Unable to retrieve aws identity:", err.Error())
		os.Exit(1)
	}

	out, err := whoami(callerIdentity, accountList)
	if err != nil {
		log(err.Error())
		os.Exit(1)
	}

	out.Print(format)
}

type callerIdentity struct {
	Account string
	Role    string
	User    string
}

type whoamiOutput struct {
	AccountName string `json:"AccountName"`
	AccountID   string `json:"AccountID"`
	Domain      string `json:"Domain"`
	Environment string `json:"Environment"`
	Quality     string `json:"Quality"`
	Role        string `json:"Role"`
	User        string `json:"User"`
}

func (o whoamiOutput) Print(format string) {
	switch format {
	case "json":
		jsonData, _ := json.MarshalIndent(o, "", "  ")
		fmt.Printf("%s\n", jsonData)
	case "text":
		t := table.NewWriter()
		t.SetOutputMirror(os.Stdout)
		t.AppendHeader(table.Row{"Domain", "Envionment", "Quality", "Role", "User"})
		t.AppendRow(table.Row{
			o.Domain,
			o.Environment,
			o.Quality,
			o.Role,
			o.User,
		})
		t.Render()
	default:
		fmt.Printf("format %s is unsupported...", format)
		os.Exit(1)
	}
}

func getCallerIdentity(ctx context.Context) (callerIdentity, error) {
	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return callerIdentity{}, err
	}

	client := sts.NewFromConfig(cfg)
	out, err := client.GetCallerIdentity(context.TODO(), &sts.GetCallerIdentityInput{})
	if err != nil {
		return callerIdentity{}, nil
	}

	matches := assumedRoleRegex.FindStringSubmatch(aws.ToString(out.Arn))
	if len(matches) != 4 {
		return callerIdentity{}, fmt.Errorf("Unable to parse caller identity from arn: %s", aws.ToString(out.Arn))
	}

	return callerIdentity{
		Account: matches[1],
		Role:    matches[2],
		User:    matches[3],
	}, nil
}

func whoami(ci callerIdentity, al AccountList) (whoamiOutput, error) {
	for _, account := range al.Accounts {
		if account.Id == ci.Account {
			out := whoamiOutput{
				AccountName: account.Name,
				Role:        ci.Role,
				User:        ci.User,
				AccountID:   ci.Account,
			}
			if val, ok := account.Tags["Environment"]; ok {
				out.Environment = val
			}
			if val, ok := account.Tags["Domain"]; ok {
				out.Domain = val
			}
			if val, ok := account.Tags["Quality"]; ok {
				out.Quality = val
			}
			return out, nil
		}
	}
	return whoamiOutput{}, errors.New(fmt.Sprintf("No matching account found for %+v", ci))
}
