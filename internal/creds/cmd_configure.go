package creds

import (
	"fmt"
	"log"
	"os"
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
	kubeConfigFile string = getenv("KUBECONFIG", filepath.Join(home, ".kube/config"))

	configDryrun            bool
	configClean             bool
	configUseIdentityCenter bool
	configUseSubstrate      bool
	configPreserveAWSAuth   bool
	awsRegion               string

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
	configUseIdentityCenter, _ = cmd.Flags().GetBool("use-identitycenter")
	configUseSubstrate, _ = cmd.Flags().GetBool("use-substrate")
	preserveAWSAuthFlag, _ := cmd.Flags().GetBool("preserve-aws-auth")
	var err error
	configPreserveAWSAuth, err = preserveAWSAuthEnabled(preserveAWSAuthFlag)
	if err != nil {
		log.Fatal(err)
	}
	awsRegion = cmd.Flag("aws-region").Value.String()
	environments := strings.Split(cmd.Flag("environments").Value.String(), ",")
	domains := strings.Split(cmd.Flag("domains").Value.String(), ",")

	// Use the running executable's path so credential_process entries in ~/.aws/config
	// always point to the binary that ran configure (important when testing local builds).
	binaryPath, err = os.Executable()
	if err != nil {
		binaryPath = os.Args[0]
	}

	if configCheck {
		err := checkConfig(environments, domains)
		if err != nil {
			log.Fatal("quikstrate configure not run...\n", err)
		}
		log.Print("quikstrate configured correctly...")
		os.Exit(0)
	}

	if configClean && !configDryrun {
		log.Print("Removing existing quikstrate config")
		os.Remove(quikstrateConfigFile)
	}

	if !configDryrun && !configPreserveAWSAuth && !configUseSubstrate && (configUseIdentityCenter || usingIDC()) {
		if err := checkAWSCLIVersion(); err != nil {
			log.Fatal(err)
		}
	}

	if !configDryrun {
		cfg := readQuikstrateConfig()
		if configUseIdentityCenter {
			cfg.CredentialSource = string(credentialSourceIDC)
		} else if configUseSubstrate {
			cfg.CredentialSource = string(credentialSourceSubstrate)
		}
		if err := writeQuikstrateConfig(cfg); err != nil {
			log.Fatal(err)
		}
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

func preserveAWSAuthEnabled(flagEnabled bool) (bool, error) {
	if flagEnabled {
		return true, nil
	}
	value := os.Getenv("QUIKSTRATE_PRESERVE_AWS_AUTH")
	if value == "" {
		return false, nil
	}
	enabled, err := strconv.ParseBool(value)
	if err != nil {
		return false, fmt.Errorf("QUIKSTRATE_PRESERVE_AWS_AUTH must be true, false, or unset")
	}
	return enabled, nil
}

func configureAWSConfig(environments, domains []string) error {
	log.Print("\nConfiguring aws config")
	if configClean {
		log.Print("Removing existing aws config")
		os.Remove(awsConfigFile)
	}

	// reverse order so staging is before prod
	sort.Sort(sort.Reverse(sort.StringSlice(environments)))

	if configPreserveAWSAuth {
		if err := removeQuikstrateAWSAuth(); err != nil {
			return err
		}
	} else if usingIDC() {
		for _, instance := range []idcInstance{metronomeIDC, activeStripeIDC()} {
			if err := writeSSOSessionConfig(instance.Name, instance.StartURL, instance.Region); err != nil {
				return err
			}
		}
	}

	for _, environment := range environments {
		for _, domain := range domains {
			profile := fmt.Sprintf("%s-%s", environment, domain)
			credentialProcess := fmt.Sprintf("\"%s assume -e %s -d %s -f json\"", binaryPath, environment, domain)
			setAWSProfile(profile, credentialProcess, awsRegion)
		}
	}

	setAWSProfile("management", fmt.Sprintf("\"%s assume --management -f json\"", binaryPath), awsRegion)
	for _, domain := range specialDomains {
		setAWSProfile(domain, fmt.Sprintf("\"%s assume --special %s -f json\"", binaryPath, domain), awsRegion)
	}

	if !configPreserveAWSAuth {
		setAWSConfigValue("default", "credential_process", fmt.Sprintf("\"%s credentials -f json\"", binaryPath))
	}
	setAWSConfigValue("default", "region", awsRegion)
	return nil
}

func setAWSProfile(name, credentialProcess, region string) {
	log.Printf("Configuring profile %s\n", name)
	if !configPreserveAWSAuth {
		setAWSConfigValue(name, "credential_process", credentialProcess)
	}
	setAWSConfigValue(name, "region", region)
}

// removeQuikstrateAWSAuth removes authentication installed by previous configure
// runs while preserving unrelated AWS configuration and the named profiles that
// devboxes use with their native credential provider chain.
func removeQuikstrateAWSAuth() error {
	if configDryrun {
		log.Printf("would remove quikstrate credential_process values and SSO sessions from %s", awsConfigFile)
		return nil
	}

	contents, err := os.ReadFile(awsConfigFile)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("reading %s: %w", awsConfigFile, err)
	}

	content := string(contents)
	for _, sessionName := range []string{metronomeIDC.Name, stripeIDC.Name, stripeAlternateIDC.Name} {
		content = removeSSOSession(content, sessionName)
	}

	var out strings.Builder
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		key, value, found := strings.Cut(trimmed, "=")
		if found && strings.TrimSpace(key) == "credential_process" && strings.Contains(value, "quikstrate") {
			continue
		}
		out.WriteString(line)
		out.WriteByte('\n')
	}

	return os.WriteFile(awsConfigFile, []byte(out.String()), 0600)
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
			if slices.Contains(kubeConfigSkips, fmt.Sprintf("%s-%s", environment, cluster.Domain)) {
				continue
			}
			// Skip if cluster has specific environments and this environment is not in the list
			if len(cluster.Environments) > 0 && !slices.Contains(cluster.Environments, environment) {
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
	// Check the profile structure rather than the authentication mechanism. This
	// allows Stripe devboxes to retain their native AWS credential provider.
	contents, err := os.ReadFile(awsConfigFile)
	if err != nil {
		return fmt.Errorf("%s doesn't exist", awsConfigFile)
	}
	for _, environment := range environments {
		for _, domain := range domains {
			profile := fmt.Sprintf("[profile %s-%s]", environment, domain)
			if !strings.Contains(string(contents), profile) {
				return fmt.Errorf("%s doesn't contain profile %s-%s", awsConfigFile, environment, domain)
			}
		}
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
			if slices.Contains(kubeConfigSkips, fmt.Sprintf("%s-%s", environment, cluster.Domain)) {
				continue
			}
			// Skip if cluster has specific environments and this environment is not in the list
			if len(cluster.Environments) > 0 && !slices.Contains(cluster.Environments, environment) {
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
