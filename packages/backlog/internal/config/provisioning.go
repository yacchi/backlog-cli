package config

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
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
