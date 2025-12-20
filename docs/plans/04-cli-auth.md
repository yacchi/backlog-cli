# Phase 04: CLI認証

## 目標

- 対話的UIヘルパー
- ローカルHTTPサーバー（認証フロー開始点）
- 認証クライアント
- `backlog auth login/logout/status/setup` コマンド

## 設計変更のポイント

新しい認証フローでは、CLIがフローの開始点となります。

```
ブラウザ → localhost/auth/start → 中継サーバー → Backlog → 中継サーバー → localhost/callback → CLI
```

### CLIローカルサーバーの役割

1. `/auth/start` - state生成、中継サーバーへリダイレクト
2. `/callback` - state検証、認可コード受信

### メリット

- state管理がCLI側で完結
- Cookie不要（ブラウザ制限の影響なし）
- 将来的なPKCE実装が容易

## 1. 対話的UIヘルパー

### internal/ui/prompt.go

```go
package ui

import (
	"github.com/AlecAivazis/survey/v2"
)

// Select は選択肢から1つを選ばせる
func Select(message string, options []string) (string, error) {
	var result string
	prompt := &survey.Select{
		Message: message,
		Options: options,
	}
	if err := survey.AskOne(prompt, &result); err != nil {
		return "", err
	}
	return result, nil
}

// SelectOption は値と説明を持つ選択肢
type SelectOption struct {
	Value       string
	Description string
}

// SelectWithDesc は説明付き選択肢から1つを選ばせる
func SelectWithDesc(message string, options []SelectOption) (string, error) {
	opts := make([]string, len(options))
	valueMap := make(map[string]string)
	for i, opt := range options {
		display := opt.Value
		if opt.Description != "" {
			display = opt.Value + " - " + opt.Description
		}
		opts[i] = display
		valueMap[display] = opt.Value
	}
	
	var result string
	prompt := &survey.Select{
		Message: message,
		Options: opts,
	}
	if err := survey.AskOne(prompt, &result); err != nil {
		return "", err
	}
	return valueMap[result], nil
}

// Input はテキスト入力を受け付ける
func Input(message string, defaultValue string) (string, error) {
	var result string
	prompt := &survey.Input{
		Message: message,
		Default: defaultValue,
	}
	if err := survey.AskOne(prompt, &result); err != nil {
		return "", err
	}
	return result, nil
}

// Confirm は確認を求める
func Confirm(message string, defaultValue bool) (bool, error) {
	var result bool
	prompt := &survey.Confirm{
		Message: message,
		Default: defaultValue,
	}
	if err := survey.AskOne(prompt, &result); err != nil {
		return false, err
	}
	return result, nil
}

// Password はパスワード入力を受け付ける
func Password(message string) (string, error) {
	var result string
	prompt := &survey.Password{
		Message: message,
	}
	if err := survey.AskOne(prompt, &result); err != nil {
		return "", err
	}
	return result, nil
}
```

## 2. 認証クライアント

### internal/auth/client.go

```go
package auth

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Client は中継サーバーとの通信を行うクライアント
type Client struct {
	relayServer string
	httpClient  *http.Client
}

// NewClient は新しい認証クライアントを作成する
func NewClient(relayServer string) *Client {
	return &Client{
		relayServer: strings.TrimSuffix(relayServer, "/"),
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// WellKnownResponse は /.well-known/bl-relay のレスポンス
type WellKnownResponse struct {
	Version          string   `json:"version"`
	Name             string   `json:"name,omitempty"`
	SupportedDomains []string `json:"supported_domains"`
}

// FetchWellKnown は中継サーバーの情報を取得する
func (c *Client) FetchWellKnown() (*WellKnownResponse, error) {
	resp, err := c.httpClient.Get(c.relayServer + "/.well-known/bl-relay")
	if err != nil {
		return nil, fmt.Errorf("failed to fetch well-known: %w", err)
	}
	defer resp.Body.Close()
	
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status: %d", resp.StatusCode)
	}
	
	var result WellKnownResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
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
}

// TokenResponse はトークンレスポンス
type TokenResponse struct {
	AccessToken  string `json:"access_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
	RefreshToken string `json:"refresh_token"`
}

