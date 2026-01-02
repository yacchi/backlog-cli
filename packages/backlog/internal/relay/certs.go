package relay

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/yacchi/backlog-cli/packages/backlog/internal/config"
)

func (s *Server) handleRelayCerts(w http.ResponseWriter, r *http.Request) {
	domain := strings.TrimSpace(r.PathValue("domain"))
	if domain == "" {
		http.Error(w, "domain is required", http.StatusBadRequest)
		return
	}

	tenant, ok := findTenantByAllowedDomain(s.cfg.Server().Tenants, domain)
	if !ok {
		http.Error(w, "tenant not found", http.StatusNotFound)
		return
	}

	jwksJSON := strings.TrimSpace(tenant.JWKS)
	if jwksJSON == "" {
		http.Error(w, "tenant jwks is empty", http.StatusInternalServerError)
		return
	}

	redacted, err := redactJWKSPrivateKeys([]byte(jwksJSON))
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to parse jwks: %v", err), http.StatusInternalServerError)
		return
	}

	// ETag生成（コンテンツのSHA256ハッシュ）
	hash := sha256.Sum256(redacted)
	etag := `"` + hex.EncodeToString(hash[:16]) + `"`

	// If-None-Matchヘッダーでキャッシュ検証
	if match := r.Header.Get("If-None-Match"); match == etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}

	// キャッシュヘッダー設定
	w.Header().Set("Content-Type", "application/json")
	SetCacheHeaders(w, CacheTypeLong, s.cfg)
	w.Header().Set("ETag", etag)
	_, _ = w.Write(redacted)
}

func redactJWKSPrivateKeys(input []byte) ([]byte, error) {
	var jwks relayJWKS
	if err := json.Unmarshal(input, &jwks); err != nil {
		return nil, err
	}
	if len(jwks.Keys) == 0 {
		return nil, fmt.Errorf("invalid jwks: keys is missing or empty")
	}

	for i := range jwks.Keys {
		key := &jwks.Keys[i]
		if key.D != "" {
			derivedX, err := derivePublicKeyFromSeed(*key)
			if err != nil {
				return nil, err
			}
			key.X = derivedX
		}
		key.D = ""
	}

	return json.Marshal(jwks)
}

func derivePublicKeyFromSeed(jwk relayJWK) (string, error) {
	if jwk.Kty != "" && jwk.Kty != "OKP" {
		return "", fmt.Errorf("unsupported JWK: kty=%s", jwk.Kty)
	}
	if jwk.Crv != "" && jwk.Crv != "Ed25519" {
		return "", fmt.Errorf("unsupported JWK: crv=%s", jwk.Crv)
	}
	if jwk.D == "" {
		return "", fmt.Errorf("missing private key material for %s", jwk.Kid)
	}
	seed, err := base64.RawURLEncoding.DecodeString(jwk.D)
	if err != nil {
		return "", fmt.Errorf("invalid JWK d for %s: %w", jwk.Kid, err)
	}
	if len(seed) != ed25519.SeedSize {
		return "", fmt.Errorf("invalid JWK seed size for %s: %d", jwk.Kid, len(seed))
	}
	privKey := ed25519.NewKeyFromSeed(seed)
	pubKey := privKey.Public().(ed25519.PublicKey)
	return base64.RawURLEncoding.EncodeToString(pubKey), nil
}

func findTenantByAllowedDomain(tenants map[string]config.ResolvedTenant, domain string) (*config.ResolvedTenant, bool) {
	for _, tenant := range tenants {
		if strings.EqualFold(tenant.AllowedDomain, domain) {
			tenantCopy := tenant
			return &tenantCopy, true
		}
	}
	return nil, false
}
