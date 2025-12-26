package config

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
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

	"github.com/yacchi/backlog-cli/internal/domain"
	jwkutil "github.com/yacchi/backlog-cli/internal/jwk"
	"gopkg.in/yaml.v3"
)

const (
	relayBundleManifestName    = "manifest.yaml"
	relayBundleManifestSigName = "manifest.yaml.sig"
)

// BundleImportOptions はバンドルインポートのオプション
type BundleImportOptions struct {
	AllowNameMismatch bool
	NoDefaults        bool
	HTTPClient        *http.Client
	Now               time.Time
	// キャッシュディレクトリ（空の場合はキャッシュ無効）
	CacheDir string
}

// RelayBundleManifest はmanifest.yamlの構造
type RelayBundleManifest struct {
	Version       int                  `yaml:"version"`
	RelayURL      string               `yaml:"relay_url"`
	AllowedDomain string               `yaml:"allowed_domain"`
	IssuedAt      string               `yaml:"issued_at"`
	ExpiresAt     string               `yaml:"expires_at"`
	CertsCacheTTL int                  `yaml:"certs_cache_ttl,omitempty"`
	BundleToken   string               `yaml:"bundle_token,omitempty"`
	RelayKeys     []RelayBundleKey     `yaml:"relay_keys"`
	Files         []RelayBundleFileRef `yaml:"files"`
}

// RelayBundleKey は信頼済み鍵の一覧
type RelayBundleKey struct {
	KeyID      string `json:"key_id" yaml:"key_id"`
	Thumbprint string `json:"thumbprint" yaml:"thumbprint"`
}

// RelayBundleFileRef は追加ファイルのハッシュ
type RelayBundleFileRef struct {
	Name   string `yaml:"name"`
	SHA256 string `yaml:"sha256"`
}

type relayBundleJWS struct {
	Payload    string               `json:"payload"`
	Signatures []relayBundleJWSSign `json:"signatures"`
}

type relayBundleJWSSign struct {
	Protected string `json:"protected"`
	Signature string `json:"signature"`
}

type relayBundleJWSHeader struct {
	Alg string `json:"alg"`
	Kid string `json:"kid"`
}

type relayBundleJWKS struct {
	Keys []relayBundleJWK `json:"keys"`
}

type relayBundleJWK struct {
	Kty string `json:"kty"`
	Crv string `json:"crv"`
	Kid string `json:"kid"`
	X   string `json:"x"`
	D   string `json:"d,omitempty"`
}