// ErrorResponse はエラーレスポンス
type ErrorResponse struct {
	Error       string `json:"error"`
	Description string `json:"error_description,omitempty"`
}

// ExchangeToken は認可コードをトークンに交換する
func (c *Client) ExchangeToken(code, space, domain string) (*TokenResponse, error) {
	return c.requestToken(TokenRequest{
		GrantType: "authorization_code",
		Code:      code,
		Space:     space,
		Domain:    domain,
	})
}

// RefreshToken はトークンを更新する
func (c *Client) RefreshToken(refreshToken, space, domain string) (*TokenResponse, error) {
	return c.requestToken(TokenRequest{
		GrantType:    "refresh_token",
		RefreshToken: refreshToken,
		Space:        space,
		Domain:       domain,
	})
}

func (c *Client) requestToken(req TokenRequest) (*TokenResponse, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}
	
	resp, err := c.httpClient.Post(
		c.relayServer+"/auth/token",
		"application/json",
		strings.NewReader(string(body)),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to request token: %w", err)
	}
	defer resp.Body.Close()
	
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}
	
	if resp.StatusCode != http.StatusOK {
		var errResp ErrorResponse
		if err := json.Unmarshal(respBody, &errResp); err != nil {
			return nil, fmt.Errorf("token request failed: %s", string(respBody))
		}
		return nil, fmt.Errorf("%s: %s", errResp.Error, errResp.Description)
	}
	
	var tokenResp TokenResponse
	if err := json.Unmarshal(respBody, &tokenResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}
	
	return &tokenResp, nil
}

// BuildAuthStartURL は認証開始URLを構築する（中継サーバー用）
func (c *Client) BuildAuthStartURL(port int, state, space, domain string) string {
	return fmt.Sprintf("%s/auth/start?port=%d&state=%s&space=%s&domain=%s",
		c.relayServer, port, state, space, domain)
}
```

## 3. コールバックサーバー

### internal/auth/callback.go

```go
package auth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"net"
	"net/http"
	"time"
)

// CallbackResult はコールバックの結果
type CallbackResult struct {
	Code  string
	State string
	Error string
}

// CallbackServer はOAuth認証のコールバックを受け取るローカルサーバー
type CallbackServer struct {
	port        int
	state       string
	relayServer string
	space       string
	domain      string
	resultChan  chan CallbackResult
	server      *http.Server
}

// NewCallbackServer は新しいコールバックサーバーを作成する
func NewCallbackServer(relayServer, space, domain string) (*CallbackServer, error) {
	port, err := FindFreePort()
	if err != nil {
		return nil, fmt.Errorf("failed to find free port: %w", err)
	}
	
	state, err := generateState()
	if err != nil {
		return nil, fmt.Errorf("failed to generate state: %w", err)
	}
	
	return &CallbackServer{
		port:        port,
		state:       state,
		relayServer: relayServer,
		space:       space,
		domain:      domain,
		resultChan:  make(chan CallbackResult, 1),
	}, nil
}

// Port はサーバーのポート番号を返す
func (s *CallbackServer) Port() int {
	return s.port
}

// State はCSRF保護用のstateを返す
func (s *CallbackServer) State() string {
	return s.state
}

// Start はサーバーを起動する
func (s *CallbackServer) Start() error {
	mux := http.NewServeMux()
	mux.HandleFunc("/auth/start", s.handleAuthStart)
	mux.HandleFunc("/callback", s.handleCallback)
	
	s.server = &http.Server{
		Addr:    fmt.Sprintf("127.0.0.1:%d", s.port),
		Handler: mux,
	}
	
	// エラーが発生した場合のみ返す（正常終了時はShutdownで止まる）
	if err := s.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}

