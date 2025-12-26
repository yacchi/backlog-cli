package config

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	jwkutil "github.com/yacchi/backlog-cli/internal/jwk"
	"gopkg.in/yaml.v3"
)

// BundleTokenClaims はbundle_tokenのJWTクレーム
type BundleTokenClaims struct {
	Subject   string `json:"sub"`           // allowed_domain
	IssuedAt  int64  `json:"iat"`           // 発行時刻
	NotBefore int64  `json:"nbf,omitempty"` // 有効開始時刻
	JTI       string `json:"jti"`           // 一意識別子
}

// GenerateBundleToken はbundle_token JWTを生成する
func GenerateBundleToken(allowedDomain string, jwkByKid map[string]relayBundleJWK, activeKeys []string, now time.Time) (string, error) {
	if len(activeKeys) == 0 {
		return "", errors.New("no active keys for signing")
	}

	// 最初のアクティブキーで署名
	kid := activeKeys[0]
	jwk, ok := jwkByKid[kid]
	if !ok {
		return "", fmt.Errorf("JWKS missing key for token signing: %s", kid)
	}

	privKey, err := jwkutil.Ed25519PrivateKeyFromJWK(jwk.Kty, jwk.Crv, jwk.Kid, jwk.D)
	if err != nil {
		return "", err
	}

	// JTI生成
	jtiBytes := make([]byte, 16)
	if _, err := rand.Read(jtiBytes); err != nil {
		return "", fmt.Errorf("failed to generate JTI: %w", err)
	}
	jti := base64.RawURLEncoding.EncodeToString(jtiBytes)

	// クレーム作成
	claims := BundleTokenClaims{
		Subject:   allowedDomain,
		IssuedAt:  now.Unix(),
		NotBefore: now.Unix(),
		JTI:       jti,
	}

	// ヘッダー
	header := map[string]string{
		"alg": "EdDSA",
		"typ": "JWT",
		"kid": kid,
	}

	headerJSON, err := json.Marshal(header)
	if err != nil {
		return "", fmt.Errorf("failed to marshal JWT header: %w", err)
	}

	claimsJSON, err := json.Marshal(claims)
	if err != nil {
		return "", fmt.Errorf("failed to marshal JWT claims: %w", err)
	}

	headerB64 := base64.RawURLEncoding.EncodeToString(headerJSON)
	claimsB64 := base64.RawURLEncoding.EncodeToString(claimsJSON)

	signingInput := headerB64 + "." + claimsB64
	signature := ed25519.Sign(privKey, []byte(signingInput))
	signatureB64 := base64.RawURLEncoding.EncodeToString(signature)

	return signingInput + "." + signatureB64, nil
}

// VerifyBundleTokenWithTenant はテナント設定を使用してbundle_token JWTを検証する
func VerifyBundleTokenWithTenant(token string, allowedDomain string, tenant *ResolvedTenant) error {
	if tenant == nil {
		return errors.New("tenant is nil")
	}
	if tenant.JWKS == "" {
		return errors.New("tenant JWKS is empty")
	}

	var jwks relayBundleJWKS
	if err := json.Unmarshal([]byte(tenant.JWKS), &jwks); err != nil {
		return fmt.Errorf("failed to parse tenant JWKS: %w", err)
	}
	if err := normalizeRelayJWKS(&jwks); err != nil {
		return err
	}

	return VerifyBundleToken(token, allowedDomain, &jwks)
}

// VerifyBundleToken はbundle_token JWTを検証する
func VerifyBundleToken(token string, allowedDomain string, jwks *relayBundleJWKS) error {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return errors.New("invalid JWT format")
	}

	headerB64, claimsB64, signatureB64 := parts[0], parts[1], parts[2]

	// ヘッダー解析
	headerJSON, err := base64.RawURLEncoding.DecodeString(headerB64)
	if err != nil {
		return fmt.Errorf("invalid JWT header encoding: %w", err)
	}

	var header struct {
		Alg string `json:"alg"`
		Kid string `json:"kid"`
	}
	if err := json.Unmarshal(headerJSON, &header); err != nil {
		return fmt.Errorf("invalid JWT header: %w", err)
	}

	if header.Alg != "EdDSA" {
		return fmt.Errorf("unsupported JWT algorithm: %s", header.Alg)
	}

	// 公開鍵を取得
	var pubKey ed25519.PublicKey
	for _, key := range jwks.Keys {
		if key.Kid == header.Kid {
			pubKey, err = jwkutil.Ed25519PublicKeyFromJWK(key.Kty, key.Crv, key.Kid, key.X)
			if err != nil {
				return err
			}
			break
		}
	}
	if pubKey == nil {
		return fmt.Errorf("unknown key ID: %s", header.Kid)
	}

	// 署名検証
	signature, err := base64.RawURLEncoding.DecodeString(signatureB64)
	if err != nil {
		return fmt.Errorf("invalid JWT signature encoding: %w", err)
	}

	signingInput := headerB64 + "." + claimsB64
	if !ed25519.Verify(pubKey, []byte(signingInput), signature) {
		return errors.New("JWT signature verification failed")
	}

	// クレーム検証
	claimsJSON, err := base64.RawURLEncoding.DecodeString(claimsB64)
	if err != nil {
		return fmt.Errorf("invalid JWT claims encoding: %w", err)
	}

	var claims BundleTokenClaims
	if err := json.Unmarshal(claimsJSON, &claims); err != nil {
		return fmt.Errorf("invalid JWT claims: %w", err)
	}

	if claims.Subject != allowedDomain {
		return fmt.Errorf("JWT subject mismatch: expected %s, got %s", allowedDomain, claims.Subject)
	}

	return nil
}

