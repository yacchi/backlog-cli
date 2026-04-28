package config

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"gopkg.in/yaml.v3"
)

func TestResolvedAuthCredentialBackendDefault(t *testing.T) {
	prepareCredentialStoreTest(t)

	store, err := newConfigStore()
	if err != nil {
		t.Fatalf("newConfigStore() error = %v", err)
	}
	if err := store.LoadAll(t.Context()); err != nil {
		t.Fatalf("LoadAll() error = %v", err)
	}

	if got := store.Auth().CredentialBackend; got != CredentialBackendAuto {
		t.Fatalf("Auth().CredentialBackend = %q, want %q", got, CredentialBackendAuto)
	}
}

func TestCredentialFileBackendRoundTrip(t *testing.T) {
	useMockCredentialSecretStore(t, true)
	prepareCredentialStoreTest(t)

	store, err := newConfigStore()
	if err != nil {
		t.Fatalf("newConfigStore() error = %v", err)
	}
	if err := store.LoadAll(t.Context()); err != nil {
		t.Fatalf("LoadAll() error = %v", err)
	}

	if err := store.Set(PathAuthCredentialBackend, string(CredentialBackendFile)); err != nil {
		t.Fatalf("Set(auth.credential_backend) error = %v", err)
	}

	expiresAt := time.Date(2030, 1, 2, 3, 4, 5, 0, time.UTC)
	if err := store.SetCredential(DefaultProfile, &Credential{
		AuthType:     AuthTypeOAuth,
		AccessToken:  "file-access-token",
		RefreshToken: "file-refresh-token",
		ExpiresAt:    expiresAt,
		UserID:       "user-1",
		UserName:     "Alice",
		UserEmail:    "alice@example.com",
		Space:        "alice-space",
		Domain:       "backlog.jp",
	}); err != nil {
		t.Fatalf("SetCredential() error = %v", err)
	}
	if err := store.Save(t.Context()); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	entry := readCredentialMetadataEntry(t, store.GetCredentialsPath(), DefaultProfile)
	if got := entry["access_token"]; got != "file-access-token" {
		t.Fatalf("credentials metadata access_token = %v, want %q", got, "file-access-token")
	}
	if got := entry["refresh_token"]; got != "file-refresh-token" {
		t.Fatalf("credentials metadata refresh_token = %v, want %q", got, "file-refresh-token")
	}
	if _, ok := entry["secret_ref"]; ok {
		t.Fatal("credentials metadata unexpectedly contains secret_ref for file backend")
	}

	reloaded, err := newConfigStore()
	if err != nil {
		t.Fatalf("newConfigStore() reload error = %v", err)
	}
	if err := reloaded.LoadAll(t.Context()); err != nil {
		t.Fatalf("LoadAll() reload error = %v", err)
	}

	cred := reloaded.Credential(DefaultProfile)
	if cred == nil {
		t.Fatal("Credential(default) = nil")
	}
	if cred.AccessToken != "file-access-token" {
		t.Fatalf("Credential().AccessToken = %q, want %q", cred.AccessToken, "file-access-token")
	}
	if cred.RefreshToken != "file-refresh-token" {
		t.Fatalf("Credential().RefreshToken = %q, want %q", cred.RefreshToken, "file-refresh-token")
	}
	if !cred.ExpiresAt.Equal(expiresAt) {
		t.Fatalf("Credential().ExpiresAt = %v, want %v", cred.ExpiresAt, expiresAt)
	}
}

