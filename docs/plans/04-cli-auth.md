# Phase 04: CLI認証

## 目標

- `backlog auth login` - OAuth2.0 ログイン
- `backlog auth logout` - ログアウト
- `backlog auth status` - 認証状態確認
- `backlog auth setup` - 中継サーバー設定
- 対話的UI（ドメイン選択、スペース入力、プロジェクト選択）
- ブラウザが開けない場合のフォールバック

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

// SelectWithDescription は説明付きの選択肢から1つを選ばせる
type SelectOption struct {
	Value       string
	Description string
}

func SelectWithDesc(message string, options []SelectOption) (string, error) {
	labels := make([]string, len(options))
	valueMap := make(map[string]string)
	
	for i, opt := range options {
		if opt.Description != "" {
			labels[i] = opt.Value + " - " + opt.Description
		} else {
			labels[i] = opt.Value
		}
		valueMap[labels[i]] = opt.Value
	}
	
	var result string
	prompt := &survey.Select{
		Message: message,
		Options: labels,
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

// Confirm は確認プロンプトを表示する
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
```

## 2. 認証クライアント

### internal/auth/client.go

```go
package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"time"
)

// Client は認証クライアント
type Client struct {
	relayServer string
	httpClient  *http.Client
}

// NewClient は新しい認証クライアントを作成する
func NewClient(relayServer string) *Client {
	return &Client{
		relayServer: relayServer,
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
	defer resp.Body.Close()
	
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
	
	resp, err := c.httpClient.Post(
		c.relayServer+"/auth/token",
		"application/json",
		bytes.NewReader(body),
	)
	if err != nil {
		return nil, fmt.Errorf("token request failed: %w", err)
	}
	defer resp.Body.Close()
	
	if resp.StatusCode != http.StatusOK {
		var errResp struct {
			Error       string `json:"error"`
			Description string `json:"error_description"`
		}
		json.NewDecoder(resp.Body).Decode(&errResp)
		return nil, fmt.Errorf("%s: %s", errResp.Error, errResp.Description)
	}
	
	var result TokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to parse token response: %w", err)
	}
	
	return &result, nil
}
```

## 3. コールバックサーバー

### internal/auth/callback.go

```go
package auth

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"sync"
)

// CallbackResult はコールバックの結果
type CallbackResult struct {
	Code  string
	Error error
}

// CallbackServer はCLIのローカルコールバックサーバー
type CallbackServer struct {
	port     int
	server   *http.Server
	result   chan CallbackResult
	listener net.Listener
	once     sync.Once
}

// NewCallbackServer は新しいコールバックサーバーを作成する
func NewCallbackServer(port int) (*CallbackServer, error) {
	// ポートが0の場合は空きポートを探す
	addr := fmt.Sprintf("127.0.0.1:%d", port)
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("failed to listen on %s: %w", addr, err)
	}
	
	// 実際のポートを取得
	actualPort := listener.Addr().(*net.TCPAddr).Port
	
	cs := &CallbackServer{
		port:     actualPort,
		result:   make(chan CallbackResult, 1),
		listener: listener,
	}
	
	mux := http.NewServeMux()
	mux.HandleFunc("/callback", cs.handleCallback)
	
	cs.server = &http.Server{
		Handler: mux,
	}
	
	return cs, nil
}

// Port は実際のポート番号を返す
func (cs *CallbackServer) Port() int {
	return cs.port
}

// Start はサーバーを起動する
func (cs *CallbackServer) Start() error {
	return cs.server.Serve(cs.listener)
}

// Wait はコールバックを待機する
func (cs *CallbackServer) Wait() CallbackResult {
	return <-cs.result
}

// Shutdown はサーバーを停止する
func (cs *CallbackServer) Shutdown(ctx context.Context) error {
	return cs.server.Shutdown(ctx)
}

