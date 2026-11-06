package creds

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ssooidc"
	oidctypes "github.com/aws/aws-sdk-go-v2/service/ssooidc/types"
	"github.com/aws/smithy-go"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteSSOSessionConfigCreatesAWSConfigDirectory(t *testing.T) {
	previousAWSConfigFile := awsConfigFile
	previousDryrun := configDryrun
	t.Cleanup(func() {
		awsConfigFile = previousAWSConfigFile
		configDryrun = previousDryrun
	})

	awsConfigFile = filepath.Join(t.TempDir(), ".aws", "config")
	configDryrun = false

	if err := writeSSOSessionConfig("metronome", "https://example.com/start", "us-west-2"); err != nil {
		t.Fatalf("writeSSOSessionConfig() error = %v", err)
	}

	contents, err := os.ReadFile(awsConfigFile)
	if err != nil {
		t.Fatalf("reading created AWS config: %v", err)
	}
	if !strings.Contains(string(contents), "[sso-session metronome]") {
		t.Fatalf("AWS config does not contain SSO session block:\n%s", contents)
	}
}

func TestPreserveAWSAuthEnabled(t *testing.T) {
	t.Run("disabled by default", func(t *testing.T) {
		t.Setenv("QUIKSTRATE_PRESERVE_AWS_AUTH", "")
		got, err := preserveAWSAuthEnabled(false)
		if err != nil || got {
			t.Fatalf("preserveAWSAuthEnabled(false) = %v, %v; want false, nil", got, err)
		}
	})

	t.Run("enabled by environment", func(t *testing.T) {
		t.Setenv("QUIKSTRATE_PRESERVE_AWS_AUTH", "true")
		got, err := preserveAWSAuthEnabled(false)
		if err != nil || !got {
			t.Fatalf("preserveAWSAuthEnabled(false) = %v, %v; want true, nil", got, err)
		}
	})

	t.Run("flag takes precedence", func(t *testing.T) {
		t.Setenv("QUIKSTRATE_PRESERVE_AWS_AUTH", "false")
		got, err := preserveAWSAuthEnabled(true)
		if err != nil || !got {
			t.Fatalf("preserveAWSAuthEnabled(true) = %v, %v; want true, nil", got, err)
		}
	})

	t.Run("invalid environment", func(t *testing.T) {
		t.Setenv("QUIKSTRATE_PRESERVE_AWS_AUTH", "sometimes")
		if _, err := preserveAWSAuthEnabled(false); err == nil {
			t.Fatal("preserveAWSAuthEnabled(false) error = nil; want invalid boolean error")
		}
	})
}

// Token tests use only isolated AWS caches and stubbed authentication calls.
func setupSSOTokenTest(t *testing.T) map[string]interface{} {
	t.Helper()
	oldHome, oldCreate, oldLogin := home, createSSOToken, loginSSOSession
	home = t.TempDir()
	t.Cleanup(func() { home, createSSOToken, loginSSOSession = oldHome, oldCreate, oldLogin })
	createSSOToken = func(string, *ssooidc.CreateTokenInput) (*ssooidc.CreateTokenOutput, error) {
		t.Fatal("unexpected refresh")
		return nil, nil
	}
	loginSSOSession = func(string, string, string) error {
		t.Fatal("unexpected login")
		return nil
	}
	return map[string]interface{}{
		"accessToken":           "old-access",
		"expiresAt":             time.Now().Add(-time.Minute).UTC().Format(time.RFC3339),
		"refreshToken":          "old-refresh",
		"clientId":              "client-id",
		"clientSecret":          "client-secret",
		"registrationExpiresAt": time.Now().Add(24 * time.Hour).UTC().Format(time.RFC3339),
		"startUrl":              "https://example.com/start",
		"region":                "us-east-2",
		"extra":                 map[string]interface{}{"number": json.Number("9007199254740993")},
	}
}

func writeTestSSOCache(t *testing.T, fields map[string]interface{}) []byte {
	t.Helper()
	data, err := json.Marshal(fields)
	if err != nil {
		t.Fatal(err)
	}
	path := tokenCachePath("test-session")
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	return data
}

func TestGetSSOTokenValidCache(t *testing.T) {
	fields := setupSSOTokenTest(t)
	fields["expiresAt"] = time.Now().Add(time.Hour).UTC().Format(time.RFC3339)
	writeTestSSOCache(t, fields)
	token, err := getSSOToken("test-session", "https://example.com/start", "us-east-2")
	if err != nil || token.AccessToken != "old-access" {
		t.Fatalf("got token/error %v/%v", token.AccessToken, err)
	}
}

