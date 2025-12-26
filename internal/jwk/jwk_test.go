package jwk

import (
	"bytes"
	"crypto/ed25519"
	"encoding/base64"
	"testing"
)

func TestEd25519PrivateKeyFromJWK(t *testing.T) {
	seed := bytes.Repeat([]byte{0x1}, ed25519.SeedSize)
	expectedPriv := ed25519.NewKeyFromSeed(seed)
	dValue := base64.RawURLEncoding.EncodeToString(seed)

	tests := []struct {
		name    string
		kty     string
		crv     string
		kid     string
		d       string
		wantErr bool
	}{
		{
			name:    "valid Ed25519",
			kty:     "OKP",
			crv:     "Ed25519",
			kid:     "k1",
			d:       dValue,
			wantErr: false,
		},
		{
			name:    "wrong kty",
			kty:     "RSA",
			crv:     "Ed25519",
			kid:     "k1",
			d:       dValue,
			wantErr: true,
		},
		{
			name:    "wrong crv",
			kty:     "OKP",
			crv:     "P-256",
			kid:     "k1",
			d:       dValue,
			wantErr: true,
		},
		{
			name:    "empty d",
			kty:     "OKP",
			crv:     "Ed25519",
			kid:     "k1",
			d:       "",
			wantErr: true,
		},
		{
			name:    "invalid base64",
			kty:     "OKP",
			crv:     "Ed25519",
			kid:     "k1",
			d:       "!!!invalid!!!",
			wantErr: true,
		},
		{
			name:    "wrong seed size",
			kty:     "OKP",
			crv:     "Ed25519",
			kid:     "k1",
			d:       base64.RawURLEncoding.EncodeToString([]byte("short")),
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Ed25519PrivateKeyFromJWK(tt.kty, tt.crv, tt.kid, tt.d)
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
			if !bytes.Equal(got, expectedPriv) {
				t.Errorf("private key mismatch")
			}
		})
	}
}

func TestEd25519PublicKeyFromJWK(t *testing.T) {
	seed := bytes.Repeat([]byte{0x1}, ed25519.SeedSize)
	priv := ed25519.NewKeyFromSeed(seed)
	pub := priv.Public().(ed25519.PublicKey)
	xValue := base64.RawURLEncoding.EncodeToString(pub)

	tests := []struct {
		name    string
		kty     string
		crv     string
		kid     string
		x       string
		wantErr bool
	}{
		{
			name:    "valid Ed25519",
			kty:     "OKP",
			crv:     "Ed25519",
			kid:     "k1",
			x:       xValue,
			wantErr: false,
		},
		{
			name:    "wrong kty",
			kty:     "RSA",
			crv:     "Ed25519",
			kid:     "k1",
			x:       xValue,
			wantErr: true,
		},
		{
			name:    "wrong crv",
			kty:     "OKP",
			crv:     "P-256",
			kid:     "k1",
			x:       xValue,
			wantErr: true,
		},
		{
			name:    "invalid base64",
			kty:     "OKP",
			crv:     "Ed25519",
			kid:     "k1",
			x:       "!!!invalid!!!",
			wantErr: true,
		},
		{
			name:    "wrong key size",
			kty:     "OKP",
			crv:     "Ed25519",
			kid:     "k1",
			x:       base64.RawURLEncoding.EncodeToString([]byte("short")),
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Ed25519PublicKeyFromJWK(tt.kty, tt.crv, tt.kid, tt.x)
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
			if !bytes.Equal(got, pub) {
				t.Errorf("public key mismatch")
			}
		})
	}
}