func (cs *CallbackServer) handleCallback(w http.ResponseWriter, r *http.Request) {
	cs.once.Do(func() {
		code := r.URL.Query().Get("code")
		errorParam := r.URL.Query().Get("error")
		
		if errorParam != "" {
			errorDesc := r.URL.Query().Get("error_description")
			cs.result <- CallbackResult{
				Error: fmt.Errorf("%s: %s", errorParam, errorDesc),
			}
		} else if code == "" {
			cs.result <- CallbackResult{
				Error: fmt.Errorf("no code received"),
			}
		} else {
			cs.result <- CallbackResult{Code: code}
		}
	})
	
	// 成功ページを表示
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

// FindFreePort は空いているポートを探す
func FindFreePort() (int, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer listener.Close()
	return listener.Addr().(*net.TCPAddr).Port, nil
}
```

## 4. Login コマンド

### internal/cmd/auth/login.go

```go
package auth

import (
	"context"
	"fmt"
	"os"
	"slices"
	"time"

	"github.com/pkg/browser"
	"github.com/spf13/cobra"
	"github.com/yourorg/backlog-cli/internal/api"
	"github.com/yourorg/backlog-cli/internal/auth"
	"github.com/yourorg/backlog-cli/internal/config"
	"github.com/yourorg/backlog-cli/internal/ui"
)

var loginCmd = &cobra.Command{
	Use:   "login",
	Short: "Log in to Backlog",
	Long: `Authenticate with Backlog using OAuth 2.0.

This command opens a browser window for authentication. If the browser
cannot be opened, a URL will be displayed for manual access.`,
	RunE: runLogin,
}

var (
	loginDomain       string
	loginSpace        string
	loginProject      string
	loginNoBrowser    bool
	loginCallbackPort int
	loginTimeout      int
)

func init() {
	loginCmd.Flags().StringVar(&loginDomain, "domain", "", "Backlog domain (backlog.jp or backlog.com)")
	loginCmd.Flags().StringVar(&loginSpace, "space", "", "Backlog space name")
	loginCmd.Flags().StringVar(&loginProject, "project", "", "Default project key")
	loginCmd.Flags().BoolVar(&loginNoBrowser, "no-browser", false, "Don't open browser, just print URL")
	loginCmd.Flags().IntVar(&loginCallbackPort, "callback-port", 0, "Fixed port for callback server")
	loginCmd.Flags().IntVar(&loginTimeout, "timeout", 0, "Timeout in seconds (default: 120)")
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
	
	// オプションのマージ
	opts := mergeLoginOptions(cfg)
	
	// 認証クライアント作成
	client := auth.NewClient(relayServer)
	
	// 1. well-known からメタ情報取得
	fmt.Println("Fetching relay server information...")
	meta, err := client.FetchWellKnown()
	if err != nil {
		return fmt.Errorf("failed to connect to relay server: %w", err)
	}
	
	if len(meta.SupportedDomains) == 0 {
		return fmt.Errorf("relay server has no supported domains configured")
	}
	
	// 2. ドメイン選択
	if opts.domain == "" {
		if len(meta.SupportedDomains) == 1 {
			opts.domain = meta.SupportedDomains[0]
		} else {
			opts.domain, err = ui.Select("Select Backlog domain:", meta.SupportedDomains)
			if err != nil {
				return err
			}
		}
	} else if !slices.Contains(meta.SupportedDomains, opts.domain) {
		return fmt.Errorf("domain '%s' is not supported by this relay server\nSupported: %v", opts.domain, meta.SupportedDomains)
	}
	
	// 3. スペース入力
	if opts.space == "" {
		opts.space, err = ui.Input("Enter space name:", "")
		if err != nil {
			return err
		}
		if opts.space == "" {
			return fmt.Errorf("space name is required")
		}
	}
	
	fmt.Printf("\nAuthenticating with %s.%s...\n", opts.space, opts.domain)
	
	// 4. コールバックサーバー起動
	callbackServer, err := auth.NewCallbackServer(opts.callbackPort)
	if err != nil {
		return fmt.Errorf("failed to start callback server: %w", err)
	}
	
	go callbackServer.Start()
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		callbackServer.Shutdown(ctx)
	}()
	
	// 5. 認可URL生成
	authURL := fmt.Sprintf("%s/auth/start?domain=%s&space=%s&port=%d",
		relayServer,
		opts.domain,
		opts.space,
		callbackServer.Port(),
	)
	if opts.project != "" {
		authURL += "&project=" + opts.project
	}
	
	// 6. URL表示 & ブラウザ起動
	fmt.Println()
	fmt.Println("If browser doesn't open automatically, visit this URL:")
	fmt.Println()
	fmt.Printf("  %s\n", authURL)
	fmt.Println()
	fmt.Printf("Waiting for authentication... (timeout: %ds)\n", opts.timeout)
	
	if !opts.noBrowser {
		if err := browser.OpenURL(authURL); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: could not open browser: %v\n", err)
		}
	}
	
	// 7. コールバック待機
	resultCh := make(chan auth.CallbackResult, 1)
	go func() {
		resultCh <- callbackServer.Wait()
	}()
	
	var result auth.CallbackResult
	select {
	case result = <-resultCh:
	case <-time.After(time.Duration(opts.timeout) * time.Second):
		return fmt.Errorf("authentication timed out after %d seconds", opts.timeout)
	}
	
	if result.Error != nil {
		return fmt.Errorf("authentication failed: %w", result.Error)
	}
	
	// 8. トークン交換
	fmt.Println("Exchanging authorization code...")
	tokenResp, err := client.ExchangeToken(auth.TokenRequest{
		GrantType: "authorization_code",
		Code:      result.Code,
		Domain:    opts.domain,
		Space:     opts.space,
	})
	if err != nil {
		return fmt.Errorf("failed to exchange token: %w", err)
	}
	
	// 9. ユーザー情報取得
	apiClient := api.NewClient(opts.space, opts.domain, tokenResp.AccessToken)
	user, err := apiClient.GetCurrentUser()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not fetch user info: %v\n", err)
	} else {
		fmt.Printf("✓ Authenticated as %s", user.Name)
		if user.MailAddress != "" {
			fmt.Printf(" (%s)", user.MailAddress)
		}
		fmt.Println()
	}
	
	// 10. デフォルトプロジェクト選択（初回のみ）
	host := opts.space + "." + opts.domain
	if opts.project == "" && cfg.Client.Credentials[host].AccessToken == "" {
		projects, err := apiClient.GetProjects()
		if err == nil && len(projects) > 0 {
			projectOpts := make([]ui.SelectOption, len(projects)+1)
			for i, p := range projects {
				projectOpts[i] = ui.SelectOption{
					Value:       p.ProjectKey,
					Description: p.Name,
				}
			}
			projectOpts[len(projects)] = ui.SelectOption{Value: "(Skip)"}
			
			selected, err := ui.SelectWithDesc("Select default project:", projectOpts)
			if err == nil && selected != "(Skip)" {
				opts.project = selected
			}
		}
	}
	
	// 11. 認証情報保存
	if cfg.Client.Credentials == nil {
		cfg.Client.Credentials = make(map[string]config.Credential)
	}
	
	cfg.Client.Credentials[host] = config.Credential{
		AccessToken:  tokenResp.AccessToken,
		RefreshToken: tokenResp.RefreshToken,
		ExpiresAt:    time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second),
		UserID:       "",
		UserName:     "",
	}
	if user != nil {
		cfg.Client.Credentials[host] = config.Credential{
			AccessToken:  tokenResp.AccessToken,
			RefreshToken: tokenResp.RefreshToken,
			ExpiresAt:    time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second),
			UserID:       fmt.Sprintf("%d", user.ID),
			UserName:     user.Name,
		}
	}
	
	// デフォルト設定の更新
	if cfg.Client.Default.Space == "" {
		cfg.Client.Default.Space = opts.space
	}
	if cfg.Client.Default.Domain == "" {
		cfg.Client.Default.Domain = opts.domain
	}
	if opts.project != "" && cfg.Client.Default.Project == "" {
		cfg.Client.Default.Project = opts.project
	}
	
	if err := config.Save(cfg); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}
	
	fmt.Printf("✓ Logged in to %s.%s\n", opts.space, opts.domain)
	if opts.project != "" {
		fmt.Printf("✓ Default project: %s\n", opts.project)
	}
	
	return nil
}

