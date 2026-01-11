package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/ogen-go/ogen/ogenerrors"
	"github.com/yacchi/backlog-cli/packages/backlog/internal/cache"
	"github.com/yacchi/backlog-cli/packages/backlog/internal/config"
	"github.com/yacchi/backlog-cli/packages/backlog/internal/debug"
	"github.com/yacchi/backlog-cli/packages/backlog/internal/gen/backlog"
)

// Client は Backlog API クライアント
type Client struct {
	space       string
	domain      string
	accessToken string
	httpClient  *http.Client

	backlogClient *backlog.Client

	// API Key認証用
	apiKey string

	// トークン更新用（OAuth）
	refreshToken  string
	expiresAt     time.Time
	relayServer   string
	onTokenUpdate func(ctx context.Context, accessToken, refreshToken string, expiresAt time.Time)

	// キャッシュ
	cache    cache.Cache
	cacheTTL time.Duration

	// HTTP/認証設定
	tokenRefreshMargin time.Duration
}

// ClientOption はクライアントオプション
type ClientOption func(*Client)

// WithCache はキャッシュを設定する
func WithCache(c cache.Cache, ttl time.Duration) ClientOption {
	return func(client *Client) {
		client.cache = c
		client.cacheTTL = ttl
	}
}

// WithTokenRefresh はトークン自動更新を有効にする（OAuth用）
func WithTokenRefresh(refreshToken, relayServer string, expiresAt time.Time, callback func(ctx context.Context, accessToken, refreshToken string, expiresAt time.Time)) ClientOption {
	return func(c *Client) {
		c.refreshToken = refreshToken
		// 末尾スラッシュを除去してパス連結時のダブルスラッシュを防止
		c.relayServer = strings.TrimRight(relayServer, "/")
		c.expiresAt = expiresAt
		c.onTokenUpdate = callback
	}
}

// WithHTTPTimeout はHTTPタイムアウトを設定する
func WithHTTPTimeout(timeout time.Duration) ClientOption {
	return func(c *Client) {
		if timeout > 0 {
			c.httpClient.Timeout = timeout
		}
	}
}

// WithTokenRefreshMargin はトークン更新のマージンを設定する
func WithTokenRefreshMargin(margin time.Duration) ClientOption {
	return func(c *Client) {
		if margin >= 0 {
			c.tokenRefreshMargin = margin
		}
	}
}

// WithAPIKey はAPI Key認証を設定する
func WithAPIKey(apiKey string) ClientOption {
	return func(c *Client) {
		c.apiKey = apiKey
		c.accessToken = "" // API Key使用時はAccessTokenは不要
	}
}

// NewClient は新しいクライアントを作成する
func NewClient(space, domain, accessToken string, opts ...ClientOption) *Client {
	c := &Client{
		space:              space,
		domain:             domain,
		accessToken:        accessToken,
		tokenRefreshMargin: 5 * time.Minute,
	}

	// RetryTransportを設定したHTTPクライアントを作成
	c.httpClient = &http.Client{
		Timeout: 30 * time.Second,
		Transport: &RetryTransport{
			Base:       http.DefaultTransport,
			MaxRetries: 5, // リトライ回数（必要に応じて設定可能にしてもよい）
		},
	}

	for _, opt := range opts {
		opt(c)
	}

	// ogen クライアントの初期化
	// カスタムHTTPクライアントを使用するように設定
	bc, err := backlog.NewClient(c.baseURL(), c, backlog.WithClient(c.httpClient))
	if err != nil {
		panic(fmt.Sprintf("failed to create backlog client: %v", err))
	}
	c.backlogClient = bc

	return c
}

// ApiKey は API Key 認証情報を提供する
func (c *Client) ApiKey(ctx context.Context, operationName backlog.OperationName) (backlog.ApiKey, error) {
	if c.apiKey != "" {
		return backlog.ApiKey{APIKey: c.apiKey}, nil
	}
	return backlog.ApiKey{}, ogenerrors.ErrSkipClientSecurity
}

