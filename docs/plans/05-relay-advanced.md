# Phase 05: 中継サーバー拡張

## 目標

- アクセス制御（IP制限、スペース制限、プロジェクト制限）
- 監査ログ（stdout, file, webhook）
- Rate Limiting
- ミドルウェアの整理

## 1. ミドルウェア構造

### internal/relay/middleware.go

```go
package relay

import (
	"log"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

// Middleware はHTTPミドルウェアの型
type Middleware func(http.Handler) http.Handler

// Chain は複数のミドルウェアをチェーンする
func Chain(h http.Handler, middlewares ...Middleware) http.Handler {
	for i := len(middlewares) - 1; i >= 0; i-- {
		h = middlewares[i](h)
	}
	return h
}

// LoggingMiddleware はリクエストをログ出力する
func LoggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		
		// レスポンスをラップしてステータスコードを取得
		wrapped := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}
		
		next.ServeHTTP(wrapped, r)
		
		log.Printf("%s %s %d %s",
			r.Method,
			r.URL.Path,
			wrapped.statusCode,
			time.Since(start).Round(time.Millisecond),
		)
	})
}

type responseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

// RecoveryMiddleware はパニックをリカバーする
func RecoveryMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if err := recover(); err != nil {
				log.Printf("panic recovered: %v", err)
				http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			}
		}()
		next.ServeHTTP(wrapped, r)
	})
}
```

## 2. IP制限

### internal/relay/access.go

```go
package relay

import (
	"fmt"
	"net"
	"net/http"
	"strings"
)

// IPRestriction はIP制限の設定
type IPRestriction struct {
	allowedNets []*net.IPNet
}

// NewIPRestriction は新しいIP制限を作成する
func NewIPRestriction(cidrs []string) (*IPRestriction, error) {
	if len(cidrs) == 0 {
		return &IPRestriction{}, nil
	}
	
	nets := make([]*net.IPNet, 0, len(cidrs))
	for _, cidr := range cidrs {
		_, ipNet, err := net.ParseCIDR(cidr)
		if err != nil {
			// CIDRでない場合は単一IPとして扱う
			ip := net.ParseIP(cidr)
			if ip == nil {
				return nil, fmt.Errorf("invalid CIDR or IP: %s", cidr)
			}
			// /32 or /128 として扱う
			bits := 32
			if ip.To4() == nil {
				bits = 128
			}
			ipNet = &net.IPNet{IP: ip, Mask: net.CIDRMask(bits, bits)}
		}
		nets = append(nets, ipNet)
	}
	
	return &IPRestriction{allowedNets: nets}, nil
}

// IsAllowed はIPが許可されているか確認する
func (ir *IPRestriction) IsAllowed(ip net.IP) bool {
	if len(ir.allowedNets) == 0 {
		return true // 制限なし
	}
	
	for _, ipNet := range ir.allowedNets {
		if ipNet.Contains(ip) {
			return true
		}
	}
	return false
}

// Middleware はIP制限ミドルウェアを返す
func (ir *IPRestriction) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if len(ir.allowedNets) == 0 {
			next.ServeHTTP(w, r)
			return
		}
		
		ip := getClientIP(r)
		if ip == nil || !ir.IsAllowed(ip) {
			http.Error(w, "Access denied", http.StatusForbidden)
			return
		}
		
		next.ServeHTTP(w, r)
	})
}

// getClientIP はクライアントIPを取得する
func getClientIP(r *http.Request) net.IP {
	// X-Forwarded-For ヘッダーをチェック
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		ips := strings.Split(xff, ",")
		if len(ips) > 0 {
			ip := net.ParseIP(strings.TrimSpace(ips[0]))
			if ip != nil {
				return ip
			}
		}
	}
	
	// X-Real-IP ヘッダーをチェック
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		ip := net.ParseIP(xri)
		if ip != nil {
			return ip
		}
	}
	
	// RemoteAddr から取得
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return nil
	}
	return net.ParseIP(host)
}
```

## 3. スペース・プロジェクト制限

### internal/relay/access.go (続き)