// handleAuthStart は認証フローを開始する（中継サーバーへリダイレクト）
func (s *CallbackServer) handleAuthStart(w http.ResponseWriter, r *http.Request) {
	// 中継サーバーの /auth/start へリダイレクト
	redirectURL := fmt.Sprintf("%s/auth/start?port=%d&state=%s&space=%s&domain=%s",
		s.relayServer, s.port, s.state, s.space, s.domain)
	
	http.Redirect(w, r, redirectURL, http.StatusFound)
}

// handleCallback は認可コールバックを処理する
func (s *CallbackServer) handleCallback(w http.ResponseWriter, r *http.Request) {
	code := r.URL.Query().Get("code")
	state := r.URL.Query().Get("state")
	errorParam := r.URL.Query().Get("error")
	
	result := CallbackResult{
		Code:  code,
		State: state,
		Error: errorParam,
	}
	
	// state検証
	if state != s.state {
		result.Error = "state_mismatch"
		s.resultChan <- result
		s.renderErrorPage(w, "Security Error", "State mismatch detected. Please try again.")
		return
	}
	
	if errorParam != "" {
		s.resultChan <- result
		errorDesc := r.URL.Query().Get("error_description")
		s.renderErrorPage(w, "Authorization Failed", errorDesc)
		return
	}
	
	if code == "" {
		result.Error = "missing_code"
		s.resultChan <- result
		s.renderErrorPage(w, "Error", "No authorization code received")
		return
	}
	
	s.resultChan <- result
	s.renderSuccessPage(w)
}

func (s *CallbackServer) renderSuccessPage(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprint(w, `<!DOCTYPE html>
<html>
<head><title>Authentication Successful</title></head>
<body>
<h1>✓ Authentication Successful</h1>
<p>You can close this window and return to the terminal.</p>
<script>window.close();</script>
</body>
</html>`)
}

func (s *CallbackServer) renderErrorPage(w http.ResponseWriter, title, message string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusBadRequest)
	fmt.Fprintf(w, `<!DOCTYPE html>
<html>
<head><title>%s</title></head>
<body>
<h1>%s</h1>
<p>%s</p>
<p>You can close this window.</p>
</body>
</html>`, title, title, message)
}

// WaitForCallback はコールバックを待機する
func (s *CallbackServer) WaitForCallback(timeout time.Duration) (*CallbackResult, error) {
	select {
	case result := <-s.resultChan:
		return &result, nil
	case <-time.After(timeout):
		return nil, fmt.Errorf("timeout waiting for callback")
	}
}

// Shutdown はサーバーを停止する
func (s *CallbackServer) Shutdown() error {
	if s.server != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return s.server.Shutdown(ctx)
	}
	return nil
}

// AuthStartURL は認証開始URLを返す（ブラウザで開くURL）
func (s *CallbackServer) AuthStartURL() string {
	return fmt.Sprintf("http://localhost:%d/auth/start", s.port)
}

// FindFreePort は空いているポートを見つける
func FindFreePort() (int, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer listener.Close()
	
	addr := listener.Addr().(*net.TCPAddr)
	return addr.Port, nil
}

func generateState() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(b), nil
}
```

## 4. Login コマンド

### internal/cmd/auth/login.go

```go
package auth

import (
	"fmt"
	"time"

	"github.com/pkg/browser"
	"github.com/spf13/cobra"
	"github.com/yourorg/backlog-cli/internal/api"
	authpkg "github.com/yourorg/backlog-cli/internal/auth"
	"github.com/yourorg/backlog-cli/internal/config"
	"github.com/yourorg/backlog-cli/internal/ui"
)

var loginCmd = &cobra.Command{
	Use:   "login",
	Short: "Log in to Backlog",
	Long: `Authenticate with Backlog using OAuth 2.0.

This command will:
1. Start a local server for OAuth callback
2. Open your browser for authentication  
3. Exchange the authorization code for tokens
4. Save the tokens locally`,
	RunE: runLogin,
}

