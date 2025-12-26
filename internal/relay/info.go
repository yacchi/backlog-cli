package relay

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/yacchi/backlog-cli/internal/domain"
	jwkutil "github.com/yacchi/backlog-cli/internal/jwk"
)

const relayInfoVersion = 1
const relayInfoDefaultTTL = 600

type relayInfoPayload struct {
	Version       int    `json:"version"`
	RelayURL      string `json:"relay_url"`
	AllowedDomain string `json:"allowed_domain"`
	Space         string `json:"space"`
	Domain        string `json:"domain"`
	IssuedAt      string `json:"issued_at"`
	ExpiresAt     string `json:"expires_at"`
	UpdateBefore  string `json:"update_before,omitempty"`
}

type relayInfoSignature struct {
	Protected string `json:"protected"`
	Signature string `json:"signature"`
}

type relayInfoResponse struct {
	Payload        string               `json:"payload"`
	Signatures     []relayInfoSignature `json:"signatures"`
	PayloadDecoded relayInfoPayload     `json:"payload_decoded"`
}

type relayJWKS struct {
	Keys []relayJWK `json:"keys"`
}

type relayJWK struct {
	Kty string `json:"kty"`
	Crv string `json:"crv"`
	Kid string `json:"kid"`
	X   string `json:"x"`
	D   string `json:"d"`
}

type relayInfoProtected struct {
	Alg string `json:"alg"`
	Kid string `json:"kid"`
}

func (s *Server) handleRelayInfo(w http.ResponseWriter, r *http.Request) {
	allowedDomain := strings.TrimSpace(r.PathValue("domain"))
	if allowedDomain == "" {
		http.Error(w, "domain is required", http.StatusBadRequest)
		return
	}

	tenant, ok := findTenantByAllowedDomain(s.cfg.Server().Tenants, allowedDomain)
	if !ok {
		http.Error(w, "tenant not found", http.StatusNotFound)
		return
	}

	issuedAt := time.Now().UTC()
	ttl := tenant.InfoTTL
	if ttl <= 0 {
		ttl = relayInfoDefaultTTL
	}
	expiresAt := issuedAt.Add(time.Duration(ttl) * time.Second)

	space, backlogDomain := domain.SplitDomain(tenant.AllowedDomain)
	payload := relayInfoPayload{
		Version:       relayInfoVersion,
		RelayURL:      s.buildRelayURL(r),
		AllowedDomain: tenant.AllowedDomain,
		Space:         space,
		Domain:        backlogDomain,
		IssuedAt:      issuedAt.Format(time.RFC3339),
		ExpiresAt:     expiresAt.Format(time.RFC3339),
	}

	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to marshal payload: %v", err), http.StatusInternalServerError)
		return
	}
	payloadB64 := base64.RawURLEncoding.EncodeToString(payloadJSON)

	signatures, err := buildRelayInfoSignatures([]byte(payloadB64), tenant.JWKS, tenant.ActiveKeys)
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to sign payload: %v", err), http.StatusInternalServerError)
		return
	}

	resp := relayInfoResponse{
		Payload:        payloadB64,
		Signatures:     signatures,
		PayloadDecoded: payload,
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

func buildRelayInfoSignatures(payloadB64 []byte, jwksJSON string, activeKeys string) ([]relayInfoSignature, error) {
	activeKeyIDs := splitKeyList(activeKeys)
	if len(activeKeyIDs) == 0 {
		return nil, fmt.Errorf("active_keys is empty")
	}

	var jwks relayJWKS
	if err := json.Unmarshal([]byte(jwksJSON), &jwks); err != nil {
		return nil, fmt.Errorf("invalid jwks: %w", err)
	}

	jwkByKid := make(map[string]relayJWK)
	for _, key := range jwks.Keys {
		if key.Kid != "" {
			jwkByKid[key.Kid] = key
		}
	}

	signatures := make([]relayInfoSignature, 0, len(activeKeyIDs))
	for _, kid := range activeKeyIDs {
		jwk, ok := jwkByKid[kid]
		if !ok {
			return nil, fmt.Errorf("jwks missing key: %s", kid)
		}
		privKey, err := jwkutil.Ed25519PrivateKeyFromJWK(jwk.Kty, jwk.Crv, jwk.Kid, jwk.D)
		if err != nil {
			return nil, err
		}

		protected := relayInfoProtected{
			Alg: "EdDSA",
			Kid: kid,
		}
		protectedJSON, err := json.Marshal(protected)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal protected header: %w", err)
		}

		protectedB64 := base64.RawURLEncoding.EncodeToString(protectedJSON)
		signingInput := protectedB64 + "." + string(payloadB64)
		sig := ed25519.Sign(privKey, []byte(signingInput))

		signatures = append(signatures, relayInfoSignature{
			Protected: protectedB64,
			Signature: base64.RawURLEncoding.EncodeToString(sig),
		})
	}

	return signatures, nil
}

func splitKeyList(value string) []string {
	items := strings.Split(value, ",")
	out := make([]string, 0, len(items))
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item != "" {
			out = append(out, item)
		}
	}
	return out
}
