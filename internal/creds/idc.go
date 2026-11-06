package creds

import (
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	awscredentials "github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/sso"
	"github.com/aws/aws-sdk-go-v2/service/sso/types"
	"github.com/aws/aws-sdk-go-v2/service/ssooidc"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	smithy "github.com/aws/smithy-go"
	version "github.com/hashicorp/go-version"
)

type idcInstance struct {
	Name     string
	StartURL string
	Region   string
}

var (
	metronomeIDC       = idcInstance{Name: "metronome", StartURL: "https://d-9267463e84.awsapps.com/start", Region: "us-west-2"}
	stripeIDC          = idcInstance{Name: "stripe", StartURL: "https://d-9267fda1d4.awsapps.com/start", Region: "us-west-2"}
	stripeAlternateIDC = idcInstance{Name: "stripe-us-east-2", StartURL: "https://ssoins-7907aa69624c0735.portal.us-east-2.app.aws", Region: "us-east-2"}
	awsCommand         = exec.Command
	awsVersionOutput   = func() (string, error) {
		output, err := awsCommand("aws", "--version").CombinedOutput()
		return string(output), err
	}
	getIDCCredentialsForInstance = func(instance idcInstance, accountID, roleName string) (Credentials, error) {
		return getSSORoleCredentials(instance.Name, instance.StartURL, instance.Region, accountID, roleName)
	}
	assumeIAMRoleCredentials = assumeIAMRole
	loginSSOSession          = runSSOLogin
	createSSOToken           = func(region string, input *ssooidc.CreateTokenInput) (*ssooidc.CreateTokenOutput, error) {
		cfg, err := awsconfig.LoadDefaultConfig(context.Background(), awsconfig.WithRegion(region))
		if err != nil {
			return nil, err
		}
		return ssooidc.NewFromConfig(cfg).CreateToken(context.Background(), input)
	}
)

func alternateRegionEnabled() bool {
	value := os.Getenv("SC_USE_ALTERNATE_REGION")
	if value == "" {
		return false
	}
	enabled, err := strconv.ParseBool(value)
	if err != nil {
		panic("SC_USE_ALTERNATE_REGION must be true, false, or unset")
	}
	return enabled
}

func activeStripeIDC() idcInstance {
	if alternateRegionEnabled() {
		return stripeAlternateIDC
	}
	return stripeIDC
}

const awsCLIVersionHelp = "https://docs.aws.amazon.com/cli/latest/userguide/getting-started-install.html"

// ssoToken mirrors the subset of fields the AWS CLI writes to ~/.aws/sso/cache/*.json.
type ssoToken struct {
	AccessToken string `json:"accessToken"`
	ExpiresAt   string `json:"expiresAt"`
}

func (t ssoToken) expiry() (time.Time, error) {
	return parseSSOTimestamp(t.ExpiresAt)
}

func parseSSOTimestamp(value string) (time.Time, error) {
	// AWS CLI writes either RFC3339 "Z" suffix or a bare "UTC" suffix depending on version.
	s := strings.TrimSuffix(value, "UTC")
	if !strings.HasSuffix(s, "Z") {
		s += "Z"
	}
	return time.Parse(time.RFC3339, s)
}

func (t ssoToken) isValid() bool {
	exp, err := t.expiry()
	if err != nil || t.AccessToken == "" {
		return false
	}
	return time.Now().Add(defaultRefreshTrigger).Before(exp)
}

// tokenCachePath returns the AWS CLI's cache path for a named sso-session token:
// sha1(sessionName).json.
func tokenCachePath(sessionName string) string {
	h := sha1.New()
	h.Write([]byte(sessionName))
	return filepath.Join(home, ".aws", "sso", "cache", fmt.Sprintf("%x.json", h.Sum(nil)))
}

func readSSOToken(sessionName string) (ssoToken, error) {
	data, err := os.ReadFile(tokenCachePath(sessionName))
	if err != nil {
		return ssoToken{}, err
	}
	var t ssoToken
	return t, json.Unmarshal(data, &t)
}

// writeSSOSessionConfig writes a managed SSO session to ~/.aws/config directly;
// `aws configure set` mishandles the sso-session.* namespace.
func writeSSOSessionConfig(sessionName, startURL, region string) error {
	managed := awsManagedSection{
		header: fmt.Sprintf("[sso-session %s]", sessionName),
		values: [][2]string{
			{"sso_start_url", startURL},
			{"sso_region", region},
			{"sso_registration_scopes", "sso:account:access"},
		},
	}

	if configDryrun {
		log.Printf("would update managed SSO session %s in %s", sessionName, awsConfigFile)
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(awsConfigFile), 0700); err != nil {
		return fmt.Errorf("creating AWS config directory: %w", err)
	}

	existing, err := os.ReadFile(awsConfigFile)
	if os.IsNotExist(err) {
		existing = nil
	} else if err != nil {
		return fmt.Errorf("reading %s: %w", awsConfigFile, err)
	}
	content := updateManagedAWSConfig(string(existing), []awsManagedSection{managed}, false)
	return os.WriteFile(awsConfigFile, []byte(content), 0600)
}

