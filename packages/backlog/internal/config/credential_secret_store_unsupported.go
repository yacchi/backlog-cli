//go:build !darwin && !windows && !linux && !netbsd && !openbsd && !(freebsd && cgo) && !(dragonfly && cgo)

package config

import "errors"

type systemCredentialSecretStore struct{}

var errCredentialSecretUnavailable = errors.New("credential secret store unavailable")

func newSystemCredentialSecretStore() credentialSecretStore {
	return systemCredentialSecretStore{}
}

func (systemCredentialSecretStore) IsAvailable() bool {
	return false
}

func (systemCredentialSecretStore) Get(string) (string, error) {
	return "", errCredentialSecretUnavailable
}

func (systemCredentialSecretStore) Set(string, string, credentialSecretMetadata) error {
	return errCredentialSecretUnavailable
}

func (systemCredentialSecretStore) Delete(string) error {
	return errCredentialSecretUnavailable
}
