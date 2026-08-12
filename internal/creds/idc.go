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

// tokenCachePath returns the path the AWS CLI uses for a named sso-session token.
// Filename is sha1(sessionName).json — the same hashing scheme the AWS CLI uses.
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

// writeSSOSessionConfig writes a [sso-session <name>] block to ~/.aws/config.
// Writes directly to the file rather than using `aws configure set` because
// AWS CLI 2.x does not reliably handle the sso-session.* namespace in
// `aws configure set` — it silently writes a garbage key under [default] instead.
func writeSSOSessionConfig(sessionName, startURL, region string) error {
	if configDryrun {
		log.Printf("aws configure set sso-session.%s.sso_start_url %s", sessionName, startURL)
		log.Printf("aws configure set sso-session.%s.sso_region %s", sessionName, region)
		log.Printf("aws configure set sso-session.%s.sso_registration_scopes sso:account:access", sessionName)
		return nil
	}

	block := fmt.Sprintf("[sso-session %s]\nsso_start_url = %s\nsso_region = %s\nsso_registration_scopes = sso:account:access\n\n",
		sessionName, startURL, region)

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

// removeSSOSession strips an existing [sso-session name] block from an AWS
// config file's contents so writeSSOSessionConfig can append a fresh one.
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

// errPermissionSetNotAvailable is returned when the engineer lacks a specific IDC permission
// set, as opposed to IDC being generally unavailable (network error, expired token, etc.).
// Callers can use errors.Is to distinguish and retry with a lower-privilege role.
var errPermissionSetNotAvailable = errors.New("permission set not available")

// isPermissionDenied returns true for unmodeled 403 ForbiddenException responses from the
// SSO API, which the SDK doesn't map to a typed error but surfaces as a smithy.APIError.
func isPermissionDenied(err error) bool {
	var apiErr smithy.APIError
	return errors.As(err, &apiErr) && apiErr.ErrorCode() == "ForbiddenException"
}

// getSSORoleCredentials exchanges an SSO access token for short-lived IAM credentials
// for the given account and permission set. Caching is handled by the caller.
func getSSORoleCredentials(sessionName, startURL, region, accountID, roleName string) (Credentials, error) {
	token, err := getSSOToken(sessionName, startURL, region)
	if err != nil {
		return Credentials{}, err
	}

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
		// Distinguish "you don't have this permission set" from other errors so the
		// message points the engineer at LMS rather than showing a raw AWS error.
		var unauthorized *types.UnauthorizedException
		var notFound *types.ResourceNotFoundException
		if errors.As(err, &unauthorized) || errors.As(err, &notFound) || isPermissionDenied(err) {
			return Credentials{}, fmt.Errorf("%w: no %q permission set for account %s", errPermissionSetNotAvailable, roleName, accountID)
		}
		return Credentials{}, fmt.Errorf("GetRoleCredentials: %w", err)
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
//
// Primary credential source. Engineers authenticate via Shibboleth (Stripe SSO)
// at https://d-9267463e84.awsapps.com/start, gated by the https://go/ldapg/access-metronome-aws-admin LMS permission.
// This instance will be deprecated when Metronome accounts migrate to the Stripe AWS org (Oct 2026).

const (
	metronomeIDCStartURL    = "https://d-9267463e84.awsapps.com/start"
	metronomeIDCRegion      = "us-west-2"
	metronomeIDCSessionName = "metronome"
)

func getMetronomeSSORoleCredentials(accountID, roleName string) (Credentials, error) {
	return getSSORoleCredentials(metronomeIDCSessionName, metronomeIDCStartURL, metronomeIDCRegion, accountID, roleName)
}