// BundleCreateOptions はバンドル作成のオプション
type BundleCreateOptions struct {
	ExpiresIn  time.Duration
	Files      []string
	OutputPath string
	IncludeAll bool
	Now        time.Time
}

// CreateRelayBundleFromConfig は設定と環境変数からバンドルを作成する
func CreateRelayBundleFromConfig(ctx context.Context, store *Store, opts BundleCreateOptions) (string, error) {
	_ = ctx
	if store == nil {
		return "", errors.New("config store is nil")
	}

	now := opts.Now
	if now.IsZero() {
		now = time.Now().UTC()
	}
	if opts.ExpiresIn <= 0 {
		opts.ExpiresIn = 30 * 24 * time.Hour
	}

	profile := store.CurrentProfile()
	if profile == nil {
		return "", errors.New("profile is not available")
	}

	if strings.TrimSpace(profile.RelayServer) == "" {
		return "", errors.New("profile.default.relay_server is required")
	}
	if strings.TrimSpace(profile.Space) == "" {
		return "", errors.New("profile.default.space is required")
	}
	if strings.TrimSpace(profile.Domain) == "" {
		return "", errors.New("profile.default.domain is required")
	}

	allowedDomain := profile.Space + "." + profile.Domain

	tenant, tenantKey, err := resolveTenantConfig(store, allowedDomain)
	if err != nil {
		return "", err
	}

	var jwks relayBundleJWKS
	if err := json.Unmarshal([]byte(tenant.JWKS), &jwks); err != nil {
		return "", fmt.Errorf("failed to parse tenant JWKS (%s): %w", tenantKey, err)
	}
	if err := normalizeRelayJWKS(&jwks); err != nil {
		return "", err
	}

	activeKeyIDs := splitCommaList(tenant.ActiveKeys)
	if len(activeKeyIDs) == 0 {
		return "", fmt.Errorf("no active keys for tenant %s", tenantKey)
	}

	jwkByKid := make(map[string]relayBundleJWK)
	for _, key := range jwks.Keys {
		jwkByKid[key.Kid] = key
	}

	manifestKeys, err := buildManifestRelayKeys(jwkByKid, activeKeyIDs, opts.IncludeAll)
	if err != nil {
		return "", err
	}

	fileRefs, fileData, err := buildBundleFiles(opts.Files)
	if err != nil {
		return "", err
	}

	// bundle_token生成
	bundleToken, err := GenerateBundleToken(allowedDomain, jwkByKid, activeKeyIDs, now)
	if err != nil {
		return "", fmt.Errorf("failed to generate bundle token: %w", err)
	}

	manifest := RelayBundleManifest{
		Version:       1,
		RelayURL:      profile.RelayServer,
		AllowedDomain: allowedDomain,
		IssuedAt:      now.Format(time.RFC3339),
		ExpiresAt:     now.Add(opts.ExpiresIn).Format(time.RFC3339),
		BundleToken:   bundleToken,
		RelayKeys:     manifestKeys,
		Files:         fileRefs,
	}

	manifestBytes, err := yaml.Marshal(&manifest)
	if err != nil {
		return "", fmt.Errorf("failed to serialize manifest: %w", err)
	}

	jwsBytes, err := signRelayBundleManifest(manifestBytes, jwkByKid, activeKeyIDs)
	if err != nil {
		return "", err
	}

	output := opts.OutputPath
	if strings.TrimSpace(output) == "" {
		output = allowedDomain + ".backlog-cli.zip"
	}

	if err := writeRelayBundleZip(output, manifestBytes, jwsBytes, fileData); err != nil {
		return "", err
	}

	return output, nil
}

