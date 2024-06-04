package terraform

import (
	"bufio"
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"log"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/bmatcuk/doublestar/v4"
	"github.com/fatih/color"
	"github.com/hashicorp/go-version"
	"github.com/hashicorp/hc-install/product"
	"github.com/hashicorp/hc-install/releases"
	"github.com/hashicorp/terraform-exec/tfexec"
	"github.com/spf13/cobra"
)

var (
	DefaultMatchPatterns = []string{"*"}
	DefaultSkipPatterns  = []string{
		"api/*/*/us-east-2", // excessive amounts of drift
		"confluent",         // requires Tailscale to be up
	}
	DefaultConcurrency = 50
	TerraformVersion   = "1.5.6"
	terraformExec      string
	startTime          = time.Now()
)

// DriftCmd calculates terraform drift
func DriftCmd(cmd *cobra.Command, args []string) {
	TerraformVersion = cmd.Flag("terraform-version").Value.String()
	path := cmd.Flag("path").Value.String()
	// filter := cmd.Flag("filter").Value.String()
	verbose, _ := strconv.ParseBool(cmd.Flag("verbose").Value.String())
	matchPatterns, err := cmd.Flags().GetStringArray("match")
	if err != nil {
		log.Fatalf("Failed to get match patterns: %s", err)
	}
	skipPatterns, err := cmd.Flags().GetStringArray("skip")
	if err != nil {
		log.Fatalf("Failed to get skip patterns: %s", err)
	}

	if verbose {
		slog.SetLogLoggerLevel(slog.LevelDebug)
	}

	// if path isn't specified, we're assuming metronome-substrate
	if path == "" {
		var err error
		path, err = findGitRoot()
		if err != nil {
			log.Fatalf("Failed to find git root: %s", err)
		}
		path = filepath.Join(path, "root-modules")
		_, err = os.Stat(path)
		if err != nil {
			log.Fatalf("Run from metronome-substrate or specify a path: %s", err)
		}
	}

	calculateDrift(path, matchPatterns, skipPatterns)
}

type config struct {
	SkipRootModules []string `yaml:"skip_root_modules"`
	Concurrency     int      `yaml:"concurrency"`
}

type planOutput struct {
	Changed bool
	Error   error
	Output  io.ReadWriter
	Path    string
}

type planInput struct {
	Path string
	Lock bool
}

func calculateDrift(path string, matchPatterns []string, skipPatterns []string) {
	ctx := context.Background()

	modules, err := getModules(path)
	if err != nil {
		log.Fatalf("Failed to get root modules: %s", err)
	}

	var matchedModules []string
	for _, module := range modules {
		for _, pattern := range matchPatterns {
			match, err := doublestar.PathMatch(fmt.Sprintf("**/%s/**", pattern), module)
			if err != nil {
				log.Fatalf("match pattern error: %s", err)
			}
			if match {
				matchedModules = append(matchedModules, module)
			}
		}
	}
	var finalModules []string
	for _, module := range matchedModules {
		for _, pattern := range skipPatterns {
			match, err := doublestar.PathMatch(fmt.Sprintf("**/%s/**", pattern), module)
			if err != nil {
				log.Fatalf("skip pattern error: %s", err)
			}
			if !match {
				finalModules = append(finalModules, module)
			}
		}
	}

	planCount := len(finalModules)
	slog.Debug(
		"SEARCH",
		slog.Int("moduleCount", len(modules)),
		slog.Any("matchPatterns", matchPatterns),
		slog.Int("matchCount", len(matchedModules)),
		slog.Any("skipPatterns", skipPatterns),
		slog.Int("skipCount", len(matchedModules)-len(finalModules)),
	)
	slog.Info("running plans", "planCount", len(finalModules))

	terraformExec, err = installTerraform(ctx)
	if err != nil {
		log.Fatalf("Failed to install terraform: %s", err)
	}

	inputChannel := make(chan planInput, planCount)
	outputChannel := make(chan planOutput, planCount)
	if DefaultConcurrency > planCount {
		DefaultConcurrency = planCount
	}
	for i := 0; i < DefaultConcurrency; i++ {
		go func(workerId int) {
			planner(ctx, workerId, inputChannel, outputChannel)
		}(i)
	}

	for _, rootModule := range finalModules {
		slog.Debug("added to inputChannel", "module", rootModule)
		slog.Debug("added to inputChannel", "module", cleanRootModulePath(rootModule))
		inputChannel <- planInput{Path: rootModule, Lock: false}
	}
	close(inputChannel)

	clean := 0
	changes := 0
	errors := 0
	for i := 0; i < planCount; i++ {
		result := <-outputChannel
		switch {
		case result.Error != nil:
			errors++
			color.Red(result.String())
		case result.Changed:
			changes++
			color.Yellow(result.String())
		default:
			clean++
			color.Green(result.String())
		}
	}
	slog.Info(
		"SUMMARY",
		slog.Int("plans", planCount),
		slog.Int("errors", errors),
		slog.Int("changes", changes),
		slog.Int("clean", clean),
		slog.Duration("duration", time.Since(startTime)),
		slog.Int("concurrency", DefaultConcurrency),
	)
}

func installTerraform(ctx context.Context) (string, error) {
	installer := &releases.ExactVersion{
		Product: product.Terraform,
		Version: version.Must(version.NewVersion(TerraformVersion)),
	}
	return installer.Install(ctx)
}

func getModules(path string) ([]string, error) {
	slog.Info("getting root modules", "path", path)
	modulePaths := []string{}
	err := filepath.WalkDir(path, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == ".terraform" {
				return filepath.SkipDir
			}
		} else {
			if d.Name() == ".terraform.lock.hcl" {
				modulePaths = append(modulePaths, filepath.Dir(path))
				return filepath.SkipDir
			}
		}
		return nil
	})
	return modulePaths, err
}