var (
	loginSpace  string
	loginDomain string
)

func init() {
	loginCmd.Flags().StringVar(&loginSpace, "space", "", "Backlog space name")
	loginCmd.Flags().StringVar(&loginDomain, "domain", "", "Backlog domain (backlog.jp or backlog.com)")
}

func runLogin(cmd *cobra.Command, args []string) error {
	// 設定読み込み
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}
	
	// 中継サーバーの確認
	relayServer := cfg.Client.Default.RelayServer
	if relayServer == "" {
		return fmt.Errorf("relay server is not configured\nRun 'backlog auth setup <relay-server-url>' first")
	}
	
	// 中継サーバーの情報を取得
	authClient := authpkg.NewClient(relayServer)
	wellKnown, err := authClient.FetchWellKnown()
	if err != nil {
		return fmt.Errorf("failed to connect to relay server: %w", err)
	}
	
	// ドメイン選択
	domain := loginDomain
	if domain == "" {
		domain = cfg.Client.Default.Domain
	}
	if domain == "" {
		if len(wellKnown.SupportedDomains) == 1 {
			domain = wellKnown.SupportedDomains[0]
		} else {
			domain, err = ui.Select("Select Backlog domain:", wellKnown.SupportedDomains)
			if err != nil {
				return err
			}
		}
	}
	
	// ドメインがサポートされているか確認
	domainSupported := false
	for _, d := range wellKnown.SupportedDomains {
		if d == domain {
			domainSupported = true
			break
		}
	}
	if !domainSupported {
		return fmt.Errorf("domain '%s' is not supported by the relay server", domain)
	}
	
	// スペース入力
	space := loginSpace
	if space == "" {
		space = cfg.Client.Default.Space
	}
	if space == "" {
		space, err = ui.Input("Enter your Backlog space name:", "")
		if err != nil {
			return err
		}
	}
	if space == "" {
		return fmt.Errorf("space name is required")
	}
	
	// コールバックサーバー起動
	callbackServer, err := authpkg.NewCallbackServer(relayServer, space, domain)
	if err != nil {
		return fmt.Errorf("failed to create callback server: %w", err)
	}
	
	// サーバーをバックグラウンドで起動
	serverErrCh := make(chan error, 1)
	go func() {
		serverErrCh <- callbackServer.Start()
	}()
	
	// クリーンアップ
	defer callbackServer.Shutdown()
	
	// ブラウザを開く
	authURL := callbackServer.AuthStartURL()
	fmt.Printf("Opening browser for authentication...\n")
	fmt.Printf("If browser doesn't open, visit: %s\n\n", authURL)
	
	if !cfg.Client.Auth.NoBrowser {
		if err := browser.OpenURL(authURL); err != nil {
			fmt.Printf("Failed to open browser: %v\n", err)
			fmt.Printf("Please open the URL manually: %s\n", authURL)
		}
	}
	
	// コールバック待機
	timeout := time.Duration(cfg.Client.Auth.Timeout) * time.Second
	if timeout == 0 {
		timeout = 120 * time.Second
	}
	
	fmt.Println("Waiting for authentication...")
	
	result, err := callbackServer.WaitForCallback(timeout)
	if err != nil {
		return fmt.Errorf("authentication failed: %w", err)
	}
	
	if result.Error != "" {
		return fmt.Errorf("authentication failed: %s", result.Error)
	}
	
	// トークン交換
	fmt.Println("Exchanging authorization code for tokens...")
	
	tokenResp, err := authClient.ExchangeToken(result.Code, space, domain)
	if err != nil {
		return fmt.Errorf("failed to exchange token: %w", err)
	}
	
	// ユーザー情報取得（確認用）
	apiClient := api.NewClient(space, domain, tokenResp.AccessToken)
	user, err := apiClient.GetCurrentUser()
	if err != nil {
		return fmt.Errorf("failed to get user info: %w", err)
	}
	
	// 認証情報を保存
	host := space + "." + domain
	if cfg.Client.Credentials == nil {
		cfg.Client.Credentials = make(map[string]config.Credential)
	}
	cfg.Client.Credentials[host] = config.Credential{
		AccessToken:  tokenResp.AccessToken,
		RefreshToken: tokenResp.RefreshToken,
		ExpiresAt:    time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second),
		UserID:       user.UserID,
		UserName:     user.Name,
	}
	
	// デフォルト設定を更新
	if cfg.Client.Default.Space == "" {
		cfg.Client.Default.Space = space
	}
	if cfg.Client.Default.Domain == "" {
		cfg.Client.Default.Domain = domain
	}
	
	if err := config.Save(cfg); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}
	
	fmt.Printf("\n✓ Logged in as %s (%s)\n", user.Name, user.MailAddress)
	fmt.Printf("  Space: %s.%s\n", space, domain)
	
	// プロジェクト選択を促す
	if cfg.Client.Default.Project == "" {
		fmt.Println("\nTip: Run 'backlog project init' to set a default project")
	}
	
	return nil
}
```

## 5. Logout コマンド

### internal/cmd/auth/logout.go

```go
package auth

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/yourorg/backlog-cli/internal/config"
	"github.com/yourorg/backlog-cli/internal/ui"
)

