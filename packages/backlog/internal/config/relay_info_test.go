package config

import (
	"bytes"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"testing"
	"time"
)

func TestVerifyRelayInfoSignature(t *testing.T) {
	seed := bytes.Repeat([]byte{0x2}, ed25519.SeedSize)
	privKey := ed25519.NewKeyFromSeed(seed)
	pubKey := privKey.Public().(ed25519.PublicKey)

	payloadJSON := []byte(`{"relay_url":"https://relay.example.com"}`)
	payload := base64.RawURLEncoding.EncodeToString(payloadJSON)

	protectedHeader := map[string]string{
		"alg": "EdDSA",
		"kid": "k1",
	}
	protectedBytes, err := json.Marshal(protectedHeader)
	if err != nil {
		t.Fatalf("marshal protected header failed: %v", err)
	}
	protected := base64.RawURLEncoding.EncodeToString(protectedBytes)

	signingInput := []byte(protected + "." + payload)
	signature := ed25519.Sign(privKey, signingInput)
	signatureB64 := base64.RawURLEncoding.EncodeToString(signature)

	signatures := []relayBundleJWSSign{
		{
			Protected: protected,
			Signature: signatureB64,
		},
	}

	allowedKeys := map[string]allowedKey{
		"k1": {PublicKey: pubKey},
	}

	if err := verifyRelayInfoSignature(payload, signatures, allowedKeys); err != nil {
		t.Fatalf("verifyRelayInfoSignature failed: %v", err)
	}

	signatures[0].Signature = base64.RawURLEncoding.EncodeToString([]byte("bad"))
	if err := verifyRelayInfoSignature(payload, signatures, allowedKeys); err == nil {
		t.Fatalf("verifyRelayInfoSignature should fail for invalid signature")
	}
}

func TestBuildRelayInfoURL(t *testing.T) {
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
			want:          "https://relay.example.com/v1/relay/tenants/space.backlog.jp/info",
			wantErr:       false,
		},
		{
			name:          "URL with trailing slash",
			relayURL:      "https://relay.example.com/",
			allowedDomain: "space.backlog.jp",
			want:          "https://relay.example.com/v1/relay/tenants/space.backlog.jp/info",
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
			got, err := BuildRelayInfoURL(tt.relayURL, tt.allowedDomain)
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

func TestBuildRelayBundleURL(t *testing.T) {
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
			want:          "https://relay.example.com/v1/relay/tenants/space.backlog.jp/bundle",
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
			got, err := BuildRelayBundleURL(tt.relayURL, tt.allowedDomain)
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

func TestRelayURLMatches(t *testing.T) {
	tests := []struct {
		name     string
		expected string
		actual   string
		want     bool
	}{
		{
			name:     "exact match",
			expected: "https://relay.example.com",
			actual:   "https://relay.example.com",
			want:     true,
		},
		{
			name:     "trailing slash difference",
			expected: "https://relay.example.com",
			actual:   "https://relay.example.com/",
			want:     true,
		},
		{
			name:     "both have trailing slash",
			expected: "https://relay.example.com/",
			actual:   "https://relay.example.com/",
			want:     true,
		},
		{
			name:     "different hosts",
			expected: "https://relay.example.com",
			actual:   "https://other.example.com",
			want:     false,
		},
		{
			name:     "different schemes",
			expected: "https://relay.example.com",
			actual:   "http://relay.example.com",
			want:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := relayURLMatches(tt.expected, tt.actual)
			if got != tt.want {
				t.Errorf("relayURLMatches(%q, %q) = %v, want %v", tt.expected, tt.actual, got, tt.want)
			}
		})
	}
}

func TestCheckBundleUpdate(t *testing.T) {
	tests := []struct {
		name        string
		info        *RelayInfoPayload
		bundle      *TrustedBundle
		force       bool
		wantErr     bool
		wantErrType string
	}{
		{
			name:    "nil info",
			info:    nil,
			bundle:  &TrustedBundle{},
			wantErr: false,
		},
		{
			name:    "nil bundle",
			info:    &RelayInfoPayload{},
			bundle:  nil,
			wantErr: false,
		},
		{
			name: "no update_before",
			info: &RelayInfoPayload{
				UpdateBefore: "",
			},
			bundle:  &TrustedBundle{IssuedAt: "2025-01-01T00:00:00Z"},
			wantErr: false,
		},
		{
			name: "bundle is newer than update_before",
			info: &RelayInfoPayload{
				UpdateBefore: "2025-01-01T00:00:00Z",
			},
			bundle:  &TrustedBundle{IssuedAt: "2025-01-15T00:00:00Z", AllowedDomain: "test.backlog.jp"},
			wantErr: false,
		},
		{
			name: "bundle is older than update_before",
			info: &RelayInfoPayload{
				UpdateBefore: "2025-01-15T00:00:00Z",
			},
			bundle:      &TrustedBundle{IssuedAt: "2025-01-01T00:00:00Z", AllowedDomain: "test.backlog.jp"},
			wantErr:     true,
			wantErrType: "BundleUpdateRequiredError",
		},
		{
			name: "force update",
			info: &RelayInfoPayload{
				UpdateBefore: "",
			},
			bundle:      &TrustedBundle{IssuedAt: "2025-01-01T00:00:00Z", AllowedDomain: "test.backlog.jp"},
			force:       true,
			wantErr:     true,
			wantErrType: "BundleUpdateRequiredError",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := CheckBundleUpdate(tt.info, tt.bundle, testTime(), tt.force)
			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error, got nil")
					return
				}
				if tt.wantErrType == "BundleUpdateRequiredError" {
					var updateErr *BundleUpdateRequiredError
					if !errors.As(err, &updateErr) {
						t.Errorf("expected BundleUpdateRequiredError, got %T", err)
					}
				}
			} else if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func testTime() time.Time {
	t, _ := time.Parse(time.RFC3339, "2025-01-10T00:00:00Z")
	return t
}
