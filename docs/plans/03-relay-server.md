# Phase 03: 中継サーバー基本

## 目標

- 中継サーバーの基本構造
- 署名付きstate による状態管理（Cookie不使用）
- `/auth/start` - 認可開始
- `/auth/callback` - コールバック受信
- `/auth/token` - トークン取得・更新
- `/.well-known/bl-relay` - メタ情報

## 設計変更のポイント

従来のCookie方式から、署名付きstate方式に変更しました。

| 項目 | 旧方式 | 新方式 |
|------|--------|--------|
| state管理 | Cookie | 署名付きstate |
| state生成 | 中継サーバー | CLI（ローカルサーバー） |
| state検証 | 中継サーバー | CLI（ローカルサーバー） |
| 中継サーバーの状態 | Cookie依存 | 完全ステートレス |

### メリット

- サードパーティCookie制限の影響を受けない
- 中継サーバーが完全にステートレスになる
- 将来のPKCE実装が容易

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
	stateSecret  []byte
}

// NewServer は新しいサーバーを作成する
func NewServer(cfg *config.ServerConfig) (*Server, error) {
	if cfg.Cookie.Secret == "" {
		return nil, fmt.Errorf("state signing secret is required")
	}
	if len(cfg.Cookie.Secret) < 32 {
		return nil, fmt.Errorf("state signing secret must be at least 32 bytes")
	}
	
	return &Server{
		cfg:         cfg,
		stateSecret: []byte(cfg.Cookie.Secret),
	}, nil
}

