package config

import (
	"testing"
)

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
