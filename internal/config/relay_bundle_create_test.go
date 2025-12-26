package config

import (
	"bytes"
	"crypto/ed25519"
	"encoding/base64"
	"strings"
	"testing"
	"time"
)

func testRelayBundleJWK() (relayBundleJWK, ed25519.PrivateKey, ed25519.PublicKey) {
	seed := bytes.Repeat([]byte{0x1}, ed25519.SeedSize)
	privKey := ed25519.NewKeyFromSeed(seed)
	pubKey := privKey.Public().(ed25519.PublicKey)
	jwk := relayBundleJWK{
		Kty: "OKP",
		Crv: "Ed25519",
		Kid: "k1",
		X:   base64.RawURLEncoding.EncodeToString(pubKey),
		D:   base64.RawURLEncoding.EncodeToString(seed),
	}
	return jwk, privKey, pubKey
}

func TestGenerateAndVerifyBundleToken(t *testing.T) {
	jwk, _, _ := testRelayBundleJWK()
	jwkByKid := map[string]relayBundleJWK{
		jwk.Kid: jwk,
	}
	now := time.Unix(1700000000, 0).UTC()

	token, err := GenerateBundleToken("example.backlog.jp", jwkByKid, []string{jwk.Kid}, now)
	if err != nil {
		t.Fatalf("GenerateBundleToken failed: %v", err)
	}

	jwks := relayBundleJWKS{Keys: []relayBundleJWK{jwk}}
	if err := VerifyBundleToken(token, "example.backlog.jp", &jwks); err != nil {
		t.Fatalf("VerifyBundleToken failed: %v", err)
	}

	if err := VerifyBundleToken(token, "other.backlog.jp", &jwks); err == nil {
		t.Fatalf("VerifyBundleToken should fail for wrong subject")
	}

	parts := strings.Split(token, ".")
	parts[2] = "invalid"
	badToken := strings.Join(parts, ".")
	if err := VerifyBundleToken(badToken, "example.backlog.jp", &jwks); err == nil {
		t.Fatalf("VerifyBundleToken should fail for tampered signature")
	}
}

func TestSplitCommaList(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  []string
	}{
		{
			name:  "single item",
			input: "k1",
			want:  []string{"k1"},
		},
		{
			name:  "multiple items",
			input: "k1,k2,k3",
			want:  []string{"k1", "k2", "k3"},
		},
		{
			name:  "items with spaces",
			input: "k1, k2 , k3",
			want:  []string{"k1", "k2", "k3"},
		},
		{
			name:  "empty string",
			input: "",
			want:  []string{},
		},
		{
			name:  "only commas",
			input: ",,,",
			want:  []string{},
		},
		{
			name:  "trailing comma",
			input: "k1,k2,",
			want:  []string{"k1", "k2"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := splitCommaList(tt.input)
			if len(got) != len(tt.want) {
				t.Errorf("got %v, want %v", got, tt.want)
				return
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("got[%d] = %q, want %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestBuildManifestRelayKeys(t *testing.T) {
	jwk1, _, pub1 := testRelayBundleJWK()
	seed2 := bytes.Repeat([]byte{0x2}, ed25519.SeedSize)
	priv2 := ed25519.NewKeyFromSeed(seed2)
	pub2 := priv2.Public().(ed25519.PublicKey)
	jwk2 := relayBundleJWK{
		Kty: "OKP",
		Crv: "Ed25519",
		Kid: "k2",
		X:   base64.RawURLEncoding.EncodeToString(pub2),
		D:   base64.RawURLEncoding.EncodeToString(seed2),
	}
	_ = pub1

	jwkByKid := map[string]relayBundleJWK{
		jwk1.Kid: jwk1,
		jwk2.Kid: jwk2,
	}

	t.Run("active keys only", func(t *testing.T) {
		keys, err := buildManifestRelayKeys(jwkByKid, []string{"k1"}, false)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(keys) != 1 {
			t.Errorf("expected 1 key, got %d", len(keys))
		}
		if keys[0].KeyID != "k1" {
			t.Errorf("expected key_id k1, got %s", keys[0].KeyID)
		}
	})

	t.Run("include all keys", func(t *testing.T) {
		keys, err := buildManifestRelayKeys(jwkByKid, []string{"k1"}, true)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(keys) != 2 {
			t.Errorf("expected 2 keys, got %d", len(keys))
		}
	})

	t.Run("missing key error", func(t *testing.T) {
		_, err := buildManifestRelayKeys(jwkByKid, []string{"missing"}, false)
		if err == nil {
			t.Error("expected error for missing key")
		}
	})

	t.Run("no duplicate keys", func(t *testing.T) {
		keys, err := buildManifestRelayKeys(jwkByKid, []string{"k1", "k1"}, false)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(keys) != 1 {
			t.Errorf("expected 1 key (no duplicates), got %d", len(keys))
		}
	})
}

func TestNormalizeRelayJWKS(t *testing.T) {
	seed := bytes.Repeat([]byte{0x1}, ed25519.SeedSize)
	privKey := ed25519.NewKeyFromSeed(seed)
	pubKey := privKey.Public().(ed25519.PublicKey)
	expectedX := base64.RawURLEncoding.EncodeToString(pubKey)

	t.Run("derive public key from seed", func(t *testing.T) {
		jwks := &relayBundleJWKS{
			Keys: []relayBundleJWK{
				{
					Kty: "OKP",
					Crv: "Ed25519",
					Kid: "k1",
					D:   base64.RawURLEncoding.EncodeToString(seed),
					// X is empty, should be derived
				},
			},
		}
		if err := normalizeRelayJWKS(jwks); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if jwks.Keys[0].X != expectedX {
			t.Errorf("X not derived correctly: got %s, want %s", jwks.Keys[0].X, expectedX)
		}
	})

	t.Run("nil jwks", func(t *testing.T) {
		err := normalizeRelayJWKS(nil)
		if err == nil {
			t.Error("expected error for nil jwks")
		}
	})

	t.Run("key without private key unchanged", func(t *testing.T) {
		jwks := &relayBundleJWKS{
			Keys: []relayBundleJWK{
				{
					Kty: "OKP",
					Crv: "Ed25519",
					Kid: "k1",
					X:   expectedX,
					// D is empty
				},
			},
		}
		if err := normalizeRelayJWKS(jwks); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if jwks.Keys[0].X != expectedX {
			t.Errorf("X should remain unchanged")
		}
	})
}
