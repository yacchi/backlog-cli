package config

import (
	"crypto/sha256"
	"encoding/hex"
	"testing"
)

func TestValidateRelayBundleManifest(t *testing.T) {
	tests := []struct {
		name    string
		m       *RelayBundleManifest
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid manifest",
			m: &RelayBundleManifest{
				Version:       1,
				RelayURL:      "https://relay.example.com",
				AllowedDomain: "space.backlog.jp",
				IssuedAt:      "2025-01-10T12:00:00Z",
				ExpiresAt:     "2025-02-10T12:00:00Z",
				RelayKeys:     []RelayBundleKey{{KeyID: "k1", Thumbprint: "abc"}},
			},
			wantErr: false,
		},
		{
			name: "unsupported version",
			m: &RelayBundleManifest{
				Version:       2,
				RelayURL:      "https://relay.example.com",
				AllowedDomain: "space.backlog.jp",
				IssuedAt:      "2025-01-10T12:00:00Z",
				ExpiresAt:     "2025-02-10T12:00:00Z",
				RelayKeys:     []RelayBundleKey{{KeyID: "k1", Thumbprint: "abc"}},
			},
			wantErr: true,
			errMsg:  "unsupported manifest version",
		},
		{
			name: "missing relay_url",
			m: &RelayBundleManifest{
				Version:       1,
				AllowedDomain: "space.backlog.jp",
				IssuedAt:      "2025-01-10T12:00:00Z",
				ExpiresAt:     "2025-02-10T12:00:00Z",
				RelayKeys:     []RelayBundleKey{{KeyID: "k1", Thumbprint: "abc"}},
			},
			wantErr: true,
			errMsg:  "relay_url is required",
		},
		{
			name: "missing allowed_domain",
			m: &RelayBundleManifest{
				Version:   1,
				RelayURL:  "https://relay.example.com",
				IssuedAt:  "2025-01-10T12:00:00Z",
				ExpiresAt: "2025-02-10T12:00:00Z",
				RelayKeys: []RelayBundleKey{{KeyID: "k1", Thumbprint: "abc"}},
			},
			wantErr: true,
			errMsg:  "allowed_domain is required",
		},
		{
			name: "missing issued_at",
			m: &RelayBundleManifest{
				Version:       1,
				RelayURL:      "https://relay.example.com",
				AllowedDomain: "space.backlog.jp",
				ExpiresAt:     "2025-02-10T12:00:00Z",
				RelayKeys:     []RelayBundleKey{{KeyID: "k1", Thumbprint: "abc"}},
			},
			wantErr: true,
			errMsg:  "issued_at is required",
		},
		{
			name: "missing expires_at",
			m: &RelayBundleManifest{
				Version:       1,
				RelayURL:      "https://relay.example.com",
				AllowedDomain: "space.backlog.jp",
				IssuedAt:      "2025-01-10T12:00:00Z",
				RelayKeys:     []RelayBundleKey{{KeyID: "k1", Thumbprint: "abc"}},
			},
			wantErr: true,
			errMsg:  "expires_at is required",
		},
		{
			name: "empty relay_keys",
			m: &RelayBundleManifest{
				Version:       1,
				RelayURL:      "https://relay.example.com",
				AllowedDomain: "space.backlog.jp",
				IssuedAt:      "2025-01-10T12:00:00Z",
				ExpiresAt:     "2025-02-10T12:00:00Z",
				RelayKeys:     []RelayBundleKey{},
			},
			wantErr: true,
			errMsg:  "relay_keys is required",
		},
		{
			name: "relay_key missing key_id",
			m: &RelayBundleManifest{
				Version:       1,
				RelayURL:      "https://relay.example.com",
				AllowedDomain: "space.backlog.jp",
				IssuedAt:      "2025-01-10T12:00:00Z",
				ExpiresAt:     "2025-02-10T12:00:00Z",
				RelayKeys:     []RelayBundleKey{{KeyID: "", Thumbprint: "abc"}},
			},
			wantErr: true,
			errMsg:  "relay_keys entries must include key_id and thumbprint",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateRelayBundleManifest(tt.m)
			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error containing %q, got nil", tt.errMsg)
				} else if tt.errMsg != "" && err.Error() != tt.errMsg && !contains(err.Error(), tt.errMsg) {
					t.Errorf("expected error containing %q, got %q", tt.errMsg, err.Error())
				}
			} else if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && (s[:len(substr)] == substr || contains(s[1:], substr)))
}