type loginOptions struct {
	domain       string
	space        string
	project      string
	noBrowser    bool
	callbackPort int
	timeout      int
}

func mergeLoginOptions(cfg *config.Config) loginOptions {
	opts := loginOptions{
		domain:       loginDomain,
		space:        loginSpace,
		project:      loginProject,
		noBrowser:    loginNoBrowser,
		callbackPort: loginCallbackPort,
		timeout:      loginTimeout,
	}
	
	// 設定ファイルからの補完
	if opts.domain == "" {
		opts.domain = cfg.Client.Default.Domain
	}
	if opts.space == "" {
		opts.space = cfg.Client.Default.Space
	}
	if opts.callbackPort == 0 {
		opts.callbackPort = cfg.Client.Auth.CallbackPort
	}
	if opts.timeout == 0 {
		opts.timeout = cfg.Client.Auth.Timeout
	}
	if !opts.noBrowser {
		opts.noBrowser = cfg.Client.Auth.NoBrowser
	}
	
	// デフォルト値
	if opts.timeout == 0 {
		opts.timeout = 120
	}
	
	return opts
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
	Long:  "Remove stored authentication credentials.",
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
		cfg.Client.Credentials = make(map[string]config.Credential)
		if err := config.Save(cfg); err != nil {
			return fmt.Errorf("failed to save config: %w", err)
		}
		fmt.Println("✓ Logged out from all accounts")
		return nil
	}
	
	// アカウント選択
	hosts := make([]string, 0, len(cfg.Client.Credentials))
	for host := range cfg.Client.Credentials {
		hosts = append(hosts, host)
	}
	
	var host string
	if len(hosts) == 1 {
		host = hosts[0]
	} else {
		host, err = ui.Select("Select account to log out:", hosts)
		if err != nil {
			return err
		}
	}
	
	delete(cfg.Client.Credentials, host)
	
	if err := config.Save(cfg); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}
	
	fmt.Printf("✓ Logged out from %s\n", host)
	return nil
}
```

## 6. Status コマンド

### internal/cmd/auth/status.go

```go
package auth

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"
	"github.com/yourorg/backlog-cli/internal/config"
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
		fmt.Println("Not logged in to any account")
		fmt.Println()
		fmt.Println("Run 'backlog auth login' to authenticate")
		return nil
	}
	
	fmt.Println("Authenticated accounts:")
	fmt.Println()
	
	for host, cred := range cfg.Client.Credentials {
		fmt.Printf("  %s\n", host)
		if cred.UserName != "" {
			fmt.Printf("    User: %s\n", cred.UserName)
		}
		
		if cred.ExpiresAt.IsZero() {
			fmt.Println("    Token: valid")
		} else if time.Now().After(cred.ExpiresAt) {
			fmt.Println("    Token: expired (will refresh on next request)")
		} else {
			remaining := time.Until(cred.ExpiresAt).Round(time.Minute)
			fmt.Printf("    Token: valid (expires in %s)\n", remaining)
		}
		fmt.Println()
	}
	
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
	"github.com/yourorg/backlog-cli/internal/auth"
	"github.com/yourorg/backlog-cli/internal/config"
)

