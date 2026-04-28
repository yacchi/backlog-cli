//go:build windows

package config

import (
	"fmt"

	"github.com/danieljoos/wincred"
)

type systemCredentialSecretStore struct {
	service string
}

func newSystemCredentialSecretStore() credentialSecretStore {
	return &systemCredentialSecretStore{service: credentialKeyringService}
}

func (s *systemCredentialSecretStore) IsAvailable() bool {
	return true
}

func (s *systemCredentialSecretStore) Get(ref string) (string, error) {
	cred, err := wincred.GetGenericCredential(s.targetName(ref))
	if err != nil {
		if err == wincred.ErrElementNotFound {
			return "", errCredentialSecretNotFound
		}
		return "", fmt.Errorf("read Windows credential %q: %w", ref, err)
	}
	return string(cred.CredentialBlob), nil
}

func (s *systemCredentialSecretStore) Set(ref string, secret string, meta credentialSecretMetadata) error {
	cred := wincred.NewGenericCredential(s.targetName(ref))
	cred.UserName = credentialSecretPrincipal(meta)
	cred.TargetAlias = credentialSecretLabel(meta)
	cred.Comment = credentialSecretComment(meta)
	cred.CredentialBlob = []byte(secret)
	if err := cred.Write(); err != nil {
		return fmt.Errorf("write Windows credential %q: %w", ref, err)
	}
	return nil
}

func (s *systemCredentialSecretStore) Delete(ref string) error {
	cred, err := wincred.GetGenericCredential(s.targetName(ref))
	if err != nil {
		if err == wincred.ErrElementNotFound {
			return errCredentialSecretNotFound
		}
		return fmt.Errorf("read Windows credential %q for delete: %w", ref, err)
	}
	if err := cred.Delete(); err != nil {
		return fmt.Errorf("delete Windows credential %q: %w", ref, err)
	}
	return nil
}

func (s *systemCredentialSecretStore) targetName(ref string) string {
	return s.service + ":" + ref
}

func credentialSecretPrincipal(meta credentialSecretMetadata) string {
	switch {
	case meta.UserEmail != "":
		return meta.UserEmail
	case meta.UserName != "":
		return meta.UserName
	default:
		return normalizeCredentialProfileName(meta.ProfileName)
	}
}