// ImportRelayBundle はバンドルを検証し、設定へ取り込む
func ImportRelayBundle(ctx context.Context, store *Store, bundlePath string, opts BundleImportOptions) (*TrustedBundle, error) {
	if store == nil {
		return nil, errors.New("config store is nil")
	}

	now := opts.Now
	if now.IsZero() {
		now = time.Now().UTC()
	}

	manifestBytes, sigBytes, files, err := readRelayBundle(bundlePath)
	if err != nil {
		return nil, err
	}

	manifest := RelayBundleManifest{}
	if err := yaml.Unmarshal(manifestBytes, &manifest); err != nil {
		return nil, fmt.Errorf("failed to parse manifest.yaml: %w", err)
	}

	if err := validateRelayBundleManifest(&manifest); err != nil {
		return nil, err
	}

	expectedName := manifest.AllowedDomain + ".backlog-cli.zip"
	actualName := filepath.Base(bundlePath)
	if !opts.AllowNameMismatch && actualName != expectedName {
		return nil, fmt.Errorf("bundle filename mismatch: expected %s, got %s", expectedName, actualName)
	}

	issuedAt, err := time.Parse(time.RFC3339, manifest.IssuedAt)
	if err != nil {
		return nil, fmt.Errorf("invalid issued_at: %w", err)
	}
	expiresAt, err := time.Parse(time.RFC3339, manifest.ExpiresAt)
	if err != nil {
		return nil, fmt.Errorf("invalid expires_at: %w", err)
	}
	if now.After(expiresAt) {
		return nil, fmt.Errorf("bundle expired at %s", manifest.ExpiresAt)
	}
	const maxIssuedAtSkew = 24 * time.Hour
	if issuedAt.After(now.Add(maxIssuedAtSkew)) {
		return nil, fmt.Errorf("issued_at is too far in the future: %s", manifest.IssuedAt)
	}

	certsURL, err := buildRelayCertsURL(manifest.RelayURL, manifest.AllowedDomain)
	if err != nil {
		return nil, err
	}

	// manifest.CertsCacheTTL が 0 または未指定の場合はキャッシュしない
	// CDNから配信する場合など、クライアント側キャッシュより
	// サーバー側キャッシュを優先したい場合に有用
	cache := newCertsCache(opts.CacheDir, manifest.CertsCacheTTL)
	jwks, err := fetchRelayJWKS(ctx, certsURL, opts.HTTPClient, cache)
	if err != nil {
		return nil, err
	}

	allowedKeys, err := buildAllowedKeys(jwks, manifest.RelayKeys)
	if err != nil {
		return nil, err
	}

	if err := verifyRelayBundleSignature(manifestBytes, sigBytes, allowedKeys); err != nil {
		return nil, err
	}

	if err := verifyRelayBundleFiles(files, manifest.Files); err != nil {
		return nil, err
	}

	bundleSHA, err := sha256File(bundlePath)
	if err != nil {
		return nil, err
	}

	trusted := TrustedBundle{
		ID:            manifest.AllowedDomain,
		RelayURL:      manifest.RelayURL,
		AllowedDomain: manifest.AllowedDomain,
		BundleToken:   manifest.BundleToken,
		RelayKeys:     toTrustedRelayKeys(manifest.RelayKeys),
		IssuedAt:      manifest.IssuedAt,
		ExpiresAt:     manifest.ExpiresAt,
		CertsCacheTTL: manifest.CertsCacheTTL,
		Source: BundleSource{
			FileName: actualName,
			SHA256:   bundleSHA,
		},
		ImportedAt: now.Format(time.RFC3339),
	}

	if err := upsertTrustedBundle(store, trusted); err != nil {
		return nil, err
	}

	if !opts.NoDefaults {
		if err := applyRelayBundleDefaults(store, manifest); err != nil {
			return nil, err
		}
	}

	return &trusted, nil
}

func validateRelayBundleManifest(manifest *RelayBundleManifest) error {
	if manifest.Version != 1 {
		return fmt.Errorf("unsupported manifest version: %d", manifest.Version)
	}
	if strings.TrimSpace(manifest.RelayURL) == "" {
		return errors.New("relay_url is required")
	}
	if strings.TrimSpace(manifest.AllowedDomain) == "" {
		return errors.New("allowed_domain is required")
	}
	if strings.TrimSpace(manifest.IssuedAt) == "" {
		return errors.New("issued_at is required")
	}
	if strings.TrimSpace(manifest.ExpiresAt) == "" {
		return errors.New("expires_at is required")
	}
	if len(manifest.RelayKeys) == 0 {
		return errors.New("relay_keys is required")
	}
	for _, key := range manifest.RelayKeys {
		if strings.TrimSpace(key.KeyID) == "" || strings.TrimSpace(key.Thumbprint) == "" {
			return errors.New("relay_keys entries must include key_id and thumbprint")
		}
	}
	return nil
}

func readRelayBundle(bundlePath string) ([]byte, []byte, map[string][]byte, error) {
	reader, err := zip.OpenReader(bundlePath)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to open bundle: %w", err)
	}
	defer func() { _ = reader.Close() }()

	files := make(map[string][]byte)
	for _, file := range reader.File {
		if file.FileInfo().IsDir() {
			continue
		}
		name := file.Name
		contents, err := readZipFile(file)
		if err != nil {
			return nil, nil, nil, err
		}
		files[name] = contents
	}

	manifestBytes, ok := files[relayBundleManifestName]
	if !ok {
		return nil, nil, nil, fmt.Errorf("missing %s", relayBundleManifestName)
	}
	sigBytes, ok := files[relayBundleManifestSigName]
	if !ok {
		return nil, nil, nil, fmt.Errorf("missing %s", relayBundleManifestSigName)
	}

	return manifestBytes, sigBytes, files, nil
}