// OAuth2 は OAuth2 認証情報を提供する
func (c *Client) OAuth2(ctx context.Context, operationName backlog.OperationName) (backlog.OAuth2, error) {
	// API Key が設定されている場合は OAuth2 をスキップ
	if c.apiKey != "" {
		return backlog.OAuth2{}, ogenerrors.ErrSkipClientSecurity
	}

	if c.accessToken != "" {
		// トークンリフレッシュの確認
		if err := c.ensureValidToken(ctx); err != nil {
			return backlog.OAuth2{}, fmt.Errorf("token refresh failed: %w", err)
		}
		return backlog.OAuth2{Token: c.accessToken}, nil
	}
	return backlog.OAuth2{}, ogenerrors.ErrSkipClientSecurity
}

// NewClientFromConfig は設定からクライアントを作成する
func NewClientFromConfig(cfg *config.Store) (*Client, error) {
	resolved := cfg.Resolved()
	profile := resolved.GetActiveProfile()
	project := cfg.Project()
	cred := resolved.GetActiveCredential()

	if cred == nil {
		return nil, fmt.Errorf("not authenticated")
	}

	// space/domainはプロジェクト設定を優先
	space := profile.Space
	domain := profile.Domain
	if project != nil {
		if project.Space != "" {
			space = project.Space
		}
		if project.Domain != "" {
			domain = project.Domain
		}
	}

	// キャッシュ設定
	var c cache.Cache
	ttl := time.Duration(resolved.Cache.TTL) * time.Second
	if resolved.Cache.Enabled {
		cacheDir, err := resolved.Cache.GetCacheDir()
		if err == nil {
			c, err = cache.NewFileCache(cacheDir)
			if err != nil {
				// キャッシュ初期化失敗はログに出さず無視（キャッシュなしで動作）
			} else {
				// バックグラウンドで期限切れキャッシュの削除を実行
				// プロセス終了とともに終了するため、context.Background()を使用
				go func() {
					_ = c.Cleanup(context.Background(), ttl)
				}()
			}
		}
	}

	// 認証タイプに応じてクライアントを作成
	switch cred.GetAuthType() {
	case config.AuthTypeAPIKey:
		// API Key認証
		httpTimeout := time.Duration(profile.HTTPTimeout) * time.Second
		client := NewClient(
			space,
			domain,
			"", // accessToken不要
			WithAPIKey(cred.APIKey),
			WithHTTPTimeout(httpTimeout),
			WithTokenRefreshMargin(time.Duration(profile.HTTPTokenRefreshMargin)*time.Second),
			WithCache(c, ttl),
		)
		return client, nil

	default:
		// OAuth認証（デフォルト）
		profileName := resolved.ActiveProfile
		httpTimeout := time.Duration(profile.HTTPTimeout) * time.Second
		client := NewClient(
			space,
			domain,
			cred.AccessToken,
			WithTokenRefresh(
				cred.RefreshToken,
				profile.RelayServer,
				cred.ExpiresAt,
				func(ctx context.Context, accessToken, refreshToken string, expiresAt time.Time) {
					// 設定ファイルを更新（プロファイルに紐づける）
					if err := cfg.SetCredential(profileName, &config.Credential{
						AuthType:     config.AuthTypeOAuth,
						AccessToken:  accessToken,
						RefreshToken: refreshToken,
						ExpiresAt:    expiresAt,
						UserID:       cred.UserID,
						UserName:     cred.UserName,
					}); err != nil {
						debug.Log("failed to set credential after token refresh", "error", err)
					}
					if err := cfg.Save(ctx); err != nil {
						debug.Log("failed to save config after token refresh", "error", err)
					}
				},
			),
			WithHTTPTimeout(httpTimeout),
			WithTokenRefreshMargin(time.Duration(profile.HTTPTokenRefreshMargin)*time.Second),
			WithCache(c, ttl),
		)
		return client, nil
	}
}

