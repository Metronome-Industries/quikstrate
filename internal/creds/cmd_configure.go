package creds

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/bitfield/script"
	"github.com/spf13/cobra"
	"k8s.io/client-go/tools/clientcmd"
)

var (
	awsConfigFile  string = getenv("AWS_CONFIG_FILE", filepath.Join(home, ".aws/config"))
	kubeConfigFile string = getenv("KUBECONFIG", filepath.Join(home, ".kube/config"))

	configDryrun bool
	configClean  bool
	awsRegion    string

	binaryName = "quikstrate"
	binaryPath string
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

	setAWSConfigValue("default", "credential_process", fmt.Sprintf("\"%s credentials -f json\"", binaryPath))
	setAWSConfigValue("default", "region", awsRegion)
	for _, environment := range environments {
		for _, domain := range domains {
			profile := fmt.Sprintf("%s-%s", environment, domain)
			log.Printf("Configuring profile %s\n", profile)

			setAWSConfigValue(profile, "credential_process", fmt.Sprintf("\"%s assume -e %s -d %s -f json\"", binaryPath, environment, domain))
			setAWSConfigValue(profile, "region", awsRegion)
		}
	}
	return nil
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
		os.Remove(kubeConfigFile)
	}
	for _, environment := range environments {
		for _, cluster := range Clusters {
			if !slices.Contains(domains, cluster.Domain) {
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
			// by nulling out these environment variables in the kubeconfig, we force kubectl to use AWS_PROFILE regardless of what is set in the environment
			for _, v := range []string{"AWS_ACCESS_KEY_ID", "AWS_SECRET_ACCESS_KEY", "AWS_SESSION_TOKEN"} {
				cmd := fmt.Sprintf("kubectl config set-credentials %s-%s --exec-env %s=\"\"", environment, cluster.Name, v)
				if configDryrun {
					log.Print(cmd)
				} else {
					_, err := script.Exec(cmd).Stdout()
					if err != nil {
						log.Fatal(err)
					}
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
	config, err := clientcmd.LoadFromFile(kubeConfigFile)
	if err != nil {
		return err
	}
	for _, environment := range environments {
		for _, cluster := range Clusters {
			if !slices.Contains(domains, cluster.Domain) {
				continue
			}
			clusterName := fmt.Sprintf("%s-%s", environment, cluster.Name)

			if _, ok := config.Contexts[clusterName]; !ok {
				return fmt.Errorf("%s doesn't contain context %s", kubeConfigFile, clusterName)
			}
			if _, ok := config.AuthInfos[clusterName]; !ok {
				return fmt.Errorf("%s doesn't contain user %s", kubeConfigFile, clusterName)
			}
		}
	}
	return nil
}