// Start はサーバーを起動する
func (s *Server) Start() error {
	mux := http.NewServeMux()
	
	// エンドポイント登録
	mux.HandleFunc("GET /health", s.handleHealth)
	mux.HandleFunc("GET /.well-known/bl-relay", s.handleWellKnown)
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

## 2. 署名付きstate

### internal/relay/state.go

```go
package relay

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"time"
)

// StatePayload は署名付きstateのペイロード
type StatePayload struct {
	Port     int    `json:"port"`
	CLIState string `json:"cli_state"`
	Space    string `json:"space"`
	Domain   string `json:"domain"`
	Exp      int64  `json:"exp"`
}

const (
	stateMaxAge = 5 * 60 // 5分
)

// createSignedState は署名付きstateを作成する
func (s *Server) createSignedState(port int, cliState, space, domain string) (string, error) {
	payload := StatePayload{
		Port:     port,
		CLIState: cliState,
		Space:    space,
		Domain:   domain,
		Exp:      time.Now().Add(time.Duration(stateMaxAge) * time.Second).Unix(),
	}
	
	// ペイロードをJSON化
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("failed to marshal payload: %w", err)
	}
	
	// Base64エンコード
	payloadB64 := base64.RawURLEncoding.EncodeToString(payloadJSON)
	
	// HMAC署名
	mac := hmac.New(sha256.New, s.stateSecret)
	mac.Write([]byte(payloadB64))
	signature := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	
	// "payload.signature" 形式で返す
	return payloadB64 + "." + signature, nil
}

// parseSignedState は署名付きstateを検証・パースする
func (s *Server) parseSignedState(signedState string) (*StatePayload, error) {
	// "payload.signature" を分離
	var payloadB64, signature string
	for i := len(signedState) - 1; i >= 0; i-- {
		if signedState[i] == '.' {
			payloadB64 = signedState[:i]
			signature = signedState[i+1:]
			break
		}
	}
	
	if payloadB64 == "" || signature == "" {
		return nil, fmt.Errorf("invalid state format")
	}
	
	// 署名検証
	mac := hmac.New(sha256.New, s.stateSecret)
	mac.Write([]byte(payloadB64))
	expectedSig := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	
	if !hmac.Equal([]byte(signature), []byte(expectedSig)) {
		return nil, fmt.Errorf("invalid signature")
	}
	
	// ペイロードをデコード
	payloadJSON, err := base64.RawURLEncoding.DecodeString(payloadB64)
	if err != nil {
		return nil, fmt.Errorf("failed to decode payload: %w", err)
	}
	
	var payload StatePayload
	if err := json.Unmarshal(payloadJSON, &payload); err != nil {
		return nil, fmt.Errorf("failed to unmarshal payload: %w", err)
	}
	
	// 有効期限チェック
	if time.Now().Unix() > payload.Exp {
		return nil, fmt.Errorf("state expired")
	}
	
	return &payload, nil
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

// WellKnownResponse は /.well-known/bl-relay のレスポンス
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
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"

	"github.com/yourorg/backlog-cli/internal/config"
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
	// パラメータ取得（CLIローカルサーバーから送られてくる）
	domain := r.URL.Query().Get("domain")
	space := r.URL.Query().Get("space")
	portStr := r.URL.Query().Get("port")
	cliState := r.URL.Query().Get("state") // CLIが生成したstate
	
	// バリデーション
	if domain == "" || space == "" || portStr == "" || cliState == "" {
		s.writeError(w, http.StatusBadRequest, "invalid_request", "domain, space, port, and state are required")
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
	
	// 署名付きstateを生成（port, cliState, space, domainを含む）
	signedState, err := s.createSignedState(port, cliState, space, domain)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "server_error", "failed to create state")
		return
	}
	
	// Backlog認可URLを構築
	redirectURI := s.buildCallbackURL()
	authURL := fmt.Sprintf("https://%s.%s/OAuth2AccessRequest.action?response_type=code&client_id=%s&redirect_uri=%s&state=%s",
		space,
		domain,
		url.QueryEscape(backlogCfg.ClientID),
		url.QueryEscape(redirectURI),
		url.QueryEscape(signedState),
	)
	
	// Backlogへリダイレクト
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
```

## 5. コールバック (/auth/callback)

### internal/relay/handlers.go (続き)

```go
func (s *Server) handleAuthCallback(w http.ResponseWriter, r *http.Request) {
	// パラメータ取得
	code := r.URL.Query().Get("code")
	signedState := r.URL.Query().Get("state")
	errorParam := r.URL.Query().Get("error")
	
	// Backlogからのエラー
	if errorParam != "" {
		errorDesc := r.URL.Query().Get("error_description")
		s.renderErrorPage(w, "Authorization Failed", errorDesc)
		return
	}
	
	if code == "" || signedState == "" {
		s.renderErrorPage(w, "Invalid Request", "Missing code or state parameter")
		return
	}
	
	// 署名付きstateを検証・パース
	payload, err := s.parseSignedState(signedState)
	if err != nil {
		if err.Error() == "state expired" {
			s.renderErrorPage(w, "Session Expired", "Please try logging in again")
		} else {
			s.renderErrorPage(w, "Security Error", "Invalid state parameter")
		}
		return
	}
	
	// CLIローカルサーバーへリダイレクト（元のcliStateを返す）
	localURL := fmt.Sprintf("http://localhost:%d/callback?code=%s&state=%s",
		payload.Port,
		url.QueryEscape(code),
		url.QueryEscape(payload.CLIState),
	)
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
	"time"

	"github.com/spf13/cobra"
	"github.com/yourorg/backlog-cli/internal/config"
	"github.com/yourorg/backlog-cli/internal/relay"
)

var ServeCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start the OAuth relay server",
	Long: `Start the OAuth relay server for Backlog CLI authentication.

The relay server handles OAuth 2.0 authentication flow, keeping the
client_id and client_secret secure on the server side.

This server is completely stateless - it does not use cookies or
store any session data.`,
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

## 8. 動作確認手順

### 設定ファイル準備 (テスト用)

```yaml
# ~/.config/backlog/config.yaml
server:
  port: 8080
  base_url: "http://localhost:8080"
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
curl http://localhost:8080/.well-known/bl-relay

# 認可開始（CLIローカルサーバーからリダイレクトされる想定）
# http://localhost:8080/auth/start?domain=backlog.jp&space=your-space&port=12345&state=random-state
```

## 完了条件

- [ ] `backlog serve` でサーバーが起動する
- [ ] `/health` が 200 OK を返す
- [ ] `/.well-known/bl-relay` がサポートドメインを返す
- [ ] `/auth/start` が署名付きstateを生成し、Backlog認可画面にリダイレクトする
- [ ] `/auth/callback` が署名を検証し、CLIローカルサーバーにリダイレクトする
- [ ] `/auth/token` がトークン交換・更新できる
- [ ] Cookieを一切使用していない（ステートレス）

## 次のステップ

`04-cli-auth.md` に進んでCLI側の認証機能を実装してください。