func TestCredentialKeyringBackendRoundTrip(t *testing.T) {
	mockStore := useMockCredentialSecretStore(t, true)
	prepareCredentialStoreTest(t)

	store, err := newConfigStore()
	if err != nil {
		t.Fatalf("newConfigStore() error = %v", err)
	}
	if err := store.LoadAll(t.Context()); err != nil {
		t.Fatalf("LoadAll() error = %v", err)
	}

	if err := store.Set(PathAuthCredentialBackend, string(CredentialBackendKeyring)); err != nil {
		t.Fatalf("Set(auth.credential_backend) error = %v", err)
	}

	expiresAt := time.Date(2031, 2, 3, 4, 5, 6, 0, time.UTC)
	if err := store.SetCredential(DefaultProfile, &Credential{
		AuthType:     AuthTypeOAuth,
		AccessToken:  "keyring-access-token",
		RefreshToken: "keyring-refresh-token",
		ExpiresAt:    expiresAt,
		UserID:       "user-2",
		UserName:     "Bob",
		UserEmail:    "bob@example.com",
		Space:        "bob-space",
		Domain:       "backlog.com",
	}); err != nil {
		t.Fatalf("SetCredential() error = %v", err)
	}
	if err := store.Save(t.Context()); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	entry := readCredentialMetadataEntry(t, store.GetCredentialsPath(), DefaultProfile)
	if _, ok := entry["access_token"]; ok {
		t.Fatal("credentials metadata still contains access_token for keyring backend")
	}
	if _, ok := entry["refresh_token"]; ok {
		t.Fatal("credentials metadata still contains refresh_token for keyring backend")
	}
	if got := entry["secret_ref"]; got != credentialRef(DefaultProfile) {
		t.Fatalf("credentials metadata secret_ref = %v, want %q", got, credentialRef(DefaultProfile))
	}

	record := readStoredCredentialSecret(t, mockStore, credentialRef(DefaultProfile))
	if got := record.Metadata.ProfileName; got != DefaultProfile {
		t.Fatalf("stored metadata profile = %q, want %q", got, DefaultProfile)
	}
	if got := record.Metadata.UserEmail; got != "bob@example.com" {
		t.Fatalf("stored metadata user_email = %q, want %q", got, "bob@example.com")
	}
	if got := record.Metadata.Space; got != "bob-space" {
		t.Fatalf("stored metadata space = %q, want %q", got, "bob-space")
	}
	if got := record.Metadata.Domain; got != "backlog.com" {
		t.Fatalf("stored metadata domain = %q, want %q", got, "backlog.com")
	}
	secretPayload := decodeCredentialSecretPayload(t, record.Secret)
	if got := secretPayload["access_token"]; got != "keyring-access-token" {
		t.Fatalf("keyring payload access_token = %v, want %q", got, "keyring-access-token")
	}
	if got := secretPayload["refresh_token"]; got != "keyring-refresh-token" {
		t.Fatalf("keyring payload refresh_token = %v, want %q", got, "keyring-refresh-token")
	}

	reloaded, err := newConfigStore()
	if err != nil {
		t.Fatalf("newConfigStore() reload error = %v", err)
	}
	if err := reloaded.LoadAll(t.Context()); err != nil {
		t.Fatalf("LoadAll() reload error = %v", err)
	}

	cred := reloaded.Credential(DefaultProfile)
	if cred == nil {
		t.Fatal("Credential(default) = nil")
	}
	if cred.AccessToken != "keyring-access-token" {
		t.Fatalf("Credential().AccessToken = %q, want %q", cred.AccessToken, "keyring-access-token")
	}
	if cred.RefreshToken != "keyring-refresh-token" {
		t.Fatalf("Credential().RefreshToken = %q, want %q", cred.RefreshToken, "keyring-refresh-token")
	}
	if !cred.ExpiresAt.Equal(expiresAt) {
		t.Fatalf("Credential().ExpiresAt = %v, want %v", cred.ExpiresAt, expiresAt)
	}
}

func TestCredentialPlaintextMigratesToKeyring(t *testing.T) {
	mockStore := useMockCredentialSecretStore(t, true)
	xdgHome := prepareCredentialStoreTest(t)
	writeUserConfig(t, xdgHome, "auth:\n  credential_backend: keyring\n")
	writeCredentialsFile(t, xdgHome, `credential:
  default:
    auth_type: oauth
    access_token: legacy-access-token
    refresh_token: legacy-refresh-token
    expires_at: 2032-03-04T05:06:07Z
    user_id: legacy-user
    user_name: Legacy User
`)

	store, err := newConfigStore()
	if err != nil {
		t.Fatalf("newConfigStore() error = %v", err)
	}
	if err := store.LoadAll(t.Context()); err != nil {
		t.Fatalf("LoadAll() error = %v", err)
	}
	if err := store.Save(t.Context()); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	entry := readCredentialMetadataEntry(t, store.GetCredentialsPath(), DefaultProfile)
	if got := entry["secret_ref"]; got != credentialRef(DefaultProfile) {
		t.Fatalf("credentials metadata secret_ref = %v, want %q", got, credentialRef(DefaultProfile))
	}
	if _, ok := entry["access_token"]; ok {
		t.Fatal("credentials metadata still contains access_token after keyring migration")
	}

	record := readStoredCredentialSecret(t, mockStore, credentialRef(DefaultProfile))
	secretPayload := decodeCredentialSecretPayload(t, record.Secret)
	if got := secretPayload["access_token"]; got != "legacy-access-token" {
		t.Fatalf("keyring payload access_token = %v, want %q", got, "legacy-access-token")
	}
	if got := secretPayload["refresh_token"]; got != "legacy-refresh-token" {
		t.Fatalf("keyring payload refresh_token = %v, want %q", got, "legacy-refresh-token")
	}
}

