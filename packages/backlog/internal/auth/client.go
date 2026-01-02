package auth

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/yacchi/backlog-cli/packages/backlog/internal/debug"
)

// Client は認証クライアント
type Client struct {
	relayServer string
	httpClient  *http.Client
}

// NewClient は新しい認証クライアントを作成する
func NewClient(relayServer string) *Client {
	return &Client{
		// 末尾スラッシュを除去してパス連結時のダブルスラッシュを防止
		relayServer: strings.TrimRight(relayServer, "/"),
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// WellKnownResponse は well-known のレスポンス
type WellKnownResponse struct {
	Version          string   `json:"version"`
	Name             string   `json:"name,omitempty"`
	SupportedDomains []string `json:"supported_domains"`
}

// FetchWellKnown は中継サーバーのメタ情報を取得する
func (c *Client) FetchWellKnown() (*WellKnownResponse, error) {
	resp, err := c.httpClient.Get(c.relayServer + "/.well-known/backlog-oauth-relay")
	if err != nil {
		return nil, fmt.Errorf("failed to fetch well-known: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("well-known returned status %d", resp.StatusCode)
	}

	var result WellKnownResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to parse well-known: %w", err)
	}

	return &result, nil
}

// TokenRequest はトークンリクエスト
type TokenRequest struct {
	GrantType    string `json:"grant_type"`
	Code         string `json:"code,omitempty"`
	RefreshToken string `json:"refresh_token,omitempty"`
	Domain       string `json:"domain"`
	Space        string `json:"space"`
	State        string `json:"state,omitempty"` // セッション追跡用（StartAuthで取得した値）
}

// TokenResponse はトークンレスポンス
type TokenResponse struct {
	AccessToken  string `json:"access_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
	RefreshToken string `json:"refresh_token"`
}

// ExchangeToken は認可コードをトークンに交換する
func (c *Client) ExchangeToken(req TokenRequest) (*TokenResponse, error) {
	return c.requestToken(req)
}

// RefreshToken はリフレッシュトークンでアクセストークンを更新する
func (c *Client) RefreshToken(domain, space, refreshToken string) (*TokenResponse, error) {
	return c.requestToken(TokenRequest{
		GrantType:    "refresh_token",
		RefreshToken: refreshToken,
		Domain:       domain,
		Space:        space,
	})
}

func (c *Client) requestToken(req TokenRequest) (*TokenResponse, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}

	tokenURL := c.relayServer + "/auth/token"
	debug.Log("sending token request", "url", tokenURL, "grant_type", req.GrantType)

	resp, err := c.httpClient.Post(
		tokenURL,
		"application/json",
		bytes.NewReader(body),
	)
	if err != nil {
		debug.Log("token request failed", "error", err)
		return nil, fmt.Errorf("token request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	debug.Log("token response received", "status", resp.StatusCode)

	if resp.StatusCode != http.StatusOK {
		var errResp struct {
			Error       string `json:"error"`
			Description string `json:"error_description"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&errResp)
		debug.Log("token request error", "error", errResp.Error, "description", errResp.Description)
		return nil, fmt.Errorf("%s: %s", errResp.Error, errResp.Description)
	}

	var result TokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to parse token response: %w", err)
	}

	debug.Log("token received", "token_type", result.TokenType, "expires_in", result.ExpiresIn)
	return &result, nil
}
