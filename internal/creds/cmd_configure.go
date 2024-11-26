package creds

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/bitfield/script"
	"github.com/spf13/cobra"
	"k8s.io/client-go/tools/clientcmd"
)

var (
	awsConfigFile  string = getenv("AWS_CONFIG_FILE", filepath.Join(home, ".aws/config"))
	KubeConfigFile string = getenv("KUBECONFIG", filepath.Join(home, ".kube/config"))

	configDryrun bool
	configClean  bool
	awsRegion    string

	binaryName = "quikstrate"
	binaryPath string

	// match fmt.Sprintf("%s-%s", environment, cluster.Domain)
	kubeConfigSkips = []string{}

	specialDomains = []string{"audit", "deploy", "network"} // management is special
)

func ConfigureCmd(cmd *cobra.Command, args []string) {
	configClean, _ = strconv.ParseBool(cmd.Flag("clean").Value.String())
	configDryrun, _ = strconv.ParseBool(cmd.Flag("dryrun").Value.String())
	configCheck, _ := strconv.ParseBool(cmd.Flag("check").Value.String())
	awsRegion = cmd.Flag("aws-region").Value.String()
	environments := strings.Split(cmd.Flag("environments").Value.String(), ",")
	domains := strings.Split(cmd.Flag("domains").Value.String(), ",")

	var err error
	binaryPath, err = exec.LookPath(binaryName)
	if err != nil {
		if path.Base(os.Args[0]) != "main" {
			log.Fatalf("could not find %s binary in path...", binaryName)
		}
		// don't worry about fullpath if running from go run
		binaryPath = binaryName
	}

	if configCheck {
		err := checkConfig(environments, domains)
		if err != nil {
			log.Fatal("quikstrate configure not run...\n", err)
		}
		log.Print("quikstrate configured correctly...")
		os.Exit(0)
	}

	err = configureAWSConfig(environments, domains)
	if err != nil {
		log.Fatal(err)
	}

	err = configureKubeConfig(environments, domains)
	if err != nil {
		log.Fatal(err)
	}
}

func configureAWSConfig(environments, domains []string) error {
	log.Print("\nConfiguring aws config")
	if configClean {
		log.Print("Removing existing aws config")
		os.Remove(awsConfigFile)
	}

	// reverse order so staging is before prod
	sort.Sort(sort.Reverse(sort.StringSlice(environments)))
	for _, environment := range environments {
		for _, domain := range domains {
			profile := fmt.Sprintf("%s-%s", environment, domain)
			setAWSProfile(profile, fmt.Sprintf("\"%s assume -e %s -d %s -f json\"", binaryPath, environment, domain), awsRegion)
		}
	}

	setAWSProfile("management", "\"substrate assume-role --management --format json\"", awsRegion)
	for _, domain := range specialDomains {
		setAWSProfile(domain, fmt.Sprintf("\"substrate assume-role --special %s --format json\"", domain), awsRegion)
	}

	setAWSConfigValue("default", "credential_process", fmt.Sprintf("\"%s credentials -f json\"", binaryPath))
	setAWSConfigValue("default", "region", awsRegion)
	return nil
}

func setAWSProfile(name, credentialProcess, region string) {
	log.Printf("Configuring profile %s\n", name)
	setAWSConfigValue(name, "credential_process", credentialProcess)
	setAWSConfigValue(name, "region", region)
}

func setAWSConfigValue(profile, key, value string) {
	cmd := fmt.Sprintf("aws configure set profile.%s.%s %s", profile, key, value)
	if configDryrun {
		log.Print(cmd)
	} else {
		script.Exec(fmt.Sprintf("aws configure set profile.%s.%s %s", profile, key, value)).Stdout()
	}
}

func configureKubeConfig(environments, domains []string) error {
	log.Print("\nConfiguring kubeconfig")
	if configClean {
		log.Print("Removing existing kubeconfig")
		os.Remove(KubeConfigFile)
	}
	for _, environment := range environments {
		for _, cluster := range Clusters {
			if !slices.Contains(domains, cluster.Domain) {
				continue
			}
			if slices.Contains(kubeConfigSkips, fmt.Sprintf("%s-%s", environment, cluster.Domain)) {
				continue
			}

			// aws eks update-config
			cmd := fmt.Sprintf("aws eks update-kubeconfig --alias %[1]s-%[3]s --user-alias %[1]s-%[3]s --name %[3]s --profile %[1]s-%[2]s", environment, cluster.Domain, cluster.Name)
			if configDryrun {
				log.Printf("export AWS_PROFILE=%s\n", fmt.Sprintf("%s-%s", environment, cluster.Domain))
				log.Print(cmd)
			} else {
				os.Setenv("AWS_PROFILE", fmt.Sprintf("%s-%s", environment, cluster.Domain))
				_, err := script.Exec(cmd).Stdout()
				if err != nil {
					log.Fatal(err)
				}
			}
		}
	}
	return nil
}
func getenv(key, fallback string) string {
	value := os.Getenv(key)
	if len(value) == 0 {
		return fallback
	}
	return value
}

func checkConfig(environments, domains []string) error {
	// simple ~/.aws/config check, greps for quikstrate string
	out, err := script.IfExists(awsConfigFile).Exec("cat " + awsConfigFile).Match(binaryName).String()
	if err != nil {
		return fmt.Errorf("%s doesn't exist", awsConfigFile)
	}
	if strings.TrimSpace(out) == "" {
		return fmt.Errorf("%s doesn't call %s", awsConfigFile, binaryName)
	}

	// simple ~/.kube/config check, validates contexts and users exist
	config, err := clientcmd.LoadFromFile(KubeConfigFile)
	if err != nil {
		return err
	}
	for _, environment := range environments {
		for _, cluster := range Clusters {
			if !slices.Contains(domains, cluster.Domain) {
				continue
			}
			if slices.Contains(kubeConfigSkips, fmt.Sprintf("%s-%s", environment, cluster.Domain)) {
				continue
			}
			clusterName := fmt.Sprintf("%s-%s", environment, cluster.Name)

			if _, ok := config.Contexts[clusterName]; !ok {
				return fmt.Errorf("%s doesn't contain context %s", KubeConfigFile, clusterName)
			}
			if _, ok := config.AuthInfos[clusterName]; !ok {
				return fmt.Errorf("%s doesn't contain user %s", KubeConfigFile, clusterName)
			}
		}
	}
	return nil
}
