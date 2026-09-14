package creds

import (
	"fmt"
	"sort"
)

// Hardcode [domain, environment] to AWS account IDs to remove dependency on Substrate
var staticServiceAccounts = map[[2]string]string{
	{"api", "staging"}:             "407752757973",
	{"api", "prod"}:                "477056945755",
	{"auth", "staging"}:            "015545333344",
	{"auth", "prod"}:               "614579421457",
	{"druid", "prod"}:              "035220036306",
	{"druid-loadtest", "prod"}:     "905418052488",
	{"graphql", "staging"}:         "008444403661",
	{"graphql", "prod"}:            "051318803586",
	{"ingest", "staging"}:          "464715055874",
	{"ingest", "prod"}:             "601156230221",
	{"integrations", "staging"}:    "637423284925",
	{"integrations", "prod"}:       "533267210102",
	{"internal-services", "prod"}:  "207567762512",
	{"lakehouse", "staging"}:       "445357087344",
	{"lakehouse", "prod"}:          "355843283871",
	{"lambda", "staging"}:          "802783861107",
	{"lambda", "prod"}:             "088932849318",
	{"marketplaces", "staging"}:    "501845335119",
	{"marketplaces", "prod"}:       "916227654331",
	{"network-staging", "staging"}: "075647413734",
	{"notifications", "staging"}:   "909838927472",
	{"notifications", "prod"}:      "078168529438",
	{"static-sites", "staging"}:    "414118243174",
	{"static-sites", "prod"}:       "447219469935",
	{"admin", "admin"}:             "666642175330",
}

// Special accounts are a concept carried over from Substrate. They are accessed via --special <name>.
var staticSpecialAccounts = map[string]string{
	"management": "420073272039",
	"audit":      "465454680116",
	"deploy":     "703712742941",
	"network":    "814412579886",
	"substrate":  "666642175330", // duplicated as {"admin", "admin"}
}

func lookupServiceAccountID(domain, environment string) (string, error) {
	if id, ok := staticServiceAccounts[[2]string{domain, environment}]; ok {
		return id, nil
	}
	return "", fmt.Errorf("unknown account: domain=%s environment=%s. Update accounts.go with account id.", domain, environment)
}

func lookupSpecialAccountID(name string) (string, error) {
	if id, ok := staticSpecialAccounts[name]; ok {
		return id, nil
	}
	return "", fmt.Errorf("unknown special account %q. Update accounts.go with account id.", name)
}

func allAccountIDs() []string {
	set := map[string]bool{}
	for _, id := range staticServiceAccounts {
		set[id] = true
	}
	for _, id := range staticSpecialAccounts {
		set[id] = true
	}
	ids := make([]string, 0, len(set))
	for id := range set {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

func accountIDsForSelection(selection string) ([]string, error) {
	set := map[string]bool{}
	switch selection {
	case "staging", "prod":
		for key, id := range staticServiceAccounts {
			if key[1] == selection {
				set[id] = true
			}
		}
	case "admin":
		for _, id := range staticSpecialAccounts {
			set[id] = true
		}
	case "":
		return nil, fmt.Errorf("account selection must be staging, prod, admin, or a 12-digit account ID")
	default:
		for _, id := range allAccountIDs() {
			if id == selection {
				return []string{id}, nil
			}
		}
		return nil, fmt.Errorf("unknown account selection %q; expected staging, prod, admin, or a known account ID", selection)
	}
	ids := make([]string, 0, len(set))
	for id := range set {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids, nil
}
