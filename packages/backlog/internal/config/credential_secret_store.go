package config

import (
	"errors"
	"fmt"
	"strings"
)

const (
	credentialKeyringService = "github.com/yacchi/backlog-cli"
)

var (
	errCredentialSecretNotFound = errors.New("credential secret not found")
)

type credentialSecretStore interface {
	IsAvailable() bool
	Get(ref string) (string, error)
	Set(ref string, secret string, meta credentialSecretMetadata) error
	Delete(ref string) error
}

var newCredentialSecretStore = newSystemCredentialSecretStore

type credentialSecretMetadata struct {
	ProfileName string
	UserName    string
	UserEmail   string
	Space       string
	Domain      string
	AuthType    AuthType
}

func newCredentialSecretMetadata(profileName string, cred *Credential) credentialSecretMetadata {
	meta := credentialSecretMetadata{
		ProfileName: normalizeCredentialProfileName(profileName),
	}
	if cred == nil {
		return meta
	}

	meta.UserName = strings.TrimSpace(cred.UserName)
	meta.UserEmail = strings.TrimSpace(cred.UserEmail)
	meta.Space = strings.TrimSpace(cred.Space)
	meta.Domain = strings.TrimSpace(cred.Domain)
	meta.AuthType = cred.GetAuthType()
	return meta
}

func normalizeCredentialProfileName(profileName string) string {
	profileName = strings.TrimSpace(profileName)
	if profileName == "" {
		return DefaultProfile
	}
	return profileName
}

func credentialSecretIdentity(meta credentialSecretMetadata) string {
	switch {
	case meta.UserName != "" && meta.UserEmail != "":
		return fmt.Sprintf("%s <%s>", meta.UserName, meta.UserEmail)
	case meta.UserEmail != "":
		return meta.UserEmail
	case meta.UserName != "":
		return meta.UserName
	default:
		return ""
	}
}

func credentialSecretLabel(meta credentialSecretMetadata) string {
	return credentialKeyringService
}

func credentialSecretComment(meta credentialSecretMetadata) string {
	comment := fmt.Sprintf(
		"Backlog CLI credential for profile %q",
		normalizeCredentialProfileName(meta.ProfileName),
	)
	if meta.AuthType != "" {
		comment += " (" + string(meta.AuthType) + ")"
	}
	if spaceURL := credentialSecretSpaceURL(meta); spaceURL != "" {
		comment += "; space=" + spaceURL
	}
	if identity := credentialSecretIdentity(meta); identity != "" {
		comment += "; user=" + identity
	}
	return comment
}

func credentialSecretSpaceURL(meta credentialSecretMetadata) string {
	switch {
	case meta.Space != "" && meta.Domain != "":
		return meta.Space + "." + meta.Domain
	case meta.Space != "":
		return meta.Space
	default:
		return ""
	}
}