var logoutCmd = &cobra.Command{
	Use:   "logout",
	Short: "Log out from Backlog",
	Long:  "Remove saved authentication tokens.",
	RunE:  runLogout,
}

var logoutAll bool

func init() {
	logoutCmd.Flags().BoolVar(&logoutAll, "all", false, "Log out from all accounts")
}

func runLogout(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}
	
	if len(cfg.Client.Credentials) == 0 {
		fmt.Println("Not logged in to any account")
		return nil
	}
	
	if logoutAll {
		// 全アカウントからログアウト
		count := len(cfg.Client.Credentials)
		cfg.Client.Credentials = make(map[string]config.Credential)
		
		if err := config.Save(cfg); err != nil {
			return fmt.Errorf("failed to save config: %w", err)
		}
		
		fmt.Printf("✓ Logged out from %d account(s)\n", count)
		return nil
	}
	
	// アカウント選択
	hosts := make([]string, 0, len(cfg.Client.Credentials))
	for host := range cfg.Client.Credentials {
		hosts = append(hosts, host)
	}
	
	var hostToRemove string
	if len(hosts) == 1 {
		hostToRemove = hosts[0]
	} else {
		hostToRemove, err = ui.Select("Select account to log out:", hosts)
		if err != nil {
			return err
		}
	}
	
	delete(cfg.Client.Credentials, hostToRemove)
	
	if err := config.Save(cfg); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}
	
	fmt.Printf("✓ Logged out from %s\n", hostToRemove)
	return nil
}
```

## 6. Status コマンド

### internal/cmd/auth/status.go

```go
package auth

import (
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"
	"github.com/yourorg/backlog-cli/internal/config"
	"github.com/yourorg/backlog-cli/internal/ui"
)

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show authentication status",
	RunE:  runStatus,
}

func runStatus(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}
	
	if len(cfg.Client.Credentials) == 0 {
		fmt.Println("Not logged in")
		fmt.Println("\nRun 'backlog auth login' to authenticate")
		return nil
	}
	
	table := ui.NewTable("HOST", "USER", "STATUS", "EXPIRES")
	
	for host, cred := range cfg.Client.Credentials {
		status := "Active"
		expires := cred.ExpiresAt.Format("2006-01-02 15:04")
		
		if time.Now().After(cred.ExpiresAt) {
			status = "Expired"
		} else if time.Until(cred.ExpiresAt) < 10*time.Minute {
			status = "Expiring soon"
		}
		
		user := cred.UserName
		if user == "" {
			user = cred.UserID
		}
		
		table.AddRow(host, user, status, expires)
	}
	
	table.Render(os.Stdout)
	
	return nil
}
```

## 7. Setup コマンド

### internal/cmd/auth/setup.go

```go
package auth