func resolveTenantConfig(store *Store, allowedDomain string) (*ResolvedTenant, string, error) {
	if store == nil {
		return nil, "", errors.New("config store is nil")
	}
	tenants := store.Server().Tenants
	if len(tenants) == 0 {
		return nil, "", errors.New("server.tenants is empty")
	}

	for key, tenant := range tenants {
		if tenant.AllowedDomain != allowedDomain {
			continue
		}
		if strings.TrimSpace(tenant.JWKS) == "" {
			return nil, key, fmt.Errorf("tenant %s missing jwks", key)
		}
		if strings.TrimSpace(tenant.ActiveKeys) == "" {
			return nil, key, fmt.Errorf("tenant %s missing active_keys", key)
		}
		return &tenant, key, nil
	}

	return nil, "", fmt.Errorf("tenant config not found for allowed_domain %s", allowedDomain)
}

func splitCommaList(value string) []string {
	items := strings.Split(value, ",")
	result := make([]string, 0, len(items))
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		result = append(result, item)
	}
	return result
}

func buildManifestRelayKeys(jwkByKid map[string]relayBundleJWK, activeKeys []string, includeAll bool) ([]RelayBundleKey, error) {
	seen := make(map[string]struct{})
	keys := make([]string, 0, len(activeKeys))
	keys = append(keys, activeKeys...)

	if includeAll {
		for kid := range jwkByKid {
			if _, ok := seen[kid]; ok {
				continue
			}
			keys = append(keys, kid)
		}
	}

	manifestKeys := make([]RelayBundleKey, 0, len(keys))
	for _, kid := range keys {
		if _, ok := seen[kid]; ok {
			continue
		}
		seen[kid] = struct{}{}
		jwk, ok := jwkByKid[kid]
		if !ok {
			return nil, fmt.Errorf("JWKS missing key: %s", kid)
		}
		thumbprint, err := jwkThumbprint(jwk)
		if err != nil {
			return nil, err
		}
		manifestKeys = append(manifestKeys, RelayBundleKey{
			KeyID:      kid,
			Thumbprint: thumbprint,
		})
	}
	return manifestKeys, nil
}

func buildBundleFiles(paths []string) ([]RelayBundleFileRef, map[string][]byte, error) {
	refs := make([]RelayBundleFileRef, 0, len(paths))
	data := make(map[string][]byte)
	for _, path := range paths {
		name := filepath.Base(path)
		if name == "" || name == "." {
			return nil, nil, fmt.Errorf("invalid file path: %s", path)
		}
		if _, exists := data[name]; exists {
			return nil, nil, fmt.Errorf("duplicate file name in bundle: %s", name)
		}
		contents, err := os.ReadFile(path)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to read %s: %w", path, err)
		}
		sum := sha256.Sum256(contents)
		refs = append(refs, RelayBundleFileRef{
			Name:   name,
			SHA256: hex.EncodeToString(sum[:]),
		})
		data[name] = contents
	}
	return refs, data, nil
}

func signRelayBundleManifest(manifest []byte, jwkByKid map[string]relayBundleJWK, activeKeys []string) ([]byte, error) {
	payload := base64.RawURLEncoding.EncodeToString(manifest)
	signatures := make([]relayBundleJWSSign, 0, len(activeKeys))

	for _, kid := range activeKeys {
		jwk, ok := jwkByKid[kid]
		if !ok {
			return nil, fmt.Errorf("JWKS missing key for signing: %s", kid)
		}
		privKey, err := jwkutil.Ed25519PrivateKeyFromJWK(jwk.Kty, jwk.Crv, jwk.Kid, jwk.D)
		if err != nil {
			return nil, err
		}

		protectedJSON, _ := json.Marshal(map[string]string{
			"alg": "EdDSA",
			"kid": kid,
		})
		protected := base64.RawURLEncoding.EncodeToString(protectedJSON)
		signingInput := []byte(protected + "." + payload)
		signature := ed25519.Sign(privKey, signingInput)

		signatures = append(signatures, relayBundleJWSSign{
			Protected: protected,
			Signature: base64.RawURLEncoding.EncodeToString(signature),
		})
	}

	jws := relayBundleJWS{
		Payload:    payload,
		Signatures: signatures,
	}

	out, err := json.MarshalIndent(jws, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to serialize JWS: %w", err)
	}
	return out, nil
}

