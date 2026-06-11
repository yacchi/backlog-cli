package config

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/yacchi/backlog-cli/packages/backlog/internal/debug"
)

// ProvisioningTokenClaims はプロビジョニングトークンのJWTクレーム
type ProvisioningTokenClaims struct {
	Subject   string `json:"sub"`
	RelayURL  string `json:"relay_url"`
	Space     string `json:"space"`
	Domain    string `json:"domain"`
	Purpose   string `json:"purpose"`
	IssuedAt  int64  `json:"iat"`
	NotBefore int64  `json:"nbf"`
	ExpiresAt int64  `json:"exp"`
	JTI       string `json:"jti"`
}

// DecodeProvisioningToken はJWTを署名検証なしでデコードし、クレームを返す。
// CLIがrelay_urlとdomainを知るために使う。署名検証はバンドルインポート時に行われる。
func DecodeProvisioningToken(token string) (*ProvisioningTokenClaims, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return nil, errors.New("invalid provisioning token format")
	}

	claimsJSON, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, fmt.Errorf("failed to decode provisioning token claims: %w", err)
	}

	var claims ProvisioningTokenClaims
	if err := json.Unmarshal(claimsJSON, &claims); err != nil {
		return nil, fmt.Errorf("failed to parse provisioning token claims: %w", err)
	}

	if claims.Purpose != "provision" {
		return nil, fmt.Errorf("invalid token purpose: %s", claims.Purpose)
	}

	if strings.TrimSpace(claims.RelayURL) == "" {
		return nil, errors.New("provisioning token missing relay_url")
	}
	if parsed, err := url.Parse(claims.RelayURL); err != nil {
		return nil, fmt.Errorf("provisioning token has invalid relay_url: %w", err)
	} else {
		host := parsed.Hostname()
		if parsed.Scheme != "https" && host != "localhost" && host != "127.0.0.1" && host != "::1" {
			return nil, fmt.Errorf("provisioning token relay_url must use HTTPS (got %s)", parsed.Scheme)
		}
	}
	if strings.TrimSpace(claims.Subject) == "" {
		return nil, errors.New("provisioning token missing sub")
	}

	return &claims, nil
}

// ProvisionOptions はプロビジョニングのオプション
type ProvisionOptions struct {
	NoDefaults bool
	Now        time.Time
}

// ProvisionResponse は /api/v1/portal/{name}/provision のレスポンス
type ProvisionResponse struct {
	Success         bool   `json:"success"`
	ProvisioningKey string `json:"provisioning_key,omitempty"`
	DefaultSpace    string `json:"default_space,omitempty"`
	Error           string `json:"error,omitempty"`
}

// RequestProvisioningKey はリレーサーバーのポータル API にパスフレーズを送り、
// プロビジョニングキーとデフォルトスペースを取得する。
func RequestProvisioningKey(ctx context.Context, relayURL, name, passphrase string) (*ProvisionResponse, error) {
	if strings.TrimSpace(relayURL) == "" {
		return nil, errors.New("relay_url is required")
	}
	if strings.TrimSpace(name) == "" {
		return nil, errors.New("name is required")
	}
	if strings.TrimSpace(passphrase) == "" {
		return nil, errors.New("passphrase is required")
	}

	endpoint, err := url.JoinPath(relayURL, "/api/v1/portal/", name, "/provision")
	if err != nil {
		return nil, fmt.Errorf("failed to build provision URL: %w", err)
	}
	debug.Log("requesting provisioning key", "url", endpoint)

	body, err := json.Marshal(map[string]string{"passphrase": passphrase})
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	client := &http.Client{Timeout: 10 * time.Second}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("provision request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read provision response: %w", err)
	}

	var result ProvisionResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("failed to parse provision response: %w", err)
	}

	if !result.Success {
		errMsg := result.Error
		if errMsg == "" {
			errMsg = fmt.Sprintf("HTTP %d", resp.StatusCode)
		}
		return nil, fmt.Errorf("provision failed: %s", errMsg)
	}

	debug.Log("provisioning key obtained", "has_default_space", result.DefaultSpace != "")
	return &result, nil
}

// ProvisionFromToken はプロビジョニングトークンを使ってバンドルをダウンロード・インポートする
func ProvisionFromToken(ctx context.Context, store *Store, token string, opts ProvisionOptions) (*TrustedBundle, error) {
	if store == nil {
		return nil, errors.New("config store is nil")
	}

	debug.Log("decoding provisioning token")
	claims, err := DecodeProvisioningToken(token)
	if err != nil {
		return nil, fmt.Errorf("invalid provisioning token: %w", err)
	}
	debug.Log("provisioning token decoded",
		"sub", claims.Subject,
		"relay_url", claims.RelayURL,
		"space", claims.Space,
		"domain", claims.Domain,
	)

	now := opts.Now
	if now.IsZero() {
		now = time.Now().UTC()
	}
	if claims.ExpiresAt > 0 && now.Unix() > claims.ExpiresAt {
		return nil, fmt.Errorf("provisioning token expired at %s", time.Unix(claims.ExpiresAt, 0).Format(time.RFC3339))
	}

	debug.Log("fetching and importing bundle via provisioning token")
	return FetchAndImportRelayBundle(ctx, store, claims.RelayURL, claims.Subject, token, BundleFetchOptions{
		NoDefaults: opts.NoDefaults,
	})
}
