package api

import (
	"testing"

	"github.com/yacchi/backlog-cli/packages/backlog/internal/config"
)

func TestCredentialFromEnv_AccessToken(t *testing.T) {
	t.Setenv("BACKLOG_ACCESS_TOKEN", "test-oauth-token")

	cred := credentialFromEnv()
	if cred == nil {
		t.Fatal("expected credential, got nil")
	}
	if cred.GetAuthType() != config.AuthTypeOAuth {
		t.Errorf("expected AuthTypeOAuth, got %q", cred.GetAuthType())
	}
	if cred.AccessToken != "test-oauth-token" {
		t.Errorf("expected access token %q, got %q", "test-oauth-token", cred.AccessToken)
	}
}

func TestCredentialFromEnv_APIKey(t *testing.T) {
	t.Setenv("BACKLOG_API_KEY", "test-api-key")

	cred := credentialFromEnv()
	if cred == nil {
		t.Fatal("expected credential, got nil")
	}
	if cred.GetAuthType() != config.AuthTypeAPIKey {
		t.Errorf("expected AuthTypeAPIKey, got %q", cred.GetAuthType())
	}
	if cred.APIKey != "test-api-key" {
		t.Errorf("expected api key %q, got %q", "test-api-key", cred.APIKey)
	}
}

func TestCredentialFromEnv_APIKeyTakesPrecedence(t *testing.T) {
	t.Setenv("BACKLOG_ACCESS_TOKEN", "test-oauth-token")
	t.Setenv("BACKLOG_API_KEY", "test-api-key")

	cred := credentialFromEnv()
	if cred == nil {
		t.Fatal("expected credential, got nil")
	}
	if cred.GetAuthType() != config.AuthTypeAPIKey {
		t.Errorf("expected AuthTypeAPIKey when both set, got %q", cred.GetAuthType())
	}
	if cred.APIKey != "test-api-key" {
		t.Errorf("expected api key %q, got %q", "test-api-key", cred.APIKey)
	}
}

func TestCredentialFromEnv_NoEnvVars(t *testing.T) {
	cred := credentialFromEnv()
	if cred != nil {
		t.Errorf("expected nil when no env vars set, got %+v", cred)
	}
}