```go
// AccessController はアクセス制御
type AccessController struct {
	allowedSpaces   map[string]struct{}
	allowedProjects map[string]struct{}
}

// NewAccessController は新しいアクセス制御を作成する
func NewAccessController(spaces, projects []string) *AccessController {
	ac := &AccessController{
		allowedSpaces:   make(map[string]struct{}),
		allowedProjects: make(map[string]struct{}),
	}
	
	for _, s := range spaces {
		ac.allowedSpaces[s] = struct{}{}
	}
	for _, p := range projects {
		ac.allowedProjects[p] = struct{}{}
	}
	
	return ac
}

// CheckSpace はスペースが許可されているか確認する
func (ac *AccessController) CheckSpace(space string) error {
	if len(ac.allowedSpaces) == 0 {
		return nil // 制限なし
	}
	
	if _, ok := ac.allowedSpaces[space]; !ok {
		return fmt.Errorf("space '%s' is not allowed", space)
	}
	return nil
}

// CheckProject はプロジェクトが許可されているか確認する
func (ac *AccessController) CheckProject(project string) error {
	if len(ac.allowedProjects) == 0 || project == "" {
		return nil // 制限なしまたはプロジェクト指定なし
	}
	
	if _, ok := ac.allowedProjects[project]; !ok {
		return fmt.Errorf("project '%s' is not allowed", project)
	}
	return nil
}
```

## 4. 監査ログ

### internal/relay/audit.go

```go
package relay

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"sync"
	"time"
)

// AuditEvent は監査イベント
type AuditEvent struct {
	Timestamp  time.Time `json:"timestamp"`
	Action     string    `json:"action"`
	UserID     string    `json:"user_id,omitempty"`
	UserName   string    `json:"user_name,omitempty"`
	UserEmail  string    `json:"user_email,omitempty"`
	Space      string    `json:"space"`
	Domain     string    `json:"domain"`
	Project    string    `json:"project,omitempty"`
	ClientIP   string    `json:"client_ip"`
	UserAgent  string    `json:"user_agent"`
	Result     string    `json:"result"` // success, error
	Error      string    `json:"error,omitempty"`
}

// AuditAction は監査アクション
const (
	AuditActionAuthStart     = "auth_start"
	AuditActionAuthCallback  = "auth_callback"
	AuditActionTokenExchange = "token_exchange"
	AuditActionTokenRefresh  = "token_refresh"
	AuditActionAccessDenied  = "access_denied"
)

// AuditLogger は監査ログ出力
type AuditLogger struct {
	enabled    bool
	output     string
	filePath   string
	webhookURL string
	
	file   *os.File
	mu     sync.Mutex
	client *http.Client
}

// NewAuditLogger は新しい監査ロガーを作成する
func NewAuditLogger(cfg *config.AuditConfig) (*AuditLogger, error) {
	al := &AuditLogger{
		enabled:    cfg.Enabled,
		output:     cfg.Output,
		filePath:   cfg.FilePath,
		webhookURL: cfg.WebhookURL,
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
	
	if !cfg.Enabled {
		return al, nil
	}
	
	// ファイル出力の場合はファイルを開く
	if cfg.Output == "file" && cfg.FilePath != "" {
		f, err := os.OpenFile(cfg.FilePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			return nil, fmt.Errorf("failed to open audit log file: %w", err)
		}
		al.file = f
	}
	
	return al, nil
}

// Log は監査イベントを記録する
func (al *AuditLogger) Log(event AuditEvent) {
	if !al.enabled {
		return
	}
	
	event.Timestamp = time.Now().UTC()
	
	switch al.output {
	case "stdout":
		al.logToStdout(event)
	case "stderr":
		al.logToStderr(event)
	case "file":
		al.logToFile(event)
	case "webhook":
		go al.logToWebhook(event) // 非同期
	}
}

func (al *AuditLogger) logToStdout(event AuditEvent) {
	data, _ := json.Marshal(event)
	fmt.Println(string(data))
}

func (al *AuditLogger) logToStderr(event AuditEvent) {
	data, _ := json.Marshal(event)
	fmt.Fprintln(os.Stderr, string(data))
}

func (al *AuditLogger) logToFile(event AuditEvent) {
	if al.file == nil {
		return
	}
	
	al.mu.Lock()
	defer al.mu.Unlock()
	
	data, _ := json.Marshal(event)
	al.file.Write(data)
	al.file.WriteString("\n")
}

func (al *AuditLogger) logToWebhook(event AuditEvent) {
	if al.webhookURL == "" {
		return
	}
	
	// Slack形式のペイロード
	payload := al.buildSlackPayload(event)
	data, _ := json.Marshal(payload)
	
	resp, err := al.client.Post(al.webhookURL, "application/json", bytes.NewReader(data))
	if err != nil {
		log.Printf("Failed to send audit log to webhook: %v", err)
		return
	}
	defer resp.Body.Close()
	
	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		log.Printf("Webhook returned error: %d %s", resp.StatusCode, string(body))
	}
}

func (al *AuditLogger) buildSlackPayload(event AuditEvent) map[string]interface{} {
	color := "good"
	emoji := "🔐"
	if event.Result == "error" {
		color = "danger"
		emoji = "❌"
	}
	
	title := fmt.Sprintf("%s Backlog CLI: %s", emoji, event.Action)
	
	fields := []map[string]interface{}{
		{"title": "Space", "value": event.Space + "." + event.Domain, "short": true},
		{"title": "Result", "value": event.Result, "short": true},
	}
	
	if event.UserName != "" {
		fields = append(fields, map[string]interface{}{
			"title": "User",
			"value": event.UserName,
			"short": true,
		})
	}
	
	if event.Project != "" {
		fields = append(fields, map[string]interface{}{
			"title": "Project",
			"value": event.Project,
			"short": true,
		})
	}
	
	if event.ClientIP != "" {
		fields = append(fields, map[string]interface{}{
			"title": "IP",
			"value": event.ClientIP,
			"short": true,
		})
	}
	
	if event.Error != "" {
		fields = append(fields, map[string]interface{}{
			"title": "Error",
			"value": event.Error,
			"short": false,
		})
	}
	
	return map[string]interface{}{
		"text": title,
		"attachments": []map[string]interface{}{
			{
				"color":  color,
				"fields": fields,
				"ts":     event.Timestamp.Unix(),
			},
		},
	}
}

// Close はリソースを解放する
func (al *AuditLogger) Close() error {
	if al.file != nil {
		return al.file.Close()
	}
	return nil
}
```