func planner(ctx context.Context, id int, input <-chan planInput, output chan<- planOutput) {
	for i := range input {
		logger := slog.With(slog.Int("worker", id), slog.String("module", cleanRootModulePath(i.Path)))
		logger.Debug("starting plan")
		output <- plan(
			ctx,
			i.Path,
			i.Lock,
			logger,
		)
	}
}

func plan(ctx context.Context, path string, lock bool, logger *slog.Logger) planOutput {
	tf, err := tfexec.NewTerraform(path, terraformExec)
	if err != nil {
		logger.Error("failed to create terraform", "error", err)
		return planOutput{Path: path, Error: err}
	}

	logger.Debug("terraform init")
	err = tf.Init(ctx, tfexec.Upgrade(false))
	if err != nil {
		logger.Error("terraform init error", "error", err)
		return planOutput{Path: path, Error: err}
	}

	var b bytes.Buffer
	output := planOutput{Path: path, Output: &b}
	logger.Debug("terraform plan")
	output.Changed, output.Error = tf.PlanJSON(ctx, output.Output, tfexec.Lock(lock))
	if output.Error != nil {
		logger.Error("terraform plan error", "error", output.Error)
	}
	return output
}

func (p planOutput) String() string {
	if p.Error != nil {
		return fmt.Sprintf("ERROR:\t\t%s", cleanRootModulePath(p.Path))
	}
	if p.Changed {
		parsedLines, _, err := parseOutput(p.Output)
		if err != nil {
			log.Fatal(err)
		}

		var summary plannedChanges
		for _, line := range parsedLines {
			switch line.Type {
			case "change_summary":
				summary.Summary = line.Message
			case "planned_change":
				switch line.Change.Action {
				case "create":
					summary.Create = append(summary.Create, line.Change.Resource.Address)
				case "update":
					summary.Update = append(summary.Update, line.Change.Resource.Address)
				case "delete":
					summary.Delete = append(summary.Delete, line.Change.Resource.Address)
				case "noop":
					summary.Noop = append(summary.Noop, line.Change.Resource.Address)
				}
			}
		}
		return fmt.Sprintf("CHANGES:\t%s\t%s%s", cleanRootModulePath(p.Path), summary.Summary, summary)
	}
	return fmt.Sprintf("CLEAN:\t\t%s", cleanRootModulePath(p.Path))
}

func cleanRootModulePath(path string) string {
	return strings.Split(path, "root-modules/")[1]
}

// https://developer.hashicorp.com/terraform/internals/machine-readable-ui
func parseOutput(output io.ReadWriter) (formattedLines []tfOutputLine, rawLines []string, err error) {
	scanner := bufio.NewScanner(output)
	for scanner.Scan() {
		var tfLine tfOutputLine
		line := scanner.Text()
		rawLines = append(rawLines, line)
		err = json.Unmarshal([]byte(line), &tfLine)
		if err != nil {
			slog.Error("parsing failed", "line", line)
			return
		}
		formattedLines = append(formattedLines, tfLine)
	}
	slog.Debug("parse complete", "raw", len(rawLines), "parsed", len(formattedLines))
	return
}

type tfOutputLine struct {
	Level     string          `json:"@level"`
	Message   string          `json:"@message"`
	Module    string          `json:"@module"`
	Timestamp string          `json:"@timestamp"`
	Type      string          `json:"type"`
	Change    tfPlannedChange `json:"change,omitempty"`
}

// https://developer.hashicorp.com/terraform/internals/machine-readable-ui#planned-change
type tfPlannedChange struct {
	Action   string           `json:"action"`
	Resource tfChangeResource `json:"resource"`
}

type tfChangeResource struct {
	Address         string      `json:"addr,omitempty"`
	Module          string      `json:"module,omitempty"`
	Resource        string      `json:"resource,omitempty"`
	ResourceType    string      `json:"resource_type,omitempty"`
	ResourceName    string      `json:"resource_name,omitempty"`
	ImpliedProvider string      `json:"implied_provider,omitempty"`
	ResourceKey     interface{} `json:"resource_key,omitempty"`
}

type plannedChanges struct {
	Summary string
	Noop    []string
	Create  []string
	Update  []string
	Delete  []string
}

func (p plannedChanges) String() string {
	var sb strings.Builder
	if len(p.Create) > 0 {
		sb.WriteString(fmt.Sprintf("\n\tCreates:"))
		for _, resource := range p.Create {
			sb.WriteString(fmt.Sprintf("\n\t\t%s", resource))
		}
	}
	if len(p.Update) > 0 {
		sb.WriteString(fmt.Sprintf("\n\tUpdates:"))
		for _, resource := range p.Update {
			sb.WriteString(fmt.Sprintf("\n\t\t%s", resource))
		}
	}
	if len(p.Delete) > 0 {
		sb.WriteString(fmt.Sprintf("\n\tDeletes:"))
		for _, resource := range p.Delete {
			sb.WriteString(fmt.Sprintf("\n\t\t%s", resource))
		}
	}
	if len(p.Noop) > 0 {
		sb.WriteString(fmt.Sprintf("\n\tNoops:"))
		for _, resource := range p.Noop {
			sb.WriteString(fmt.Sprintf("\n\t\t%s", resource))
		}
	}
	return sb.String()
}

func findGitRoot() (string, error) {
	path, err := exec.Command("git", "rev-parse", "--show-toplevel").Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(path)), nil
}