func readZipFile(file *zip.File) ([]byte, error) {
	rc, err := file.Open()
	if err != nil {
		return nil, fmt.Errorf("failed to open %s: %w", file.Name, err)
	}
	defer func() { _ = rc.Close() }()
	return io.ReadAll(rc)
}

func buildRelayCertsURL(relayURL, allowedDomain string) (string, error) {
	if relayURL == "" {
		return "", errors.New("relay_url is empty")
	}
	if _, err := url.Parse(relayURL); err != nil {
		return "", fmt.Errorf("invalid relay_url: %w", err)
	}
	return url.JoinPath(relayURL, "/v1/relay/tenants/"+allowedDomain+"/certs")
}

type certsCache struct {
	dir string
	ttl time.Duration
}

func newCertsCache(dir string, ttlSeconds int) *certsCache {
	if dir == "" || ttlSeconds <= 0 {
		return nil
	}
	return &certsCache{
		dir: dir,
		ttl: time.Duration(ttlSeconds) * time.Second,
	}
}

func (c *certsCache) cacheKey(certsURL string) string {
	hash := sha256.Sum256([]byte(certsURL))
	return "certs_" + hex.EncodeToString(hash[:8])
}

func (c *certsCache) get(certsURL string) (*relayBundleJWKS, bool) {
	if c == nil {
		return nil, false
	}

	cacheFile := filepath.Join(c.dir, c.cacheKey(certsURL)+".json")
	data, err := os.ReadFile(cacheFile)
	if err != nil {
		return nil, false
	}

	var item struct {
		ExpiresAt time.Time        `json:"expires_at"`
		Data      *relayBundleJWKS `json:"data"`
	}
	if err := json.Unmarshal(data, &item); err != nil {
		return nil, false
	}

	if time.Now().After(item.ExpiresAt) {
		_ = os.Remove(cacheFile)
		return nil, false
	}

	return item.Data, true
}

func (c *certsCache) set(certsURL string, jwks *relayBundleJWKS) {
	if c == nil {
		return
	}

	_ = os.MkdirAll(c.dir, 0700)

	item := struct {
		ExpiresAt time.Time        `json:"expires_at"`
		Data      *relayBundleJWKS `json:"data"`
	}{
		ExpiresAt: time.Now().Add(c.ttl),
		Data:      jwks,
	}

	data, err := json.Marshal(item)
	if err != nil {
		return
	}

	cacheFile := filepath.Join(c.dir, c.cacheKey(certsURL)+".json")
	_ = os.WriteFile(cacheFile, data, 0600)
}

func fetchRelayJWKS(ctx context.Context, certsURL string, client *http.Client, cache *certsCache) (*relayBundleJWKS, error) {
	// キャッシュからの取得を試みる
	if jwks, ok := cache.get(certsURL); ok {
		return jwks, nil
	}

	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, certsURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create JWKS request: %w", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch JWKS: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("JWKS request failed: %s (%s)", resp.Status, strings.TrimSpace(string(body)))
	}

	var jwks relayBundleJWKS
	if err := json.NewDecoder(resp.Body).Decode(&jwks); err != nil {
		return nil, fmt.Errorf("failed to parse JWKS: %w", err)
	}

	// キャッシュに保存
	cache.set(certsURL, &jwks)

	return &jwks, nil
}

type allowedKey struct {
	PublicKey  ed25519.PublicKey
	Thumbprint string
}

func buildAllowedKeys(jwks *relayBundleJWKS, keys []RelayBundleKey) (map[string]allowedKey, error) {
	jwkByKid := make(map[string]relayBundleJWK)
	for _, key := range jwks.Keys {
		jwkByKid[key.Kid] = key
	}

	allowed := make(map[string]allowedKey)
	for _, key := range keys {
		jwk, ok := jwkByKid[key.KeyID]
		if !ok {
			return nil, fmt.Errorf("relay key %s not found in JWKS", key.KeyID)
		}
		thumbprint, err := jwkThumbprint(jwk)
		if err != nil {
			return nil, err
		}
		if thumbprint != key.Thumbprint {
			return nil, fmt.Errorf("thumbprint mismatch for key %s", key.KeyID)
		}
		publicKey, err := jwkutil.Ed25519PublicKeyFromJWK(jwk.Kty, jwk.Crv, jwk.Kid, jwk.X)
		if err != nil {
			return nil, err
		}
		allowed[key.KeyID] = allowedKey{
			PublicKey:  publicKey,
			Thumbprint: thumbprint,
		}
	}

	return allowed, nil
}