## 5. Rate Limiting

### internal/relay/ratelimit.go

```go
package relay

import (
	"net/http"
	"sync"
	"time"
)

// RateLimiter はレートリミッター
type RateLimiter struct {
	enabled bool
	rate    int           // リクエスト/分
	burst   int           // バースト許容数
	
	mu      sync.Mutex
	clients map[string]*clientRate
}

type clientRate struct {
	tokens    float64
	lastCheck time.Time
}

// NewRateLimiter は新しいレートリミッターを作成する
func NewRateLimiter(enabled bool, requestsPerMinute, burst int) *RateLimiter {
	return &RateLimiter{
		enabled: enabled,
		rate:    requestsPerMinute,
		burst:   burst,
		clients: make(map[string]*clientRate),
	}
}

// Allow はリクエストを許可するか確認する
func (rl *RateLimiter) Allow(clientIP string) bool {
	if !rl.enabled {
		return true
	}
	
	rl.mu.Lock()
	defer rl.mu.Unlock()
	
	now := time.Now()
	cr, exists := rl.clients[clientIP]
	
	if !exists {
		rl.clients[clientIP] = &clientRate{
			tokens:    float64(rl.burst - 1),
			lastCheck: now,
		}
		return true
	}
	
	// トークンを補充
	elapsed := now.Sub(cr.lastCheck).Minutes()
	cr.tokens += elapsed * float64(rl.rate)
	if cr.tokens > float64(rl.burst) {
		cr.tokens = float64(rl.burst)
	}
	cr.lastCheck = now
	
	// トークンを消費
	if cr.tokens >= 1 {
		cr.tokens--
		return true
	}
	
	return false
}

// Middleware はレートリミットミドルウェアを返す
func (rl *RateLimiter) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !rl.enabled {
			next.ServeHTTP(w, r)
			return
		}
		
		ip := getClientIP(r)
		if ip == nil {
			next.ServeHTTP(w, r)
			return
		}
		
		if !rl.Allow(ip.String()) {
			w.Header().Set("Retry-After", "60")
			http.Error(w, "Too Many Requests", http.StatusTooManyRequests)
			return
		}
		
		next.ServeHTTP(w, r)
	})
}

// Cleanup は古いエントリを削除する（定期実行用）
func (rl *RateLimiter) Cleanup() {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	
	threshold := time.Now().Add(-10 * time.Minute)
	for ip, cr := range rl.clients {
		if cr.lastCheck.Before(threshold) {
			delete(rl.clients, ip)
		}
	}
}
```

