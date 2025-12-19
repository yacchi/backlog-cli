# Phase 03: 中継サーバー基本

## 目標

- 中継サーバーの基本構造
- JWT Cookie による状態管理
- `/auth/start` - 認可開始
- `/auth/callback` - コールバック受信
- `/auth/token` - トークン取得・更新
- `/.well-known/backlog-oauth-relay` - メタ情報

## 1. サーバー構造

### internal/relay/server.go

```go
package relay

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/yourorg/backlog-cli/internal/config"
)

// Server は中継サーバー
type Server struct {
	cfg          *config.ServerConfig
	httpServer   *http.Server
	cookieSecret []byte
}

// NewServer は新しいサーバーを作成する
func NewServer(cfg *config.ServerConfig) (*Server, error) {
	if cfg.Cookie.Secret == "" {
		return nil, fmt.Errorf("cookie secret is required")
	}
	
	return &Server{
		cfg:          cfg,
		cookieSecret: []byte(cfg.Cookie.Secret),
	}, nil
}

// Start はサーバーを起動する
func (s *Server) Start() error {
	mux := http.NewServeMux()
	
	// エンドポイント登録
	mux.HandleFunc("GET /health", s.handleHealth)
	mux.HandleFunc("GET /.well-known/backlog-oauth-relay", s.handleWellKnown)
	mux.HandleFunc("GET /auth/start", s.handleAuthStart)
	mux.HandleFunc("GET /auth/callback", s.handleAuthCallback)
	mux.HandleFunc("POST /auth/token", s.handleAuthToken)
	
	addr := fmt.Sprintf("%s:%d", s.cfg.Host, s.cfg.Port)
	
	s.httpServer = &http.Server{
		Addr:         addr,
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}
	
	log.Printf("Starting relay server on %s", addr)
	return s.httpServer.ListenAndServe()
}

// Shutdown はサーバーを停止する
func (s *Server) Shutdown(ctx context.Context) error {
	if s.httpServer != nil {
		return s.httpServer.Shutdown(ctx)
	}
	return nil
}

// handleHealth はヘルスチェック
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("OK"))
}
```

## 2. JWT Cookie

### internal/relay/jwt.go

```go
package relay

import (
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// OAuthSessionClaims はOAuthセッションのJWTクレーム
type OAuthSessionClaims struct {
	Port    int    `json:"port"`
	State   string `json:"state"`
	Domain  string `json:"domain"`
	Space   string `json:"space"`
	Project string `json:"project,omitempty"`
	jwt.RegisteredClaims
}

const (
	cookieName   = "oauth_session"
	cookieMaxAge = 5 * 60 // 5分
)

// createSessionToken はセッショントークンを作成する
func (s *Server) createSessionToken(port int, state, domain, space, project string) (string, error) {
	claims := OAuthSessionClaims{
		Port:    port,
		State:   state,
		Domain:  domain,
		Space:   space,
		Project: project,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Duration(s.cfg.Cookie.MaxAge) * time.Second)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}
	
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(s.cookieSecret)
}

// parseSessionToken はセッショントークンを検証・パースする
func (s *Server) parseSessionToken(tokenString string) (*OAuthSessionClaims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &OAuthSessionClaims{}, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return s.cookieSecret, nil
	})
	
	if err != nil {
		return nil, err
	}
	
	if claims, ok := token.Claims.(*OAuthSessionClaims); ok && token.Valid {
		return claims, nil
	}
	
	return nil, fmt.Errorf("invalid token")
}
```

## 3. Well-Known エンドポイント

### internal/relay/wellknown.go

```go
package relay

import (
	"encoding/json"
	"net/http"
)

// WellKnownResponse は /.well-known/backlog-oauth-relay のレスポンス
type WellKnownResponse struct {
	Version          string   `json:"version"`
	Name             string   `json:"name,omitempty"`
	SupportedDomains []string `json:"supported_domains"`
}

func (s *Server) handleWellKnown(w http.ResponseWriter, r *http.Request) {
	// サポートするドメインを収集
	domains := make([]string, 0, len(s.cfg.Backlog))
	for _, b := range s.cfg.Backlog {
		if b.ClientID != "" && b.ClientSecret != "" {
			domains = append(domains, b.Domain)
		}
	}
	
	resp := WellKnownResponse{
		Version:          "1",
		SupportedDomains: domains,
	}
	
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}
```

## 4. 認可開始 (/auth/start)

### internal/relay/handlers.go