func (c *Client) baseURL() string {
	return fmt.Sprintf("https://%s.%s/api/v2", c.space, c.domain)
}

// RawBaseURL returns the base URL without the /api/v2 suffix
func (c *Client) RawBaseURL() string {
	return fmt.Sprintf("https://%s.%s", c.space, c.domain)
}

// ensureValidToken はトークンが有効か確認し、必要なら更新する
func (c *Client) ensureValidToken(ctx context.Context) error {
	if c.refreshToken == "" || c.relayServer == "" {
		return nil // 自動更新なし
	}

	// 有効期限の5分前に更新
	if time.Now().Add(c.tokenRefreshMargin).Before(c.expiresAt) {
		return nil // まだ有効
	}

	// トークン更新
	return c.doRefreshToken(ctx)
}

func (c *Client) doRefreshToken(ctx context.Context) error {
	reqBody := map[string]string{
		"grant_type":    "refresh_token",
		"refresh_token": c.refreshToken,
		"domain":        c.domain,
		"space":         c.space,
	}

	body, _ := json.Marshal(reqBody)

	req, err := http.NewRequestWithContext(ctx, "POST", c.relayServer+"/auth/token", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to create token refresh request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("token refresh request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("token refresh failed with status %d", resp.StatusCode)
	}

	var tokenResp struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int    `json:"expires_in"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return fmt.Errorf("failed to parse token response: %w", err)
	}

	// 更新
	c.accessToken = tokenResp.AccessToken
	c.refreshToken = tokenResp.RefreshToken
	c.expiresAt = time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second)

	// コールバック（キャンセルを切って値のみ伝播）
	if c.onTokenUpdate != nil {
		c.onTokenUpdate(context.WithoutCancel(ctx), c.accessToken, c.refreshToken, c.expiresAt)
	}

	return nil
}

// Request はAPIリクエストを実行する
func (c *Client) Request(ctx context.Context, method, path string, query url.Values, body interface{}) (*http.Response, error) {
	// OAuth認証の場合のみトークン更新チェック
	if c.apiKey == "" {
		if err := c.ensureValidToken(ctx); err != nil {
			return nil, fmt.Errorf("token refresh failed: %w", err)
		}
	}

	u := c.baseURL() + path

	// クエリパラメータの構築
	if query == nil {
		query = url.Values{}
	}
	// API Key認証の場合はクエリパラメータに追加
	if c.apiKey != "" {
		query.Set("apiKey", c.apiKey)
	}
	if len(query) > 0 {
		u += "?" + query.Encode()
	}

	var reqBody io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		reqBody = bytes.NewReader(data)
	}

	req, err := http.NewRequestWithContext(ctx, method, u, reqBody)
	if err != nil {
		return nil, err
	}

	// OAuth認証の場合のみAuthorizationヘッダーを設定
	if c.apiKey == "" && c.accessToken != "" {
		req.Header.Set("Authorization", "Bearer "+c.accessToken)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	return c.httpClient.Do(req)
}

// Get はGETリクエストを実行する
func (c *Client) Get(ctx context.Context, path string, query url.Values) (*http.Response, error) {
	return c.Request(ctx, "GET", path, query, nil)
}

// Post はPOSTリクエストを実行する
func (c *Client) Post(ctx context.Context, path string, body interface{}) (*http.Response, error) {
	return c.Request(ctx, "POST", path, nil, body)
}

// PostForm はフォーム形式でPOSTする
func (c *Client) PostForm(ctx context.Context, path string, data url.Values) (*http.Response, error) {
	// OAuth認証の場合のみトークン更新チェック
	if c.apiKey == "" {
		if err := c.ensureValidToken(ctx); err != nil {
			return nil, fmt.Errorf("token refresh failed: %w", err)
		}
	}

	// API Key認証の場合はURLにapiKeyを追加
	requestURL := c.baseURL() + path
	if c.apiKey != "" {
		requestURL += "?apiKey=" + url.QueryEscape(c.apiKey)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", requestURL, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, err
	}

	// OAuth認証の場合のみAuthorizationヘッダーを設定
	if c.apiKey == "" && c.accessToken != "" {
		req.Header.Set("Authorization", "Bearer "+c.accessToken)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	return c.httpClient.Do(req)
}

// PatchForm はフォーム形式でPATCHする
func (c *Client) PatchForm(ctx context.Context, path string, data url.Values) (*http.Response, error) {
	// OAuth認証の場合のみトークン更新チェック
	if c.apiKey == "" {
		if err := c.ensureValidToken(ctx); err != nil {
			return nil, fmt.Errorf("token refresh failed: %w", err)
		}
	}

	// API Key認証の場合はURLにapiKeyを追加
	requestURL := c.baseURL() + path
	if c.apiKey != "" {
		requestURL += "?apiKey=" + url.QueryEscape(c.apiKey)
	}

	req, err := http.NewRequestWithContext(ctx, "PATCH", requestURL, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, err
	}

	// OAuth認証の場合のみAuthorizationヘッダーを設定
	if c.apiKey == "" && c.accessToken != "" {
		req.Header.Set("Authorization", "Bearer "+c.accessToken)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	return c.httpClient.Do(req)
}

// Patch はPATCHリクエストを実行する
func (c *Client) Patch(ctx context.Context, path string, body interface{}) (*http.Response, error) {
	return c.Request(ctx, "PATCH", path, nil, body)
}

// Delete はDELETEリクエストを実行する
func (c *Client) Delete(ctx context.Context, path string) (*http.Response, error) {
	return c.Request(ctx, "DELETE", path, nil, nil)
}

// RawRequest は任意のパスに対してAPIリクエストを実行する（/api/v2 プレフィックスなし）
// backlog api コマンドなど、任意のAPIバージョンにアクセスする場合に使用
func (c *Client) RawRequest(ctx context.Context, method, path string, query url.Values, body io.Reader, contentType string) (*http.Response, error) {
	// OAuth認証の場合のみトークン更新チェック
	if c.apiKey == "" {
		if err := c.ensureValidToken(ctx); err != nil {
			return nil, fmt.Errorf("token refresh failed: %w", err)
		}
	}

	u := c.RawBaseURL() + path

	// クエリパラメータの構築
	if query == nil {
		query = url.Values{}
	}
	// API Key認証の場合はクエリパラメータに追加
	if c.apiKey != "" {
		query.Set("apiKey", c.apiKey)
	}
	if len(query) > 0 {
		u += "?" + query.Encode()
	}

	req, err := http.NewRequestWithContext(ctx, method, u, body)
	if err != nil {
		return nil, err
	}

	// OAuth認証の場合のみAuthorizationヘッダーを設定
	if c.apiKey == "" && c.accessToken != "" {
		req.Header.Set("Authorization", "Bearer "+c.accessToken)
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}

	return c.httpClient.Do(req)
}

// DeleteWithForm はフォーム形式でDELETEする
func (c *Client) DeleteWithForm(ctx context.Context, path string, data url.Values) (*http.Response, error) {
	// OAuth認証の場合のみトークン更新チェック
	if c.apiKey == "" {
		if err := c.ensureValidToken(ctx); err != nil {
			return nil, fmt.Errorf("token refresh failed: %w", err)
		}
	}

	// API Key認証の場合はURLにapiKeyを追加
	requestURL := c.baseURL() + path
	if c.apiKey != "" {
		requestURL += "?apiKey=" + url.QueryEscape(c.apiKey)
	}

	req, err := http.NewRequestWithContext(ctx, "DELETE", requestURL, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, err
	}

	// OAuth認証の場合のみAuthorizationヘッダーを設定
	if c.apiKey == "" && c.accessToken != "" {
		req.Header.Set("Authorization", "Bearer "+c.accessToken)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	return c.httpClient.Do(req)
}
