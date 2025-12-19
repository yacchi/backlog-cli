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
	"github.com/yacchi/backlog-cli/internal/backlog"
	"github.com/yacchi/backlog-cli/internal/config"
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
	onTokenUpdate func(accessToken, refreshToken string, expiresAt time.Time)
}

// ClientOption はクライアントオプション
type ClientOption func(*Client)

// WithTokenRefresh はトークン自動更新を有効にする（OAuth用）
func WithTokenRefresh(refreshToken, relayServer string, expiresAt time.Time, callback func(string, string, time.Time)) ClientOption {
	return func(c *Client) {
		c.refreshToken = refreshToken
		c.relayServer = relayServer
		c.expiresAt = expiresAt
		c.onTokenUpdate = callback
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
		space:       space,
		domain:      domain,
		accessToken: accessToken,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}

	for _, opt := range opts {
		opt(c)
	}

	// ogen クライアントの初期化
	bc, err := backlog.NewClient(c.baseURL(), c)
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
		if err := c.ensureValidToken(); err != nil {
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

	// 認証タイプに応じてクライアントを作成
	switch cred.GetAuthType() {
	case config.AuthTypeAPIKey:
		// API Key認証
		client := NewClient(
			space,
			domain,
			"", // accessToken不要
			WithAPIKey(cred.APIKey),
		)
		return client, nil

	default:
		// OAuth認証（デフォルト）
		profileName := resolved.ActiveProfile
		client := NewClient(
			space,
			domain,
			cred.AccessToken,
			WithTokenRefresh(
				cred.RefreshToken,
				profile.RelayServer,
				cred.ExpiresAt,
				func(accessToken, refreshToken string, expiresAt time.Time) {
					// 設定ファイルを更新（プロファイルに紐づける）
					ctx := context.Background()
					cfg.SetCredential(profileName, &config.Credential{
						AuthType:     config.AuthTypeOAuth,
						AccessToken:  accessToken,
						RefreshToken: refreshToken,
						ExpiresAt:    expiresAt,
						UserID:       cred.UserID,
						UserName:     cred.UserName,
					})
					cfg.Save(ctx)
				},
			),
		)
		return client, nil
	}
}

func (c *Client) baseURL() string {
	return fmt.Sprintf("https://%s.%s/api/v2", c.space, c.domain)
}

// ensureValidToken はトークンが有効か確認し、必要なら更新する
func (c *Client) ensureValidToken() error {
	if c.refreshToken == "" || c.relayServer == "" {
		return nil // 自動更新なし
	}

	// 有効期限の5分前に更新
	if time.Now().Add(5 * time.Minute).Before(c.expiresAt) {
		return nil // まだ有効
	}

	// トークン更新
	return c.doRefreshToken()
}

func (c *Client) doRefreshToken() error {
	reqBody := map[string]string{
		"grant_type":    "refresh_token",
		"refresh_token": c.refreshToken,
		"domain":        c.domain,
		"space":         c.space,
	}

	body, _ := json.Marshal(reqBody)
	resp, err := c.httpClient.Post(
		c.relayServer+"/auth/token",
		"application/json",
		bytes.NewReader(body),
	)
	if err != nil {
		return fmt.Errorf("token refresh request failed: %w", err)
	}
	defer resp.Body.Close()

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

	// コールバック
	if c.onTokenUpdate != nil {
		c.onTokenUpdate(c.accessToken, c.refreshToken, c.expiresAt)
	}

	return nil
}

// Request はAPIリクエストを実行する
func (c *Client) Request(method, path string, query url.Values, body interface{}) (*http.Response, error) {
	// OAuth認証の場合のみトークン更新チェック
	if c.apiKey == "" {
		if err := c.ensureValidToken(); err != nil {
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

	req, err := http.NewRequest(method, u, reqBody)
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
func (c *Client) Get(path string, query url.Values) (*http.Response, error) {
	return c.Request("GET", path, query, nil)
}

// Post はPOSTリクエストを実行する
func (c *Client) Post(path string, body interface{}) (*http.Response, error) {
	return c.Request("POST", path, nil, body)
}

// PostForm はフォーム形式でPOSTする
func (c *Client) PostForm(path string, data url.Values) (*http.Response, error) {
	// OAuth認証の場合のみトークン更新チェック
	if c.apiKey == "" {
		if err := c.ensureValidToken(); err != nil {
			return nil, fmt.Errorf("token refresh failed: %w", err)
		}
	}

	// API Key認証の場合はURLにapiKeyを追加
	requestURL := c.baseURL() + path
	if c.apiKey != "" {
		requestURL += "?apiKey=" + url.QueryEscape(c.apiKey)
	}

	req, err := http.NewRequest("POST", requestURL, strings.NewReader(data.Encode()))
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
func (c *Client) PatchForm(path string, data url.Values) (*http.Response, error) {
	// OAuth認証の場合のみトークン更新チェック
	if c.apiKey == "" {
		if err := c.ensureValidToken(); err != nil {
			return nil, fmt.Errorf("token refresh failed: %w", err)
		}
	}

	// API Key認証の場合はURLにapiKeyを追加
	requestURL := c.baseURL() + path
	if c.apiKey != "" {
		requestURL += "?apiKey=" + url.QueryEscape(c.apiKey)
	}

	req, err := http.NewRequest("PATCH", requestURL, strings.NewReader(data.Encode()))
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
func (c *Client) Patch(path string, body interface{}) (*http.Response, error) {
	return c.Request("PATCH", path, nil, body)
}

// Delete はDELETEリクエストを実行する
func (c *Client) Delete(path string) (*http.Response, error) {
	return c.Request("DELETE", path, nil, nil)
}