func TestGetSSOTokenRefresh(t *testing.T) {
	for _, nearExpiry := range []bool{false, true} {
		for _, rotated := range []bool{false, true} {
			t.Run(fmt.Sprintf("nearExpiry=%v/rotated=%v", nearExpiry, rotated), func(t *testing.T) {
				fields := setupSSOTokenTest(t)
				if nearExpiry {
					fields["expiresAt"] = time.Now().Add(4 * time.Minute).UTC().Format(time.RFC3339)
				}
				// Older CLI timestamp format must work for registration expiry too.
				fields["registrationExpiresAt"] = time.Now().Add(24 * time.Hour).UTC().Format("2006-01-02T15:04:05UTC")
				before := writeTestSSOCache(t, fields)
				calls := 0
				createSSOToken = func(region string, input *ssooidc.CreateTokenInput) (*ssooidc.CreateTokenOutput, error) {
					calls++
					if region != "us-east-2" || aws.ToString(input.GrantType) != "refresh_token" || aws.ToString(input.ClientId) != "client-id" || aws.ToString(input.ClientSecret) != "client-secret" || aws.ToString(input.RefreshToken) != "old-refresh" {
						t.Fatal("incorrect refresh request")
					}
					resp := &ssooidc.CreateTokenOutput{AccessToken: aws.String("new-access"), ExpiresIn: 1800}
					if rotated {
						resp.RefreshToken = aws.String("new-refresh")
					}
					return resp, nil
				}
				started := time.Now()
				token, err := getSSOToken("test-session", "https://example.com/start", "us-east-2")
				if err != nil || token.AccessToken != "new-access" || calls != 1 {
					t.Fatalf("refresh: calls=%d, err=%v", calls, err)
				}
				expiry, err := token.expiry()
				if err != nil || expiry.Before(started.Add(1799*time.Second)) || expiry.After(time.Now().Add(1800*time.Second)) {
					t.Fatal("expiration does not match AWS lifetime")
				}
				after, err := os.ReadFile(tokenCachePath("test-session"))
				if err != nil {
					t.Fatal(err)
				}
				var original, saved map[string]json.RawMessage
				if err := json.Unmarshal(before, &original); err != nil {
					t.Fatal(err)
				}
				if err := json.Unmarshal(after, &saved); err != nil {
					t.Fatal(err)
				}
				for key, value := range original {
					if key == "accessToken" || key == "expiresAt" || (key == "refreshToken" && rotated) {
						continue
					}
					if !bytes.Equal(value, saved[key]) {
						t.Fatalf("metadata %s changed", key)
					}
				}
				wantRefresh := "old-refresh"
				if rotated {
					wantRefresh = "new-refresh"
				}
				if string(saved["refreshToken"]) != strconv.Quote(wantRefresh) {
					t.Fatal("incorrect cached refresh token")
				}
				cached, err := readSSOToken("test-session")
				if err != nil || cached != token {
					t.Fatal("returned and cached tokens differ")
				}
				info, err := os.Stat(tokenCachePath("test-session"))
				if err != nil || info.Mode().Perm() != 0600 {
					t.Fatal("cache must have mode 0600")
				}
			})
		}
	}
}