// refreshSSOToken returns refreshed=false when interactive authentication is needed.
// The pinned SDK provider only refreshes after expiry and clears omitted refresh
// tokens, so call CreateToken directly to preserve our window and CLI cache state.
func refreshSSOToken(sessionName, region string) (ssoToken, bool, error) {
	data, err := os.ReadFile(tokenCachePath(sessionName))
	if err != nil {
		return ssoToken{}, false, errors.New("reading SSO refresh cache failed")
	}
	var state struct {
		RefreshToken          string `json:"refreshToken"`
		ClientID              string `json:"clientId"`
		ClientSecret          string `json:"clientSecret"`
		RegistrationExpiresAt string `json:"registrationExpiresAt"`
	}
	if json.Unmarshal(data, &state) != nil || state.RefreshToken == "" || state.ClientID == "" || state.ClientSecret == "" {
		return ssoToken{}, false, nil
	}
	registrationExpiry, err := parseSSOTimestamp(state.RegistrationExpiresAt)
	if err != nil || !time.Now().Before(registrationExpiry) {
		return ssoToken{}, false, nil
	}
	resp, err := createSSOToken(region, &ssooidc.CreateTokenInput{
		ClientId:     aws.String(state.ClientID),
		ClientSecret: aws.String(state.ClientSecret),
		RefreshToken: aws.String(state.RefreshToken),
		GrantType:    aws.String("refresh_token"),
	})
	if err != nil {
		var apiErr smithy.APIError
		if errors.As(err, &apiErr) {
			switch apiErr.ErrorCode() {
			case "InvalidGrantException", "InvalidClientException", "UnauthorizedClientException", "ExpiredTokenException":
				return ssoToken{}, false, nil
			}
		}
		// SDK errors may contain request/response details; do not expose secrets.
		return ssoToken{}, false, errors.New("SSO token refresh failed; retry later")
	}
	if resp == nil {
		return ssoToken{}, false, errors.New("SSO token refresh returned an invalid token")
	}
	token := ssoToken{
		AccessToken: aws.ToString(resp.AccessToken),
		ExpiresAt:   time.Now().UTC().Add(time.Duration(resp.ExpiresIn) * time.Second).Format(time.RFC3339),
	}
	if !token.isValid() {
		return ssoToken{}, false, errors.New("SSO token refresh returned an invalid token")
	}
	// Retain all AWS CLI metadata, including a refresh token omitted by AWS.
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return ssoToken{}, false, errors.New("reading SSO refresh cache failed")
	}
	fields["accessToken"], _ = json.Marshal(token.AccessToken)
	fields["expiresAt"], _ = json.Marshal(token.ExpiresAt)
	if aws.ToString(resp.RefreshToken) != "" {
		fields["refreshToken"], _ = json.Marshal(*resp.RefreshToken)
	}
	if err := writeSSOTokenCache(tokenCachePath(sessionName), fields); err != nil {
		return ssoToken{}, false, errors.New("saving refreshed SSO token failed")
	}
	return token, true, nil
}

// writeSSOTokenCache replaces the cache only after a complete, private file is written.
func writeSSOTokenCache(path string, fields map[string]json.RawMessage) error {
	file, err := os.CreateTemp(filepath.Dir(path), ".quikstrate-sso-*")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	if err := json.NewEncoder(file).Encode(fields); err != nil {
		file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	return os.Rename(file.Name(), path)
}

// getSSOToken reuses or silently refreshes the cache before requesting browser login.
func getSSOToken(sessionName, startURL, region string) (ssoToken, error) {
	token, err := readSSOToken(sessionName)
	if err == nil {
		if token.isValid() {
			return token, nil
		}
		if refreshedToken, refreshed, err := refreshSSOToken(sessionName, region); err != nil {
			return ssoToken{}, err
		} else if refreshed {
			return refreshedToken, nil
		}
	}
	if err := loginSSOSession(sessionName, startURL, region); err != nil {
		return ssoToken{}, err
	}
	token, err = readSSOToken(sessionName)
	if err != nil {
		return ssoToken{}, fmt.Errorf("reading SSO token after login: %w", err)
	}
	return token, nil
}

func runSSOLogin(sessionName, startURL, region string) error {
	if err := writeSSOSessionConfig(sessionName, startURL, region); err != nil {
		return err
	}
	if err := checkAWSCLIVersion(); err != nil {
		return err
	}
	cmd := awsCommand("aws", "sso", "login", "--sso-session", sessionName)
	cmd.Stdin = os.Stdin
	var loginOutput bytes.Buffer
	cmd.Stdout = &loginOutput
	cmd.Stderr = &loginOutput
	if err := cmd.Run(); err != nil {
		details := strings.TrimSpace(loginOutput.String())
		if details != "" {
			return fmt.Errorf("aws sso login --sso-session %s: %w: %s", sessionName, err, details)
		}
		return fmt.Errorf("aws sso login --sso-session %s: %w", sessionName, err)
	}
	return nil
}

func checkAWSCLIVersion() error {
	output, err := awsVersionOutput()
	if err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			return fmt.Errorf("AWS CLI executable not found; install AWS CLI v2.9.0 or newer: %s", awsCLIVersionHelp)
		}
		return fmt.Errorf("running aws --version: %w", err)
	}
	match := regexp.MustCompile(`aws-cli/([0-9][^ ]*)`).FindStringSubmatch(output)
	if len(match) != 2 {
		return fmt.Errorf("could not parse AWS CLI version from %q; install AWS CLI v2.9.0 or newer: %s", strings.TrimSpace(output), awsCLIVersionHelp)
	}
	actual, err := version.NewVersion(match[1])
	if err != nil || actual.Segments()[0] != 2 {
		return fmt.Errorf("could not parse AWS CLI v2 version from %q; install AWS CLI v2.9.0 or newer: %s", match[1], awsCLIVersionHelp)
	}
	required := version.Must(version.NewVersion("2.9.0"))
	if actual.LessThan(required) {
		return fmt.Errorf("AWS CLI %s is too old; AWS CLI v2.9.0 or newer is required for Identity Center. Upgrade: %s. USE_SUBSTRATE=true is a temporary fallback while Substrate remains available", actual, awsCLIVersionHelp)
	}
	return nil
}

