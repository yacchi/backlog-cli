package config

import (
	"bytes"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"testing"
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