func TestGetSSOTokenLoginFallback(t *testing.T) {
	for _, scenario := range []string{"missing cache", "malformed cache", "missing refreshToken", "missing clientId", "missing clientSecret", "missing registrationExpiresAt", "invalid registration expiry", "expired registration", "unusable metadata", "InvalidGrantException", "InvalidClientException", "UnauthorizedClientException", "ExpiredTokenException"} {
		t.Run(scenario, func(t *testing.T) {
			fields := setupSSOTokenTest(t)
			switch {
			case strings.HasPrefix(scenario, "missing "):
				delete(fields, strings.TrimPrefix(scenario, "missing "))
			case scenario == "invalid registration expiry":
				fields["registrationExpiresAt"] = "invalid"
			case scenario == "expired registration":
				fields["registrationExpiresAt"] = fields["expiresAt"]
			case scenario == "unusable metadata":
				fields["refreshToken"] = 42
			}
			writeTestSSOCache(t, fields)
			if scenario == "missing cache" {
				if err := os.Remove(tokenCachePath("test-session")); err != nil {
					t.Fatal(err)
				}
			}
			if scenario == "malformed cache" {
				if err := os.WriteFile(tokenCachePath("test-session"), []byte("{"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			refreshes := 0
			if strings.HasSuffix(scenario, "Exception") {
				createSSOToken = func(string, *ssooidc.CreateTokenInput) (*ssooidc.CreateTokenOutput, error) {
					refreshes++
					return nil, fmt.Errorf("wrapped: %w", &smithy.GenericAPIError{Code: scenario, Message: "secret"})
				}
			}
			logins := 0
			loginSSOSession = func(session, startURL, region string) error {
				logins++
				if session != "test-session" || startURL != "https://example.com/start" || region != "us-east-2" {
					t.Fatal("incorrect login routing")
				}
				fields["accessToken"] = "login-access"
				fields["expiresAt"] = time.Now().Add(time.Hour).UTC().Format(time.RFC3339)
				writeTestSSOCache(t, fields)
				return nil
			}
			token, err := getSSOToken("test-session", "https://example.com/start", "us-east-2")
			if err != nil || token.AccessToken != "login-access" || logins != 1 {
				t.Fatalf("login fallback: calls=%d, err=%v", logins, err)
			}
			if strings.HasSuffix(scenario, "Exception") && refreshes != 1 {
				t.Fatal("refresh was not attempted")
			}
		})
	}
}

func TestGetSSOTokenOperationalFailure(t *testing.T) {
	for _, scenario := range []string{"network", "throttling", "service", "unexpected API rejection", "empty access token", "invalid lifetime", "short lifetime", "nil response", "cache write"} {
		t.Run(scenario, func(t *testing.T) {
			fields := setupSSOTokenTest(t)
			before := writeTestSSOCache(t, fields)
			createSSOToken = func(string, *ssooidc.CreateTokenInput) (*ssooidc.CreateTokenOutput, error) {
				resp := &ssooidc.CreateTokenOutput{AccessToken: aws.String("new-access"), ExpiresIn: 1800}
				switch scenario {
				case "network":
					return nil, errors.New("network error containing secret")
				case "throttling":
					return nil, &oidctypes.SlowDownException{Message: aws.String("secret")}
				case "service":
					return nil, &oidctypes.InternalServerException{Message: aws.String("secret")}
				case "unexpected API rejection":
					return nil, &oidctypes.InvalidRequestException{Message: aws.String("secret")}
				case "empty access token":
					resp.AccessToken = nil
				case "invalid lifetime":
					resp.ExpiresIn = -1
				case "short lifetime":
					resp.ExpiresIn = 60
				case "nil response":
					return nil, nil
				case "cache write":
					dir := filepath.Dir(tokenCachePath("test-session"))
					if err := os.Chmod(dir, 0500); err != nil {
						t.Fatal(err)
					}
					t.Cleanup(func() {
						if err := os.Chmod(dir, 0700); err != nil {
							t.Error(err)
						}
					})
				}
				return resp, nil
			}
			_, err := getSSOToken("test-session", "https://example.com/start", "us-east-2")
			if err == nil || strings.Contains(err.Error(), "secret") {
				t.Fatalf("expected sanitized error, got %v", err)
			}
			after, readErr := os.ReadFile(tokenCachePath("test-session"))
			if readErr != nil || !bytes.Equal(before, after) {
				t.Fatal("failed refresh changed old cache")
			}
			entries, readErr := os.ReadDir(filepath.Dir(tokenCachePath("test-session")))
			if readErr != nil || len(entries) != 1 {
				t.Fatal("failed refresh left temporary files")
			}
		})
	}
}

func TestWriteSSOTokenCachePreservesOldCacheOnEncodingFailure(t *testing.T) {
	fields := setupSSOTokenTest(t)
	before := writeTestSSOCache(t, fields)
	if err := writeSSOTokenCache(tokenCachePath("test-session"), map[string]json.RawMessage{"invalid": json.RawMessage("{")}); err == nil {
		t.Fatal("expected encoding error")
	}
	after, err := os.ReadFile(tokenCachePath("test-session"))
	if err != nil || !bytes.Equal(before, after) {
		t.Fatal("encoding failure changed old cache")
	}
	entries, err := os.ReadDir(filepath.Dir(tokenCachePath("test-session")))
	if err != nil || len(entries) != 1 {
		t.Fatal("temporary file not cleaned up")
	}
}
