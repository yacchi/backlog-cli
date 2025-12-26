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
