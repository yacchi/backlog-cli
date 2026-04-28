package config

import (
	"strings"
	"testing"
)

func TestCredentialSecretLabel(t *testing.T) {
	got := credentialSecretLabel(credentialSecretMetadata{
		ProfileName: DefaultProfile,
		UserName:    "Fujie",
		UserEmail:   "fujie@example.com",
		AuthType:    AuthTypeOAuth,
	})
	if got != credentialKeyringService {
		t.Fatalf("credentialSecretLabel() = %q, want %q", got, credentialKeyringService)
	}
}

func TestCredentialSecretComment(t *testing.T) {
	got := credentialSecretComment(credentialSecretMetadata{
		ProfileName: DefaultProfile,
		UserName:    "Fujie",
		UserEmail:   "fujie@example.com",
		Space:       "team",
		Domain:      "backlog.jp",
		AuthType:    AuthTypeOAuth,
	})
	if !strings.Contains(got, `profile "default"`) {
		t.Fatalf("credentialSecretComment() = %q, want profile", got)
	}
	if !strings.Contains(got, "space=team.backlog.jp") {
		t.Fatalf("credentialSecretComment() = %q, want space URL", got)
	}
	if !strings.Contains(got, "user=Fujie <fujie@example.com>") {
		t.Fatalf("credentialSecretComment() = %q, want user identity", got)
	}
}
