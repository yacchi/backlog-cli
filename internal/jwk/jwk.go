package jwk

import (
	"crypto/ed25519"
	"encoding/base64"
	"fmt"
)

// Ed25519PrivateKeyFromJWK converts Ed25519 JWK parameters into a private key.
func Ed25519PrivateKeyFromJWK(kty, crv, kid, d string) (ed25519.PrivateKey, error) {
	if kty != "OKP" || crv != "Ed25519" {
		return nil, fmt.Errorf("unsupported JWK: kty=%s crv=%s", kty, crv)
	}
	if d == "" {
		return nil, fmt.Errorf("missing private key material for %s", kid)
	}
	seed, err := base64.RawURLEncoding.DecodeString(d)
	if err != nil {
		return nil, fmt.Errorf("invalid JWK d for %s: %w", kid, err)
	}
	if len(seed) != ed25519.SeedSize {
		return nil, fmt.Errorf("invalid JWK seed size for %s: %d", kid, len(seed))
	}
	return ed25519.NewKeyFromSeed(seed), nil
}

// Ed25519PublicKeyFromJWK converts Ed25519 JWK parameters into a public key.
func Ed25519PublicKeyFromJWK(kty, crv, kid, x string) (ed25519.PublicKey, error) {
	if kty != "OKP" || crv != "Ed25519" {
		return nil, fmt.Errorf("unsupported JWK: kty=%s crv=%s", kty, crv)
	}
	keyBytes, err := base64.RawURLEncoding.DecodeString(x)
	if err != nil {
		return nil, fmt.Errorf("invalid JWK x for %s: %w", kid, err)
	}
	if len(keyBytes) != ed25519.PublicKeySize {
		return nil, fmt.Errorf("invalid Ed25519 public key size for %s: %d", kid, len(keyBytes))
	}
	return ed25519.PublicKey(keyBytes), nil
}