import (
	"fmt"

	"github.com/spf13/cobra"
	authpkg "github.com/yourorg/backlog-cli/internal/auth"
	"github.com/yourorg/backlog-cli/internal/config"
)

var setupCmd = &cobra.Command{
	Use:   "setup <relay-server-url>",
	Short: "Configure relay server",
	Long: `Configure the OAuth relay server URL.

The relay server handles OAuth authentication, keeping your
client_id and client_secret secure.

Example:
  backlog auth setup https://relay.example.com`,
	Args: cobra.ExactArgs(1),
	RunE: runSetup,
}

func runSetup(cmd *cobra.Command, args []string) error {
	relayServer := args[0]
	
	// 中継サーバーの確認
	client := authpkg.NewClient(relayServer)
	wellKnown, err := client.FetchWellKnown()
	if err != nil {
		return fmt.Errorf("failed to connect to relay server: %w\nMake sure the URL is correct and the server is running", err)
	}
	
	// 設定を保存
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}
	
	cfg.Client.Default.RelayServer = relayServer
	
	if err := config.Save(cfg); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}
	
	fmt.Printf("✓ Relay server configured: %s\n", relayServer)
	fmt.Printf("  Supported domains: %v\n", wellKnown.SupportedDomains)
	fmt.Println("\nRun 'backlog auth login' to authenticate")
	
	return nil
}
```

## 8. サブコマンド登録

### internal/cmd/auth/auth.go (更新)

```go
package auth

import (
	"github.com/spf13/cobra"
)

var AuthCmd = &cobra.Command{
	Use:   "auth",
	Short: "Authenticate with Backlog",
	Long:  "Manage authentication state for Backlog CLI.",
}

func init() {
	AuthCmd.AddCommand(loginCmd)
	AuthCmd.AddCommand(logoutCmd)
	AuthCmd.AddCommand(statusCmd)
	AuthCmd.AddCommand(setupCmd)
}
```

## 9. 最小限のAPIクライアント（確認用）

### internal/api/client.go (最小版)

```go
package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// Client は Backlog API クライアント
type Client struct {
	baseURL     string
	accessToken string
	httpClient  *http.Client
}

// User はユーザー情報
type User struct {
	ID          int    `json:"id"`
	UserID      string `json:"userId"`
	Name        string `json:"name"`
	MailAddress string `json:"mailAddress"`
	RoleType    int    `json:"roleType"`
}

// NewClient は新しいAPIクライアントを作成する
func NewClient(space, domain, accessToken string) *Client {
	return &Client{
		baseURL:     fmt.Sprintf("https://%s.%s/api/v2", space, domain),
		accessToken: accessToken,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// GetCurrentUser は現在のユーザー情報を取得する
func (c *Client) GetCurrentUser() (*User, error) {
	req, err := http.NewRequest("GET", c.baseURL+"/users/myself", nil)
	if err != nil {
		return nil, err
	}
	
	req.Header.Set("Authorization", "Bearer "+c.accessToken)
	
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status: %d", resp.StatusCode)
	}
	
	var user User
	if err := json.NewDecoder(resp.Body).Decode(&user); err != nil {
		return nil, err
	}
	
	return &user, nil
}
```

## 完了条件

- [ ] `backlog auth setup <url>` で中継サーバーを設定できる
- [ ] `backlog auth login` で対話的にログインできる
- [ ] ブラウザが自動で開く
- [ ] ローカルサーバーが `/auth/start` で中継サーバーへリダイレクトする
- [ ] `/callback` でstate検証が行われる
- [ ] トークンが設定ファイルに保存される
- [ ] `backlog auth status` で認証状態が表示される
- [ ] `backlog auth logout` でログアウトできる
- [ ] Cookie を一切使用していない

## 次のステップ

`05-relay-advanced.md` に進んで中継サーバーの拡張機能を実装してください。
