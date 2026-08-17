package creds

import (
	"context"
	"crypto/sha1"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/sso"
	"github.com/aws/aws-sdk-go-v2/service/sso/types"
	smithy "github.com/aws/smithy-go"
)

// ssoToken mirrors the subset of fields the AWS CLI writes to ~/.aws/sso/cache/*.json.
type ssoToken struct {
	AccessToken string `json:"accessToken"`
	ExpiresAt   string `json:"expiresAt"`
}

func (t ssoToken) expiry() (time.Time, error) {
	// AWS CLI writes either RFC3339 "Z" suffix or a bare "UTC" suffix depending on version.
	s := strings.TrimSuffix(t.ExpiresAt, "UTC")
	if !strings.HasSuffix(s, "Z") {
		s += "Z"
	}
	return time.Parse(time.RFC3339, s)
}

func (t ssoToken) isValid() bool {
	exp, err := t.expiry()
	if err != nil {
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

// writeSSOSessionConfig writes a [sso-session metronome] block to ~/.aws/config directly,
// `aws configure set` mishandles the sso-session.* namespace
func writeSSOSessionConfig(sessionName, startURL, region string) error {
	block := fmt.Sprintf("[sso-session %s]\nsso_start_url = %s\nsso_region = %s\nsso_registration_scopes = sso:account:access\n\n",
		sessionName, startURL, region)

	if configDryrun {
		log.Printf("would write to %s:\n%s", awsConfigFile, block)
		return nil
	}

	existing, err := os.ReadFile(awsConfigFile)
	if os.IsNotExist(err) {
		return os.WriteFile(awsConfigFile, []byte(block), 0600)
	}
	if err != nil {
		return fmt.Errorf("reading %s: %w", awsConfigFile, err)
	}

	content := removeSSOSession(string(existing), sessionName) + block
	return os.WriteFile(awsConfigFile, []byte(content), 0600)
}

// removeSSOSession strips the [sso-session metronome] block so writeSSOSessionConfig
// can append a fresh one.
func removeSSOSession(content, sessionName string) string {
	header := "[sso-session " + sessionName + "]"
	var out strings.Builder
	skip := false
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == header {
			skip = true
			continue
		}
		if skip && strings.HasPrefix(trimmed, "[") {
			skip = false
		}
		if !skip {
			out.WriteString(line + "\n")
		}
	}
	return out.String()
}

// getSSOToken returns a valid token for the given session, triggering an
// interactive browser login if the cached token is missing or expired.
func getSSOToken(sessionName, startURL, region string) (ssoToken, error) {
	token, err := readSSOToken(sessionName)
	if err == nil && token.isValid() {
		return token, nil
	}
	if err := writeSSOSessionConfig(sessionName, startURL, region); err != nil {
		return ssoToken{}, err
	}
	cmd := exec.Command("aws", "sso", "login", "--sso-session", sessionName)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stderr // must use stderr so login prompts don't pollute eval $() captures
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return ssoToken{}, fmt.Errorf("aws sso login --sso-session %s: %w", sessionName, err)
	}
	token, err = readSSOToken(sessionName)
	if err != nil {
		return ssoToken{}, fmt.Errorf("reading SSO token after login: %w", err)
	}
	return token, nil
}

var errPermissionSetNotAvailable = errors.New("permission set not available")

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

// ---- Metronome IAM Identity Center ----
// Opt-in credential source
// Gated by the https://go/ldapg/access-metronome-aws-admin
// This instance is retired when Metronome accounts migrate to the Stripe AWS org
const (
	metronomeIDCStartURL    = "https://d-9267463e84.awsapps.com/start"
	metronomeIDCRegion      = "us-west-2"
	metronomeIDCSessionName = "metronome"
)

func getMetronomeSSORoleCredentials(accountID, roleName string) (Credentials, error) {
	return getSSORoleCredentials(metronomeIDCSessionName, metronomeIDCStartURL, metronomeIDCRegion, accountID, roleName)
}