## 6. サーバーへの統合

### internal/relay/server.go (修正)

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

type Server struct {
	cfg           *config.ServerConfig
	httpServer    *http.Server
	cookieSecret  []byte
	accessControl *AccessController
	ipRestriction *IPRestriction
	rateLimiter   *RateLimiter
	auditLogger   *AuditLogger
}

func NewServer(cfg *config.ServerConfig) (*Server, error) {
	if cfg.Cookie.Secret == "" {
		return nil, fmt.Errorf("cookie secret is required")
	}
	
	// IP制限
	ipRestriction, err := NewIPRestriction(cfg.Access.AllowedCIDRs)
	if err != nil {
		return nil, fmt.Errorf("invalid IP restriction config: %w", err)
	}
	
	// アクセス制御
	accessControl := NewAccessController(
		cfg.Access.AllowedSpaces,
		cfg.Access.AllowedProjects,
	)
	
	// レートリミッター
	rateLimiter := NewRateLimiter(
		cfg.RateLimit.Enabled,
		cfg.RateLimit.RequestsPerMinute,
		cfg.RateLimit.Burst,
	)
	
	// 監査ログ
	auditLogger, err := NewAuditLogger(&cfg.Audit)
	if err != nil {
		return nil, fmt.Errorf("failed to create audit logger: %w", err)
	}
	
	return &Server{
		cfg:           cfg,
		cookieSecret:  []byte(cfg.Cookie.Secret),
		accessControl: accessControl,
		ipRestriction: ipRestriction,
		rateLimiter:   rateLimiter,
		auditLogger:   auditLogger,
	}, nil
}

func (s *Server) Start() error {
	mux := http.NewServeMux()
	
	// エンドポイント登録
	mux.HandleFunc("GET /health", s.handleHealth)
	mux.HandleFunc("GET /.well-known/bl-relay", s.handleWellKnown)
	mux.HandleFunc("GET /auth/start", s.handleAuthStart)
	mux.HandleFunc("GET /auth/callback", s.handleAuthCallback)
	mux.HandleFunc("POST /auth/token", s.handleAuthToken)
	
	// ミドルウェアチェーン
	handler := Chain(
		mux,
		RecoveryMiddleware,
		LoggingMiddleware,
		s.ipRestriction.Middleware,
		s.rateLimiter.Middleware,
	)
	
	addr := fmt.Sprintf("%s:%d", s.cfg.Host, s.cfg.Port)
	
	s.httpServer = &http.Server{
		Addr:         addr,
		Handler:      handler,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}
	
	// レートリミッタークリーンアップ
	go func() {
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			s.rateLimiter.Cleanup()
		}
	}()
	
	log.Printf("Starting relay server on %s", addr)
	return s.httpServer.ListenAndServe()
}