var (
	errPermissionSetNotAvailable          = errors.New("permission set not available")
	errPermissionSetUnavailableEverywhere = errors.New("permission set unavailable in all configured IDC instances")
)

func isUnmodeledForbidden(err error) bool {
	var apiErr smithy.APIError
	return errors.As(err, &apiErr) && apiErr.ErrorCode() == "ForbiddenException"
}

// getSSORoleCredentials exchanges an SSO access token for short-lived IAM credentials
func getSSORoleCredentials(sessionName, startURL, region, accountID, roleName string) (Credentials, error) {
	token, err := getSSOToken(sessionName, startURL, region)
	if err != nil {
		return Credentials{}, err
	}

	creds, err := exchangeSSOToken(region, accountID, roleName, token)
	if err == nil {
		return creds, nil
	}

	var unauthorized *types.UnauthorizedException
	var notFound *types.ResourceNotFoundException
	if errors.As(err, &unauthorized) || errors.As(err, &notFound) || isUnmodeledForbidden(err) {
		return Credentials{}, fmt.Errorf("%w: no %q permission set for account %s", errPermissionSetNotAvailable, roleName, accountID)
	}
	return Credentials{}, fmt.Errorf("GetRoleCredentials: %w", err)
}

func exchangeSSOToken(region, accountID, roleName string, token ssoToken) (Credentials, error) {
	cfg, err := awsconfig.LoadDefaultConfig(context.Background(), awsconfig.WithRegion(region))
	if err != nil {
		return Credentials{}, err
	}

	resp, err := sso.NewFromConfig(cfg).GetRoleCredentials(context.Background(), &sso.GetRoleCredentialsInput{
		AccountId:   aws.String(accountID),
		RoleName:    aws.String(roleName),
		AccessToken: aws.String(token.AccessToken),
	})
	if err != nil {
		return Credentials{}, err
	}

	return Credentials{
		AccessKeyId:     aws.ToString(resp.RoleCredentials.AccessKeyId),
		SecretAccessKey: aws.ToString(resp.RoleCredentials.SecretAccessKey),
		SessionToken:    aws.ToString(resp.RoleCredentials.SessionToken),
		Expiration:      time.UnixMilli(resp.RoleCredentials.Expiration),
		Version:         1,
	}, nil
}

func assumeIAMRole(baseCreds Credentials, accountID, roleName string) (Credentials, error) {
	cfg, err := awsconfig.LoadDefaultConfig(
		context.Background(),
		awsconfig.WithRegion("us-west-2"),
		awsconfig.WithCredentialsProvider(awscredentials.NewStaticCredentialsProvider(
			baseCreds.AccessKeyId,
			baseCreds.SecretAccessKey,
			baseCreds.SessionToken,
		)),
	)
	if err != nil {
		return Credentials{}, err
	}

	sessionName := os.Getenv("USER")
	if sessionName == "" {
		sessionName = "quikstrate"
	}
	resp, err := sts.NewFromConfig(cfg).AssumeRole(context.Background(), &sts.AssumeRoleInput{
		RoleArn:         aws.String(fmt.Sprintf("arn:aws:iam::%s:role/%s", accountID, roleName)),
		RoleSessionName: aws.String(sessionName),
	})
	if err != nil {
		return Credentials{}, err
	}

	return Credentials{
		AccessKeyId:     aws.ToString(resp.Credentials.AccessKeyId),
		SecretAccessKey: aws.ToString(resp.Credentials.SecretAccessKey),
		SessionToken:    aws.ToString(resp.Credentials.SessionToken),
		Expiration:      aws.ToTime(resp.Credentials.Expiration),
		Version:         1,
	}, nil
}

func getMetronomeSSORoleCredentials(accountID, roleName string) (Credentials, error) {
	return getIDCCredentialsForInstance(metronomeIDC, accountID, roleName)
}
