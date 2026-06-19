package config

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/yacchi/backlog-cli/packages/backlog/internal/debug"
)

// RelayInfoPayload is the decoded payload from /v1/relay/tenants/{name}/info.
type RelayInfoPayload struct {
	Version  int    `json:"version"`
	Name     string `json:"name"`
	RelayURL string `json:"relay_url"`
	// Deprecated: v1 互換。name が無い場合のフォールバックとしてのみ読む。
	AllowedDomain string `json:"allowed_domain,omitempty"`
	IssuedAt      string `json:"issued_at"`
	ExpiresAt     string `json:"expires_at"`
	UpdateBefore  string `json:"update_before,omitempty"`
}

// PayloadName は info の識別子（name）を返す。v1 payload では allowed_domain を用いる。
func (p RelayInfoPayload) PayloadName() string {
	if p.Name != "" {
		return p.Name
	}
	return p.AllowedDomain
}

type relayInfoResponse struct {
	Payload    string               `json:"payload"`
	Signatures []relayBundleJWSSign `json:"signatures"`
}

// RelayInfoOptions configures info verification.
type RelayInfoOptions struct {
	HTTPClient    *http.Client
	CacheDir      string
	CertsCacheTTL int
	Now           time.Time
}

// VerifyRelayInfo fetches and verifies relay info using the trusted relay keys.
func VerifyRelayInfo(ctx context.Context, relayURL, name, bundleToken string, relayKeys []TrustedRelayKey, opts RelayInfoOptions) (*RelayInfoPayload, error) {
	if strings.TrimSpace(bundleToken) == "" {
		return nil, errors.New("bundle_token is required")
	}
	if strings.TrimSpace(relayURL) == "" {
		return nil, errors.New("relay_url is required")
	}
	if strings.TrimSpace(name) == "" {
		return nil, errors.New("name is required")
	}
	if len(relayKeys) == 0 {
		return nil, errors.New("relay_keys is required")
	}

	infoURL, err := BuildRelayInfoURL(relayURL, name)
	if err != nil {
		return nil, err
	}

	client := opts.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, infoURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create info request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+bundleToken)

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch info: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("info request failed: %s", resp.Status)
	}

	var info relayInfoResponse
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return nil, fmt.Errorf("failed to parse info: %w", err)
	}
	if info.Payload == "" || len(info.Signatures) == 0 {
		return nil, errors.New("info payload is empty")
	}

	payloadBytes, err := base64.RawURLEncoding.DecodeString(info.Payload)
	if err != nil {
		return nil, fmt.Errorf("invalid info payload encoding: %w", err)
	}
	var payload RelayInfoPayload
	if err := json.Unmarshal(payloadBytes, &payload); err != nil {
		return nil, fmt.Errorf("failed to parse info payload: %w", err)
	}

	if payload.PayloadName() != name {
		return nil, fmt.Errorf("info name mismatch: expected %s, got %s", name, payload.PayloadName())
	}
	if !relayURLMatches(relayURL, payload.RelayURL) {
		return nil, fmt.Errorf("info relay_url mismatch: expected %s, got %s", relayURL, payload.RelayURL)
	}

	now := opts.Now
	if now.IsZero() {
		now = time.Now().UTC()
	}
	if payload.ExpiresAt == "" {
		return nil, errors.New("info expires_at is required")
	}
	expiresAt, err := time.Parse(time.RFC3339, payload.ExpiresAt)
	if err != nil {
		return nil, fmt.Errorf("invalid info expires_at: %w", err)
	}
	if now.After(expiresAt) {
		return nil, fmt.Errorf("info expired at %s", payload.ExpiresAt)
	}

	cacheDir := opts.CacheDir
	if cacheDir == "" {
		cacheDir, err = defaultCacheDir()
		if err != nil {
			return nil, err
		}
	}
	cache := newCertsCache(cacheDir, opts.CertsCacheTTL)
	certsURL, err := buildRelayCertsURL(relayURL, name)
	if err != nil {
		return nil, err
	}
	jwks, err := fetchRelayJWKS(ctx, certsURL, client, cache)
	if err != nil {
		return nil, err
	}

	manifestKeys := make([]RelayBundleKey, 0, len(relayKeys))
	for _, key := range relayKeys {
		manifestKeys = append(manifestKeys, RelayBundleKey(key))
	}

	allowedKeys, err := buildAllowedKeys(jwks, manifestKeys)
	if err != nil {
		return nil, err
	}

	if err := verifyRelayInfoSignature(info.Payload, info.Signatures, allowedKeys); err != nil {
		return nil, err
	}

	return &payload, nil
}

// BundleFetchOptions configures bundle fetch/import.
type BundleFetchOptions struct {
	HTTPClient *http.Client
	CacheDir   string
	NoDefaults bool
}

// BundleImportResult は FetchAndImportRelayBundle の結果
type BundleImportResult struct {
	Bundle    *TrustedBundle
	Unchanged bool
}