```go
package relay

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
)

// ErrorResponse はエラーレスポンス
type ErrorResponse struct {
	Error       string `json:"error"`
	Description string `json:"error_description,omitempty"`
}

func (s *Server) writeError(w http.ResponseWriter, status int, err, desc string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(ErrorResponse{
		Error:       err,
		Description: desc,
	})
}

func (s *Server) handleAuthStart(w http.ResponseWriter, r *http.Request) {
	// パラメータ取得
	domain := r.URL.Query().Get("domain")
	space := r.URL.Query().Get("space")
	portStr := r.URL.Query().Get("port")
	project := r.URL.Query().Get("project")
	
	// バリデーション
	if domain == "" || space == "" || portStr == "" {
		s.writeError(w, http.StatusBadRequest, "invalid_request", "domain, space, and port are required")
		return
	}
	
	port, err := strconv.Atoi(portStr)
	if err != nil || port < 1024 || port > 65535 {
		s.writeError(w, http.StatusBadRequest, "invalid_request", "port must be between 1024 and 65535")
		return
	}
	
	// Backlog設定を取得
	backlogCfg := s.findBacklogConfig(domain)
	if backlogCfg == nil {
		s.writeError(w, http.StatusBadRequest, "invalid_request", fmt.Sprintf("domain '%s' is not supported", domain))
		return
	}
	
	// state生成
	state, err := generateState()
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "server_error", "failed to generate state")
		return
	}
	
	// JWTトークン作成
	token, err := s.createSessionToken(port, state, domain, space, project)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "server_error", "failed to create session")
		return
	}
	
	// Cookie設定
	http.SetCookie(w, &http.Cookie{
		Name:     cookieName,
		Value:    token,
		Path:     "/auth",
		MaxAge:   s.cfg.Cookie.MaxAge,
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
	})
	
	// Backlog認可URLを構築
	redirectURI := s.buildCallbackURL()
	authURL := fmt.Sprintf("https://%s.%s/OAuth2AccessRequest.action?response_type=code&client_id=%s&redirect_uri=%s&state=%s",
		space,
		domain,
		url.QueryEscape(backlogCfg.ClientID),
		url.QueryEscape(redirectURI),
		url.QueryEscape(state),
	)
	
	// リダイレクト
	http.Redirect(w, r, authURL, http.StatusFound)
}

func (s *Server) findBacklogConfig(domain string) *config.BacklogAppConfig {
	for _, b := range s.cfg.Backlog {
		if b.Domain == domain {
			return &b
		}
	}
	return nil
}

func (s *Server) buildCallbackURL() string {
	if s.cfg.BaseURL != "" {
		return s.cfg.BaseURL + "/auth/callback"
	}
	// デフォルト（開発用）
	return fmt.Sprintf("http://localhost:%d/auth/callback", s.cfg.Port)
}

func generateState() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(b), nil
}
```

## 5. コールバック (/auth/callback)

### internal/relay/handlers.go (続き)

```go
func (s *Server) handleAuthCallback(w http.ResponseWriter, r *http.Request) {
	// パラメータ取得
	code := r.URL.Query().Get("code")
	state := r.URL.Query().Get("state")
	errorParam := r.URL.Query().Get("error")
	
	// Backlogからのエラー
	if errorParam != "" {
		errorDesc := r.URL.Query().Get("error_description")
		s.renderErrorPage(w, "Authorization Failed", errorDesc)
		return
	}
	
	if code == "" || state == "" {
		s.renderErrorPage(w, "Invalid Request", "Missing code or state parameter")
		return
	}
	
	// Cookie取得
	cookie, err := r.Cookie(cookieName)
	if err != nil {
		s.renderErrorPage(w, "Session Expired", "Please try logging in again")
		return
	}
	
	// JWT検証
	claims, err := s.parseSessionToken(cookie.Value)
	if err != nil {
		s.renderErrorPage(w, "Session Invalid", "Please try logging in again")
		return
	}
	
	// state検証
	if claims.State != state {
		s.renderErrorPage(w, "Security Error", "State mismatch detected")
		return
	}
	
	// Cookie削除
	http.SetCookie(w, &http.Cookie{
		Name:     cookieName,
		Value:    "",
		Path:     "/auth",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
	})
	
	// CLIローカルサーバーへリダイレクト
	localURL := fmt.Sprintf("http://localhost:%d/callback?code=%s", claims.Port, url.QueryEscape(code))
	http.Redirect(w, r, localURL, http.StatusFound)
}

func (s *Server) renderErrorPage(w http.ResponseWriter, title, message string) {
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
```

## 6. トークン取得・更新 (/auth/token)

### internal/relay/handlers.go (続き)