var setupCmd = &cobra.Command{
	Use:   "setup <relay-server-url>",
	Short: "Configure relay server",
	Long: `Configure the OAuth relay server URL.

The relay server handles OAuth authentication, keeping the client_id
and client_secret secure on the server side.

Example:
  backlog auth setup https://relay.example.com`,
	Args: cobra.ExactArgs(1),
	RunE: runSetup,
}

func runSetup(cmd *cobra.Command, args []string) error {
	relayServer := args[0]
	
	// well-known を取得して確認
	fmt.Printf("Fetching relay server information from %s...\n", relayServer)
	
	client := auth.NewClient(relayServer)
	meta, err := client.FetchWellKnown()
	if err != nil {
		return fmt.Errorf("failed to connect to relay server: %w", err)
	}
	
	fmt.Println()
	fmt.Printf("✓ Relay server: %s\n", relayServer)
	if meta.Name != "" {
		fmt.Printf("✓ Name: %s\n", meta.Name)
	}
	fmt.Printf("✓ Supported domains: %v\n", meta.SupportedDomains)
	
	// 設定に保存
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}
	
	cfg.Client.Default.RelayServer = relayServer
	
	if err := config.Save(cfg); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}
	
	fmt.Println()
	fmt.Println("✓ Configuration saved")
	fmt.Println()
	fmt.Println("Run 'backlog auth login' to authenticate")
	
	return nil
}
```

## 8. 最小限のAPIクライアント

### internal/api/client.go (一部)

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
	space       string
	domain      string
	accessToken string
	httpClient  *http.Client
}

// NewClient は新しいクライアントを作成する
func NewClient(space, domain, accessToken string) *Client {
	return &Client{
		space:       space,
		domain:      domain,
		accessToken: accessToken,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

func (c *Client) baseURL() string {
	return fmt.Sprintf("https://%s.%s/api/v2", c.space, c.domain)
}

func (c *Client) doRequest(method, path string, body interface{}) (*http.Response, error) {
	req, err := http.NewRequest(method, c.baseURL()+path, nil)
	if err != nil {
		return nil, err
	}
	
	req.Header.Set("Authorization", "Bearer "+c.accessToken)
	req.Header.Set("Content-Type", "application/json")
	
	return c.httpClient.Do(req)
}

// User はユーザー情報
type User struct {
	ID          int    `json:"id"`
	UserID      string `json:"userId"`
	Name        string `json:"name"`
	RoleType    int    `json:"roleType"`
	Lang        string `json:"lang"`
	MailAddress string `json:"mailAddress"`
}

// GetCurrentUser は認証ユーザーの情報を取得する
func (c *Client) GetCurrentUser() (*User, error) {
	resp, err := c.doRequest("GET", "/users/myself", nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API returned status %d", resp.StatusCode)
	}
	
	var user User
	if err := json.NewDecoder(resp.Body).Decode(&user); err != nil {
		return nil, err
	}
	
	return &user, nil
}

// Project はプロジェクト情報
type Project struct {
	ID         int    `json:"id"`
	ProjectKey string `json:"projectKey"`
	Name       string `json:"name"`
}

// GetProjects はプロジェクト一覧を取得する
func (c *Client) GetProjects() ([]Project, error) {
	resp, err := c.doRequest("GET", "/projects", nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API returned status %d", resp.StatusCode)
	}
	
	var projects []Project
	if err := json.NewDecoder(resp.Body).Decode(&projects); err != nil {
		return nil, err
	}
	
	return projects, nil
}
```

## 完了条件

- [ ] `backlog auth setup <url>` で中継サーバーが設定できる
- [ ] `backlog auth login` でブラウザが開き、認証できる
- [ ] `--no-browser` オプションでURLのみ表示できる
- [ ] `--callback-port` でポートを固定できる
- [ ] 対話的にドメイン選択、スペース入力ができる
- [ ] 初回ログイン時にプロジェクト選択ができる
- [ ] `backlog auth status` で認証状態が表示される
- [ ] `backlog auth logout` でログアウトできる

## 次のステップ

`05-relay-advanced.md` に進んで中継サーバーの拡張機能を実装してください。