func jwkThumbprint(jwk relayBundleJWK) (string, error) {
	if jwk.Kty != "OKP" || jwk.Crv != "Ed25519" {
		return "", fmt.Errorf("unsupported JWK: kty=%s crv=%s", jwk.Kty, jwk.Crv)
	}
	if jwk.X == "" {
		return "", errors.New("JWK missing x parameter")
	}

	canonical := fmt.Sprintf(`{"crv":"%s","kty":"%s","x":"%s"}`, jwk.Crv, jwk.Kty, jwk.X)
	sum := sha256.Sum256([]byte(canonical))
	return base64.RawURLEncoding.EncodeToString(sum[:]), nil
}

func verifyRelayBundleSignature(manifestBytes, sigBytes []byte, allowedKeys map[string]allowedKey) error {
	var jws relayBundleJWS
	if err := json.Unmarshal(sigBytes, &jws); err != nil {
		return fmt.Errorf("failed to parse manifest.yaml.sig: %w", err)
	}
	if jws.Payload == "" || len(jws.Signatures) == 0 {
		return errors.New("manifest signature payload is empty")
	}

	payloadBytes, err := base64.RawURLEncoding.DecodeString(jws.Payload)
	if err != nil {
		return fmt.Errorf("invalid JWS payload encoding: %w", err)
	}
	if !bytes.Equal(payloadBytes, manifestBytes) {
		return errors.New("manifest payload does not match manifest.yaml")
	}

	signingInput := jws.Payload
	valid := false

	for _, sig := range jws.Signatures {
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
		return errors.New("manifest signature verification failed")
	}
	return nil
}

func verifyRelayBundleFiles(files map[string][]byte, refs []RelayBundleFileRef) error {
	for _, ref := range refs {
		contents, ok := files[ref.Name]
		if !ok {
			return fmt.Errorf("missing file in bundle: %s", ref.Name)
		}

		sum := sha256.Sum256(contents)
		expected := strings.TrimSpace(ref.SHA256)
		if expected == "" {
			return fmt.Errorf("missing sha256 for file %s", ref.Name)
		}

		actual := hex.EncodeToString(sum[:])
		if !strings.EqualFold(actual, expected) {
			return fmt.Errorf("sha256 mismatch for %s", ref.Name)
		}
	}
	return nil
}

func sha256File(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("failed to open bundle for hashing: %w", err)
	}
	defer func() { _ = f.Close() }()

	hash := sha256.New()
	if _, err := io.Copy(hash, f); err != nil {
		return "", fmt.Errorf("failed to hash bundle: %w", err)
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func toTrustedRelayKeys(keys []RelayBundleKey) []TrustedRelayKey {
	converted := make([]TrustedRelayKey, 0, len(keys))
	for _, key := range keys {
		converted = append(converted, TrustedRelayKey(key))
	}
	return converted
}

func upsertTrustedBundle(store *Store, bundle TrustedBundle) error {
	resolved := store.Resolved()
	existing := resolved.Client.Trust.Bundles
	updated := make([]TrustedBundle, 0, len(existing)+1)
	replaced := false

	for _, current := range existing {
		if current.ID == bundle.ID {
			updated = append(updated, bundle)
			replaced = true
			continue
		}
		updated = append(updated, current)
	}
	if !replaced {
		updated = append(updated, bundle)
	}

	return store.Set("client.trust.bundles", updated)
}

func applyRelayBundleDefaults(store *Store, manifest RelayBundleManifest) error {
	space, backlogDomain, err := domain.SplitAllowedDomain(manifest.AllowedDomain)
	if err != nil {
		return err
	}
	if err := store.Set("profile.default.relay_server", manifest.RelayURL); err != nil {
		return err
	}
	if err := store.Set("profile.default.space", space); err != nil {
		return err
	}
	if err := store.Set("profile.default.domain", backlogDomain); err != nil {
		return err
	}
	return nil
}