func TestBuildRelayCertsURL(t *testing.T) {
	tests := []struct {
		name          string
		relayURL      string
		allowedDomain string
		want          string
		wantErr       bool
	}{
		{
			name:          "valid URL",
			relayURL:      "https://relay.example.com",
			allowedDomain: "space.backlog.jp",
			want:          "https://relay.example.com/v1/relay/tenants/space.backlog.jp/certs",
			wantErr:       false,
		},
		{
			name:          "URL with trailing slash",
			relayURL:      "https://relay.example.com/",
			allowedDomain: "space.backlog.jp",
			want:          "https://relay.example.com/v1/relay/tenants/space.backlog.jp/certs",
			wantErr:       false,
		},
		{
			name:          "empty relay_url",
			relayURL:      "",
			allowedDomain: "space.backlog.jp",
			wantErr:       true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := buildRelayCertsURL(tt.relayURL, tt.allowedDomain)
			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestVerifyRelayBundleFiles(t *testing.T) {
	content := []byte("test content")
	hash := sha256.Sum256(content)
	hashStr := hex.EncodeToString(hash[:])

	tests := []struct {
		name    string
		files   map[string][]byte
		refs    []RelayBundleFileRef
		wantErr bool
	}{
		{
			name:    "no files to verify",
			files:   map[string][]byte{},
			refs:    []RelayBundleFileRef{},
			wantErr: false,
		},
		{
			name:  "valid file hash",
			files: map[string][]byte{"test.txt": content},
			refs: []RelayBundleFileRef{
				{Name: "test.txt", SHA256: hashStr},
			},
			wantErr: false,
		},
		{
			name:  "case insensitive hash",
			files: map[string][]byte{"test.txt": content},
			refs: []RelayBundleFileRef{
				{Name: "test.txt", SHA256: "9A0364B9E99BB480DD25E1F0284C8555" + hashStr[32:]},
			},
			wantErr: true, // different hash
		},
		{
			name:  "missing file",
			files: map[string][]byte{},
			refs: []RelayBundleFileRef{
				{Name: "missing.txt", SHA256: hashStr},
			},
			wantErr: true,
		},
		{
			name:  "hash mismatch",
			files: map[string][]byte{"test.txt": content},
			refs: []RelayBundleFileRef{
				{Name: "test.txt", SHA256: "0000000000000000000000000000000000000000000000000000000000000000"},
			},
			wantErr: true,
		},
		{
			name:  "empty sha256",
			files: map[string][]byte{"test.txt": content},
			refs: []RelayBundleFileRef{
				{Name: "test.txt", SHA256: ""},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := verifyRelayBundleFiles(tt.files, tt.refs)
			if tt.wantErr && err == nil {
				t.Errorf("expected error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestRelayBundleSignatureVerification(t *testing.T) {
	jwk, _, _ := testRelayBundleJWK()
	jwkByKid := map[string]relayBundleJWK{
		jwk.Kid: jwk,
	}

	manifest := []byte("manifest-contents")
	jwsBytes, err := signRelayBundleManifest(manifest, jwkByKid, []string{jwk.Kid})
	if err != nil {
		t.Fatalf("signRelayBundleManifest failed: %v", err)
	}

	thumbprint, err := jwkThumbprint(jwk)
	if err != nil {
		t.Fatalf("jwkThumbprint failed: %v", err)
	}

	allowedKeys, err := buildAllowedKeys(
		&relayBundleJWKS{Keys: []relayBundleJWK{jwk}},
		[]RelayBundleKey{{KeyID: jwk.Kid, Thumbprint: thumbprint}},
	)
	if err != nil {
		t.Fatalf("buildAllowedKeys failed: %v", err)
	}

	if err := verifyRelayBundleSignature(manifest, jwsBytes, allowedKeys); err != nil {
		t.Fatalf("verifyRelayBundleSignature failed: %v", err)
	}

	if err := verifyRelayBundleSignature([]byte("tampered"), jwsBytes, allowedKeys); err == nil {
		t.Fatalf("verifyRelayBundleSignature should fail for tampered payload")
	}
}
