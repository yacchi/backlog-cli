package config

import (
	"encoding/base64"
	"encoding/json"
	"testing"
)

func makeTestProvisioningToken(claims map[string]interface{}) string {
	header := map[string]string{"alg": "EdDSA", "typ": "JWT", "kid": "test-key"}
	headerJSON, _ := json.Marshal(header)
	claimsJSON, _ := json.Marshal(claims)

	headerB64 := base64.RawURLEncoding.EncodeToString(headerJSON)
	claimsB64 := base64.RawURLEncoding.EncodeToString(claimsJSON)
	sigB64 := base64.RawURLEncoding.EncodeToString([]byte("fake-signature"))

	return headerB64 + "." + claimsB64 + "." + sigB64
}

func TestDecodeProvisioningToken(t *testing.T) {
	token := makeTestProvisioningToken(map[string]interface{}{
		"sub":       "myspace.backlog.jp",
		"relay_url": "https://relay.example.com",
		"space":     "myspace",
		"domain":    "backlog.jp",
		"purpose":   "provision",
		"iat":       1700000000,
		"nbf":       1700000000,
		"exp":       1700000900,
		"jti":       "test-jti",
	})

	claims, err := DecodeProvisioningToken(token)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if claims.Subject != "myspace.backlog.jp" {
		t.Errorf("subject: want myspace.backlog.jp, got %s", claims.Subject)
	}
	if claims.RelayURL != "https://relay.example.com" {
		t.Errorf("relay_url: want https://relay.example.com, got %s", claims.RelayURL)
	}
	if claims.Space != "myspace" {
		t.Errorf("space: want myspace, got %s", claims.Space)
	}
	if claims.Domain != "backlog.jp" {
		t.Errorf("domain: want backlog.jp, got %s", claims.Domain)
	}
	if claims.Purpose != "provision" {
		t.Errorf("purpose: want provision, got %s", claims.Purpose)
	}
	if claims.ExpiresAt != 1700000900 {
		t.Errorf("exp: want 1700000900, got %d", claims.ExpiresAt)
	}
}

func TestDecodeProvisioningToken_InvalidFormat(t *testing.T) {
	_, err := DecodeProvisioningToken("not-a-jwt")
	if err == nil {
		t.Fatal("expected error for invalid format")
	}
}

func TestDecodeProvisioningToken_WrongPurpose(t *testing.T) {
	token := makeTestProvisioningToken(map[string]interface{}{
		"sub":       "myspace.backlog.jp",
		"relay_url": "https://relay.example.com",
		"purpose":   "bundle",
		"iat":       1700000000,
	})

	_, err := DecodeProvisioningToken(token)
	if err == nil {
		t.Fatal("expected error for wrong purpose")
	}
}

func TestDecodeProvisioningToken_MissingRelayURL(t *testing.T) {
	token := makeTestProvisioningToken(map[string]interface{}{
		"sub":     "myspace.backlog.jp",
		"purpose": "provision",
		"iat":     1700000000,
	})

	_, err := DecodeProvisioningToken(token)
	if err == nil {
		t.Fatal("expected error for missing relay_url")
	}
}

func TestDecodeProvisioningToken_MissingSub(t *testing.T) {
	token := makeTestProvisioningToken(map[string]interface{}{
		"relay_url": "https://relay.example.com",
		"purpose":   "provision",
		"iat":       1700000000,
	})

	_, err := DecodeProvisioningToken(token)
	if err == nil {
		t.Fatal("expected error for missing sub")
	}
}
