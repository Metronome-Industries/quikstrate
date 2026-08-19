package cmd

import (
	"sort"
	"strings"

	"github.com/metronome-industries/quikstrate/internal/creds"
	"github.com/spf13/cobra"
)

var configureCmd = &cobra.Command{
	Use:   "configure",
	Short: "Sets up the aws and kubectl clis with creds",
	Long: `This command loops through all environments and domains and sets up ~/.aws/config and ~/.kube/config to use this binary. It assumes you have already installed the aws and kubectl clis.
	aws-cli:
		- creates a profile for each environment and domain
		- sets the region for each profile to "us-west-2" (configurable)
		- sets the credential_process for each profile to this tool, allowing you to easily use cached credentials
		- set the profile by:
			- setting the AWS_PROFILE environment variable
			- using the --profile flag on the aws-cli
	kubectl:
		- creates a context for each cluster
		- uses the "aws eks update-kubeconfig" command to set the correct AWS_PROFILE for each context
	identity center (idc):
		- when idc is configured, adds an sso session block to ~/.aws/config
	`,
	Run:    creds.ConfigureCmd,
	PreRun: creds.PreRunCmd,
}

func init() {
	configureCmd.Flags().BoolP("clean", "c", false, "removes existing config files before configuring")
	configureCmd.Flags().Bool("check", false, "checks if this command has been run before")
	configureCmd.Flags().BoolP("dryrun", "d", false, "removes existing config files before configuring")
	configureCmd.Flags().Bool("use-identitycenter", false, "use Metronome IAM Identity Center as the credential source (write Identity Center block to ~/.aws/config)")
	configureCmd.Flags().Bool("use-substrate", false, "use Substrate as the credential source (pair with --clean to hard delete Identity Center settings)")
	configureCmd.MarkFlagsMutuallyExclusive("clean", "dryrun", "check")
	configureCmd.MarkFlagsMutuallyExclusive("use-identitycenter", "use-substrate")
	configureCmd.Flags().String("aws-region", "us-west-2", "aws region to configure")
	var defaultEnvs []string
	for _, env := range creds.EnvironmentMap {
		defaultEnvs = append(defaultEnvs, env.Name)
	}
	sort.Strings(defaultEnvs)
	configureCmd.Flags().String("environments", strings.Join(defaultEnvs, ","), "comma separated list of environments to configure")
	configureCmd.Flags().String("domains", strings.Join(creds.Domains, ","), "comma separated list of domains to configure")
	configureCmd.Flags().MarkHidden("environments")
	configureCmd.Flags().MarkHidden("domains")
	rootCmd.AddCommand(configureCmd)
}
