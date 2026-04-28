//go:build darwin

package config

import (
	"fmt"
	"os/exec"
	"strings"
)

const macOSKeychainCommand = "/usr/bin/security"

type systemCredentialSecretStore struct {
	service string
}

func newSystemCredentialSecretStore() credentialSecretStore {
	return &systemCredentialSecretStore{service: credentialKeyringService}
}

func (s *systemCredentialSecretStore) IsAvailable() bool {
	_, err := exec.LookPath(macOSKeychainCommand)
	return err == nil
}

func (s *systemCredentialSecretStore) Get(ref string) (string, error) {
	out, err := exec.Command(
		macOSKeychainCommand,
		"find-generic-password",
		"-s", s.service,
		"-a", ref,
		"-w",
	).CombinedOutput()
	if err != nil {
		if strings.Contains(string(out), "could not be found") {
			return "", errCredentialSecretNotFound
		}
		return "", fmt.Errorf("read macOS keychain item %q: %w", ref, err)
	}

	return strings.TrimRight(string(out), "\r\n"), nil
}

func (s *systemCredentialSecretStore) Set(ref string, secret string, meta credentialSecretMetadata) error {
	out, err := exec.Command(
		macOSKeychainCommand,
		"add-generic-password",
		"-U",
		"-s", s.service,
		"-a", ref,
		"-l", credentialSecretLabel(meta),
		"-j", credentialSecretComment(meta),
		"-w", secret,
	).CombinedOutput()
	if err != nil {
		return fmt.Errorf("write macOS keychain item %q: %w: %s", ref, err, strings.TrimSpace(string(out)))
	}
	return nil
}

func (s *systemCredentialSecretStore) Delete(ref string) error {
	out, err := exec.Command(
		macOSKeychainCommand,
		"delete-generic-password",
		"-s", s.service,
		"-a", ref,
	).CombinedOutput()
	if err != nil {
		if strings.Contains(string(out), "could not be found") {
			return errCredentialSecretNotFound
		}
		return fmt.Errorf("delete macOS keychain item %q: %w", ref, err)
	}
	return nil
}
