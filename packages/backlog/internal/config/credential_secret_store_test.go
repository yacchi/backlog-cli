package config

import "testing"

type mockCredentialSecretStore struct {
	available bool
	records   map[string]mockCredentialSecretRecord
}

type mockCredentialSecretRecord struct {
	Secret   string
	Metadata credentialSecretMetadata
}

func useMockCredentialSecretStore(t *testing.T, available bool) *mockCredentialSecretStore {
	t.Helper()

	store := &mockCredentialSecretStore{
		available: available,
		records:   make(map[string]mockCredentialSecretRecord),
	}

	previous := newCredentialSecretStore
	newCredentialSecretStore = func() credentialSecretStore {
		return store
	}
	t.Cleanup(func() {
		newCredentialSecretStore = previous
	})

	return store
}

func (s *mockCredentialSecretStore) IsAvailable() bool {
	return s.available
}

func (s *mockCredentialSecretStore) Get(ref string) (string, error) {
	record, ok := s.records[ref]
	if !ok {
		return "", errCredentialSecretNotFound
	}
	return record.Secret, nil
}

func (s *mockCredentialSecretStore) Set(ref string, secret string, meta credentialSecretMetadata) error {
	s.records[ref] = mockCredentialSecretRecord{
		Secret:   secret,
		Metadata: meta,
	}
	return nil
}

func (s *mockCredentialSecretStore) Delete(ref string) error {
	if _, ok := s.records[ref]; !ok {
		return errCredentialSecretNotFound
	}
	delete(s.records, ref)
	return nil
}

func readStoredCredentialSecret(t *testing.T, store *mockCredentialSecretStore, ref string) mockCredentialSecretRecord {
	t.Helper()

	record, ok := store.records[ref]
	if !ok {
		t.Fatalf("stored credential %q not found", ref)
	}
	return record
}