// FetchAndImportRelayBundle fetches a bundle from relay and imports it.
// ダウンロードしたバンドルの SHA256 が既存バンドルと一致する場合はインポートをスキップし、
// Unchanged=true を返す。
func FetchAndImportRelayBundle(ctx context.Context, store *Store, relayURL, name, bundleToken string, opts BundleFetchOptions) (*BundleImportResult, error) {
	if store == nil {
		return nil, errors.New("config store is nil")
	}
	if strings.TrimSpace(bundleToken) == "" {
		return nil, errors.New("bundle_token is required")
	}

	bundleURL, err := BuildRelayBundleURL(relayURL, name)
	if err != nil {
		return nil, err
	}

	client := opts.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, bundleURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create bundle request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+bundleToken)

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch bundle: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("bundle request failed: %s", resp.Status)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read bundle: %w", err)
	}

	cacheDir := opts.CacheDir
	if cacheDir == "" {
		cacheDir, err = defaultCacheDir()
		if err != nil {
			return nil, err
		}
	}
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		return nil, fmt.Errorf("failed to create cache dir: %w", err)
	}

	filename := name + ".backlog-cli.zip"
	bundlePath := filepath.Join(cacheDir, filename)
	if err := os.WriteFile(bundlePath, body, 0o600); err != nil {
		return nil, fmt.Errorf("failed to write bundle: %w", err)
	}

	newSHA, err := sha256File(bundlePath)
	if err != nil {
		return nil, err
	}

	if existing := FindTrustedBundleByName(store, name); existing != nil && existing.Source.SHA256 == newSHA {
		debug.Log("bundle unchanged, skipping import", "name", name, "sha256", newSHA)
		return &BundleImportResult{Bundle: existing, Unchanged: true}, nil
	}

	imported, err := ImportRelayBundle(ctx, store, bundlePath, BundleImportOptions{
		NoDefaults: opts.NoDefaults,
		CacheDir:   cacheDir,
	})
	if err != nil {
		return nil, err
	}
	return &BundleImportResult{Bundle: imported, Unchanged: false}, nil
}

// BundleUpdateRequiredError indicates the bundle should be refreshed.
type BundleUpdateRequiredError struct {
	Name           string
	UpdateBefore   string
	BundleIssuedAt string
	Forced         bool
}

func (e *BundleUpdateRequiredError) Error() string {
	if e.Forced {
		return fmt.Sprintf("bundle update forced for %s", e.Name)
	}
	return fmt.Sprintf("bundle update required for %s (issued_at=%s update_before=%s)", e.Name, e.BundleIssuedAt, e.UpdateBefore)
}

// CheckBundleUpdate returns BundleUpdateRequiredError when update_before is reached.
func CheckBundleUpdate(info *RelayInfoPayload, bundle *TrustedBundle, now time.Time, force bool) error {
	if force {
		return &BundleUpdateRequiredError{
			Name:   bundle.ResolvedName(),
			Forced: true,
		}
	}
	if info == nil || bundle == nil {
		return nil
	}
	if strings.TrimSpace(info.UpdateBefore) == "" {
		return nil
	}

	updateBefore, err := time.Parse(time.RFC3339, info.UpdateBefore)
	if err != nil {
		return fmt.Errorf("invalid update_before: %w", err)
	}
	bundleIssuedAt, err := time.Parse(time.RFC3339, bundle.IssuedAt)
	if err != nil {
		return fmt.Errorf("invalid bundle issued_at: %w", err)
	}
	if bundleIssuedAt.Before(updateBefore) {
		return &BundleUpdateRequiredError{
			Name:           bundle.ResolvedName(),
			UpdateBefore:   info.UpdateBefore,
			BundleIssuedAt: bundle.IssuedAt,
		}
	}
	return nil
}

func BuildRelayInfoURL(relayURL, name string) (string, error) {
	if relayURL == "" {
		return "", errors.New("relay_url is empty")
	}
	if _, err := url.Parse(relayURL); err != nil {
		return "", fmt.Errorf("invalid relay_url: %w", err)
	}
	return url.JoinPath(relayURL, "/v1/relay/tenants/"+name+"/info")
}

func BuildRelayBundleURL(relayURL, name string) (string, error) {
	if relayURL == "" {
		return "", errors.New("relay_url is empty")
	}
	if _, err := url.Parse(relayURL); err != nil {
		return "", fmt.Errorf("invalid relay_url: %w", err)
	}
	return url.JoinPath(relayURL, "/v1/relay/tenants/"+name+"/bundle")
}

func verifyRelayInfoSignature(payload string, signatures []relayBundleJWSSign, allowedKeys map[string]allowedKey) error {
	signingInput := payload
	valid := false

	for _, sig := range signatures {
		protectedBytes, err := base64.RawURLEncoding.DecodeString(sig.Protected)
		if err != nil {
			continue
		}

		var header relayBundleJWSHeader
		if err := json.Unmarshal(protectedBytes, &header); err != nil {
			continue
		}
		if header.Alg != "EdDSA" || header.Kid == "" {
			continue
		}

		key, ok := allowedKeys[header.Kid]
		if !ok {
			continue
		}

		signatureBytes, err := base64.RawURLEncoding.DecodeString(sig.Signature)
		if err != nil {
			continue
		}

		input := []byte(sig.Protected + "." + signingInput)
		if ed25519.Verify(key.PublicKey, input, signatureBytes) {
			valid = true
			break
		}
	}

	if !valid {
		return errors.New("info signature verification failed")
	}
	return nil
}

func relayURLMatches(expected, actual string) bool {
	return strings.TrimRight(expected, "/") == strings.TrimRight(actual, "/")
}