func normalizeRelayJWKS(jwks *relayBundleJWKS) error {
	if jwks == nil {
		return errors.New("jwks is nil")
	}

	for i := range jwks.Keys {
		key := &jwks.Keys[i]
		if key.D == "" {
			continue
		}
		privKey, err := jwkutil.Ed25519PrivateKeyFromJWK(key.Kty, key.Crv, key.Kid, key.D)
		if err != nil {
			return err
		}
		pubKey := privKey.Public().(ed25519.PublicKey)
		key.X = base64.RawURLEncoding.EncodeToString(pubKey)
	}

	return nil
}

func writeRelayBundleZip(path string, manifest, sig []byte, files map[string][]byte) error {
	out, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("failed to create bundle: %w", err)
	}
	defer func() { _ = out.Close() }()

	zipWriter := zip.NewWriter(out)
	defer func() { _ = zipWriter.Close() }()

	if err := writeZipFile(zipWriter, relayBundleManifestName, manifest); err != nil {
		return err
	}
	if err := writeZipFile(zipWriter, relayBundleManifestSigName, sig); err != nil {
		return err
	}
	for name, contents := range files {
		if err := writeZipFile(zipWriter, name, contents); err != nil {
			return err
		}
	}
	return nil
}

func writeZipFile(writer *zip.Writer, name string, contents []byte) error {
	header := &zip.FileHeader{
		Name:   name,
		Method: zip.Deflate,
	}
	header.Modified = time.Now().UTC()
	file, err := writer.CreateHeader(header)
	if err != nil {
		return fmt.Errorf("failed to create zip entry %s: %w", name, err)
	}
	_, err = io.Copy(file, bytes.NewReader(contents))
	if err != nil {
		return fmt.Errorf("failed to write zip entry %s: %w", name, err)
	}
	return nil
}

// CreatePortalBundle はテナント設定からバンドルをメモリ上に作成する（ポータル用）
func CreatePortalBundle(tenant *ResolvedTenant, allowedDomain, relayURL string) ([]byte, error) {
	if tenant == nil {
		return nil, errors.New("tenant is nil")
	}

	now := time.Now().UTC()
	expiresIn := 30 * 24 * time.Hour

	var jwks relayBundleJWKS
	if err := json.Unmarshal([]byte(tenant.JWKS), &jwks); err != nil {
		return nil, fmt.Errorf("failed to parse tenant JWKS: %w", err)
	}
	if err := normalizeRelayJWKS(&jwks); err != nil {
		return nil, err
	}

	activeKeyIDs := splitCommaList(tenant.ActiveKeys)
	if len(activeKeyIDs) == 0 {
		return nil, errors.New("no active keys for tenant")
	}

	jwkByKid := make(map[string]relayBundleJWK)
	for _, key := range jwks.Keys {
		jwkByKid[key.Kid] = key
	}

	manifestKeys, err := buildManifestRelayKeys(jwkByKid, activeKeyIDs, false)
	if err != nil {
		return nil, err
	}

	// bundle_token生成
	bundleToken, err := GenerateBundleToken(allowedDomain, jwkByKid, activeKeyIDs, now)
	if err != nil {
		return nil, fmt.Errorf("failed to generate bundle token: %w", err)
	}

	manifest := RelayBundleManifest{
		Version:       1,
		RelayURL:      relayURL,
		AllowedDomain: allowedDomain,
		IssuedAt:      now.Format(time.RFC3339),
		ExpiresAt:     now.Add(expiresIn).Format(time.RFC3339),
		BundleToken:   bundleToken,
		RelayKeys:     manifestKeys,
		Files:         []RelayBundleFileRef{},
	}

	manifestBytes, err := yaml.Marshal(&manifest)
	if err != nil {
		return nil, fmt.Errorf("failed to serialize manifest: %w", err)
	}

	jwsBytes, err := signRelayBundleManifest(manifestBytes, jwkByKid, activeKeyIDs)
	if err != nil {
		return nil, err
	}

	return createBundleZipBytes(manifestBytes, jwsBytes)
}

// createBundleZipBytes はバンドルZIPをメモリ上に作成する
func createBundleZipBytes(manifest, sig []byte) ([]byte, error) {
	var buf bytes.Buffer
	zipWriter := zip.NewWriter(&buf)

	if err := writeZipFile(zipWriter, relayBundleManifestName, manifest); err != nil {
		return nil, err
	}
	if err := writeZipFile(zipWriter, relayBundleManifestSigName, sig); err != nil {
		return nil, err
	}

	if err := zipWriter.Close(); err != nil {
		return nil, fmt.Errorf("failed to finalize zip: %w", err)
	}

	return buf.Bytes(), nil
}