func TestCredentialKeyringMigratesBackToFile(t *testing.T) {
	mockStore := useMockCredentialSecretStore(t, true)
	prepareCredentialStoreTest(t)

	store, err := newConfigStore()
	if err != nil {
		t.Fatalf("newConfigStore() error = %v", err)
	}
	if err := store.LoadAll(t.Context()); err != nil {
		t.Fatalf("LoadAll() error = %v", err)
	}

	if err := store.Set(PathAuthCredentialBackend, string(CredentialBackendKeyring)); err != nil {
		t.Fatalf("Set(auth.credential_backend=keyring) error = %v", err)
	}
	if err := store.SetCredential(DefaultProfile, &Credential{
		AuthType:     AuthTypeOAuth,
		AccessToken:  "migrating-access-token",
		RefreshToken: "migrating-refresh-token",
		ExpiresAt:    time.Date(2033, 4, 5, 6, 7, 8, 0, time.UTC),
		UserID:       "user-3",
		UserName:     "Carol",
		UserEmail:    "carol@example.com",
		Space:        "carol-space",
		Domain:       "backlog.jp",
	}); err != nil {
		t.Fatalf("SetCredential() error = %v", err)
	}
	if err := store.Save(t.Context()); err != nil {
		t.Fatalf("Save() initial error = %v", err)
	}

	reloaded, err := newConfigStore()
	if err != nil {
		t.Fatalf("newConfigStore() reload error = %v", err)
	}
	if err := reloaded.LoadAll(t.Context()); err != nil {
		t.Fatalf("LoadAll() reload error = %v", err)
	}

	if err := reloaded.Set(PathAuthCredentialBackend, string(CredentialBackendFile)); err != nil {
		t.Fatalf("Set(auth.credential_backend=file) error = %v", err)
	}
	if err := reloaded.Save(t.Context()); err != nil {
		t.Fatalf("Save() migration error = %v", err)
	}

	entry := readCredentialMetadataEntry(t, reloaded.GetCredentialsPath(), DefaultProfile)
	if got := entry["access_token"]; got != "migrating-access-token" {
		t.Fatalf("credentials metadata access_token = %v, want %q", got, "migrating-access-token")
	}
	if got := entry["refresh_token"]; got != "migrating-refresh-token" {
		t.Fatalf("credentials metadata refresh_token = %v, want %q", got, "migrating-refresh-token")
	}
	if _, ok := entry["secret_ref"]; ok {
		t.Fatal("credentials metadata still contains secret_ref after file migration")
	}

	_, err = mockStore.Get(credentialRef(DefaultProfile))
	if !errors.Is(err, errCredentialSecretNotFound) {
		t.Fatalf("mockStore.Get() error = %v, want errCredentialSecretNotFound", err)
	}
}

func prepareCredentialStoreTest(t *testing.T) string {
	t.Helper()

	xdgHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", xdgHome)

	workDir := filepath.Join(xdgHome, "work")
	if err := os.MkdirAll(workDir, 0755); err != nil {
		t.Fatalf("MkdirAll(%q) error = %v", workDir, err)
	}

	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd() error = %v", err)
	}
	if err := os.Chdir(workDir); err != nil {
		t.Fatalf("Chdir(%q) error = %v", workDir, err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(oldWD)
	})

	return xdgHome
}

func writeUserConfig(t *testing.T, xdgHome, body string) {
	t.Helper()

	configDir := filepath.Join(xdgHome, AppName)
	if err := os.MkdirAll(configDir, 0755); err != nil {
		t.Fatalf("MkdirAll(%q) error = %v", configDir, err)
	}
	path := filepath.Join(configDir, "config.yaml")
	if err := os.WriteFile(path, []byte(body), 0644); err != nil {
		t.Fatalf("WriteFile(%q) error = %v", path, err)
	}
}

func writeCredentialsFile(t *testing.T, xdgHome, body string) {
	t.Helper()

	configDir := filepath.Join(xdgHome, AppName)
	if err := os.MkdirAll(configDir, 0755); err != nil {
		t.Fatalf("MkdirAll(%q) error = %v", configDir, err)
	}
	path := filepath.Join(configDir, "credentials.yaml")
	if err := os.WriteFile(path, []byte(body), 0600); err != nil {
		t.Fatalf("WriteFile(%q) error = %v", path, err)
	}
}

func readCredentialMetadataEntry(t *testing.T, path, profile string) map[string]any {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", path, err)
	}

	var doc struct {
		Credential map[string]map[string]any `yaml:"credential"`
	}
	if err := yaml.Unmarshal(data, &doc); err != nil {
		t.Fatalf("yaml.Unmarshal(%q) error = %v", path, err)
	}
	return doc.Credential[profile]
}

func decodeCredentialSecretPayload(t *testing.T, value string) map[string]any {
	t.Helper()

	var payload map[string]any
	if err := json.Unmarshal([]byte(value), &payload); err != nil {
		t.Fatalf("json.Unmarshal(keyring payload) error = %v", err)
	}
	return payload
}
