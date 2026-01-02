package relay

import (
	"bytes"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"testing"
)

func TestBuildRelayInfoSignatures(t *testing.T) {
	seed := bytes.Repeat([]byte{0x3}, ed25519.SeedSize)
	privKey := ed25519.NewKeyFromSeed(seed)
	pubKey := privKey.Public().(ed25519.PublicKey)

	jwk := relayJWK{
		Kty: "OKP",
		Crv: "Ed25519",
		Kid: "k1",
		X:   base64.RawURLEncoding.EncodeToString(pubKey),
		D:   base64.RawURLEncoding.EncodeToString(seed),
	}
	jwksJSON, err := json.Marshal(relayJWKS{Keys: []relayJWK{jwk}})
	if err != nil {
		t.Fatalf("marshal jwks failed: %v", err)
	}

	payloadJSON := []byte(`{"ok":true}`)
	payloadB64 := base64.RawURLEncoding.EncodeToString(payloadJSON)

	signatures, err := buildRelayInfoSignatures([]byte(payloadB64), string(jwksJSON), "k1")
	if err != nil {
		t.Fatalf("buildRelayInfoSignatures failed: %v", err)
	}
	if len(signatures) != 1 {
		t.Fatalf("expected 1 signature, got %d", len(signatures))
	}

	protected := signatures[0].Protected
	sigBytes, err := base64.RawURLEncoding.DecodeString(signatures[0].Signature)
	if err != nil {
		t.Fatalf("decode signature failed: %v", err)
	}

	signingInput := []byte(protected + "." + payloadB64)
	if !ed25519.Verify(pubKey, signingInput, sigBytes) {
		t.Fatalf("signature verification failed")
	}
}