func (s *Server) Shutdown(ctx context.Context) error {
	if s.auditLogger != nil {
		s.auditLogger.Close()
	}
	if s.httpServer != nil {
		return s.httpServer.Shutdown(ctx)
	}
	return nil
}
```

## 7. ハンドラーへの統合

### internal/relay/handlers.go (修正)

```go
func (s *Server) handleAuthStart(w http.ResponseWriter, r *http.Request) {
	domain := r.URL.Query().Get("domain")
	space := r.URL.Query().Get("space")
	portStr := r.URL.Query().Get("port")
	project := r.URL.Query().Get("project")
	
	clientIP := ""
	if ip := getClientIP(r); ip != nil {
		clientIP = ip.String()
	}
	
	// バリデーション
	if domain == "" || space == "" || portStr == "" {
		s.writeError(w, http.StatusBadRequest, "invalid_request", "domain, space, and port are required")
		return
	}
	
	// スペース制限チェック
	if err := s.accessControl.CheckSpace(space); err != nil {
		s.auditLogger.Log(AuditEvent{
			Action:    AuditActionAccessDenied,
			Space:     space,
			Domain:    domain,
			Project:   project,
			ClientIP:  clientIP,
			UserAgent: r.UserAgent(),
			Result:    "error",
			Error:     err.Error(),
		})
		s.writeError(w, http.StatusForbidden, "access_denied", err.Error())
		return
	}
	
	// プロジェクト制限チェック
	if err := s.accessControl.CheckProject(project); err != nil {
		s.auditLogger.Log(AuditEvent{
			Action:    AuditActionAccessDenied,
			Space:     space,
			Domain:    domain,
			Project:   project,
			ClientIP:  clientIP,
			UserAgent: r.UserAgent(),
			Result:    "error",
			Error:     err.Error(),
		})
		s.writeError(w, http.StatusForbidden, "access_denied", err.Error())
		return
	}
	
	// 監査ログ
	s.auditLogger.Log(AuditEvent{
		Action:    AuditActionAuthStart,
		Space:     space,
		Domain:    domain,
		Project:   project,
		ClientIP:  clientIP,
		UserAgent: r.UserAgent(),
		Result:    "success",
	})
	
	// ... 残りの処理
}

// handleAuthToken でユーザー情報を取得して監査ログに記録
func (s *Server) handleAuthToken(w http.ResponseWriter, r *http.Request) {
	// ... トークン取得処理 ...
	
	// トークン取得成功後、ユーザー情報を取得
	if req.GrantType == "authorization_code" {
		userInfo, err := s.fetchUserInfo(tokenResp.AccessToken, req.Space, req.Domain)
		
		s.auditLogger.Log(AuditEvent{
			Action:    AuditActionTokenExchange,
			UserID:    userInfo.UserID,
			UserName:  userInfo.Name,
			UserEmail: userInfo.MailAddress,
			Space:     req.Space,
			Domain:    req.Domain,
			ClientIP:  clientIP,
			UserAgent: r.UserAgent(),
			Result:    "success",
		})
	}
	
	// ...
}

func (s *Server) fetchUserInfo(accessToken, space, domain string) (*UserInfo, error) {
	url := fmt.Sprintf("https://%s.%s/api/v2/users/myself", space, domain)
	
	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("Authorization", "Bearer "+accessToken)
	
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	
	var user UserInfo
	json.NewDecoder(resp.Body).Decode(&user)
	return &user, nil
}

type UserInfo struct {
	ID          int    `json:"id"`
	UserID      string `json:"userId"`
	Name        string `json:"name"`
	MailAddress string `json:"mailAddress"`
}
```

## 8. 設定構造体の更新

### internal/config/config.go (追加)

```go
// RateLimitConfig はレートリミット設定
type RateLimitConfig struct {
	Enabled           bool `yaml:"enabled"`
	RequestsPerMinute int  `yaml:"requests_per_minute"`
	Burst             int  `yaml:"burst"`
}

// ServerConfig に追加
type ServerConfig struct {
	// ...
	RateLimit RateLimitConfig `yaml:"rate_limit"`
}
```

## 完了条件

- [ ] IP制限が動作する（設定したCIDRからのみアクセス可能）
- [ ] スペース制限が動作する
- [ ] プロジェクト制限が動作する
- [ ] 監査ログがstdoutに出力される
- [ ] 監査ログがファイルに出力される
- [ ] 監査ログがWebhook（Slack）に送信される
- [ ] Rate Limitingが動作する（制限超過で429を返す）
- [ ] ユーザー情報が監査ログに含まれる

## 次のステップ

`06-api-client.md` に進んでBacklog APIクライアントを実装してください。