```go
import (
	"bytes"
	"io"
)

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

func (s *Server) handleAuthToken(w http.ResponseWriter, r *http.Request) {
	// リクエストボディをパース
	var req TokenRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeError(w, http.StatusBadRequest, "invalid_request", "Invalid JSON body")
		return
	}
	
	// バリデーション
	if req.Domain == "" || req.Space == "" {
		s.writeError(w, http.StatusBadRequest, "invalid_request", "domain and space are required")
		return
	}
	
	backlogCfg := s.findBacklogConfig(req.Domain)
	if backlogCfg == nil {
		s.writeError(w, http.StatusBadRequest, "invalid_request", fmt.Sprintf("domain '%s' is not supported", req.Domain))
		return
	}
	
	var tokenResp *TokenResponse
	var err error
	
	switch req.GrantType {
	case "authorization_code":
		if req.Code == "" {
			s.writeError(w, http.StatusBadRequest, "invalid_request", "code is required for authorization_code grant")
			return
		}
		tokenResp, err = s.exchangeCode(backlogCfg, req.Space, req.Code)
		
	case "refresh_token":
		if req.RefreshToken == "" {
			s.writeError(w, http.StatusBadRequest, "invalid_request", "refresh_token is required for refresh_token grant")
			return
		}
		tokenResp, err = s.refreshToken(backlogCfg, req.Space, req.RefreshToken)
		
	default:
		s.writeError(w, http.StatusBadRequest, "unsupported_grant_type", "Supported: authorization_code, refresh_token")
		return
	}
	
	if err != nil {
		s.writeError(w, http.StatusBadGateway, "upstream_error", err.Error())
		return
	}
	
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(tokenResp)
}

func (s *Server) exchangeCode(cfg *config.BacklogAppConfig, space, code string) (*TokenResponse, error) {
	return s.requestToken(cfg, space, url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"redirect_uri":  {s.buildCallbackURL()},
		"client_id":     {cfg.ClientID},
		"client_secret": {cfg.ClientSecret},
	})
}

func (s *Server) refreshToken(cfg *config.BacklogAppConfig, space, refreshToken string) (*TokenResponse, error) {
	return s.requestToken(cfg, space, url.Values{
		"grant_type":    {"refresh_token"},
		"refresh_token": {refreshToken},
		"client_id":     {cfg.ClientID},
		"client_secret": {cfg.ClientSecret},
	})
}

func (s *Server) requestToken(cfg *config.BacklogAppConfig, space string, params url.Values) (*TokenResponse, error) {
	tokenURL := fmt.Sprintf("https://%s.%s/api/v2/oauth2/token", space, cfg.Domain)
	
	resp, err := http.PostForm(tokenURL, params)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()
	
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}
	
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("token request failed: %s", string(body))
	}
	
	var tokenResp TokenResponse
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}
	
	return &tokenResp, nil
}
```

## 7. Serve コマンド

### internal/cmd/serve/serve.go

```go
package serve

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"
	"github.com/yourorg/backlog-cli/internal/config"
	"github.com/yourorg/backlog-cli/internal/relay"
)

var ServeCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start the OAuth relay server",
	Long: `Start the OAuth relay server for Backlog CLI authentication.

The relay server handles OAuth 2.0 authentication flow, keeping the
client_id and client_secret secure on the server side.`,
	RunE: runServe,
}

var (
	configFile string
	port       int
)

func init() {
	ServeCmd.Flags().StringVarP(&configFile, "config", "c", "", "Config file path")
	ServeCmd.Flags().IntVar(&port, "port", 0, "Server port (overrides config)")
}

func runServe(cmd *cobra.Command, args []string) error {
	// 設定読み込み
	var cfg *config.Config
	var err error
	
	if configFile != "" {
		cfg, err = config.LoadFromFile(configFile)
	} else {
		cfg, err = config.Load()
	}
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}
	
	// ポートのオーバーライド
	if port > 0 {
		cfg.Server.Port = port
	}
	
	// サーバー作成
	srv, err := relay.NewServer(&cfg.Server)
	if err != nil {
		return fmt.Errorf("failed to create server: %w", err)
	}
	
	// シグナルハンドリング
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	
	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.Start()
	}()
	
	select {
	case err := <-errCh:
		return err
	case <-stop:
		fmt.Println("\nShutting down...")
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return srv.Shutdown(ctx)
	}
}
```

### internal/cmd/root.go に追加

```go
import (
	"github.com/yourorg/backlog-cli/internal/cmd/serve"
)

func init() {
	// ...
	rootCmd.AddCommand(serve.ServeCmd)
}
```

## 8. 動作確認手順

### 設定ファイル準備 (テスト用)

```yaml
# ~/.config/backlog/config.yaml
server:
  port: 8080
  base_url: "http://localhost:8080"
  cookie:
    secret: "your-32-byte-secret-key-here!!"
  backlog:
    - domain: "backlog.jp"
      client_id: "YOUR_CLIENT_ID"
      client_secret: "YOUR_CLIENT_SECRET"
```

### 起動

```bash
make dev
./backlog serve
```

### エンドポイント確認

```bash
# ヘルスチェック
curl http://localhost:8080/health

# Well-known
curl http://localhost:8080/.well-known/backlog-oauth-relay

# 認可開始（ブラウザで開く）
# http://localhost:8080/auth/start?domain=backlog.jp&space=your-space&port=12345
```

## 完了条件

- [ ] `backlog serve` でサーバーが起動する
- [ ] `/health` が 200 OK を返す
- [ ] `/.well-known/backlog-oauth-relay` がサポートドメインを返す
- [ ] `/auth/start` が Backlog 認可画面にリダイレクトする
- [ ] `/auth/callback` が state 検証後、CLI にリダイレクトする
- [ ] `/auth/token` がトークン交換・更新できる
- [ ] Cookie が JWT 形式で署名されている

## 次のステップ

`04-cli-auth.md` に進んでCLI側の認証機能を実装してください。
