package relay

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/yacchi/backlog-cli/internal/config"
)

// Server は中継サーバー
type Server struct {
	cfg           *config.Store
	httpServer    *http.Server
	accessControl *AccessController
	ipRestriction *IPRestriction
	rateLimiter   *RateLimiter
	auditLogger   *AuditLogger
	bundleAuth    *BundleAuthMiddleware
}

// NewServer は新しいサーバーを作成する
func NewServer(cfg *config.Store) (*Server, error) {
	server := cfg.Server()

	// IP制限
	ipRestriction, err := NewIPRestriction(server.AllowedCIDRs)
	if err != nil {
		return nil, fmt.Errorf("invalid IP restriction config: %w", err)
	}

	// アクセス制御
	accessControl := NewAccessController(
		server.AllowedSpaces,
		server.AllowedProjects,
	)

	// レートリミッター
	rateLimiter := NewRateLimiter(
		server.RateLimitEnabled,
		server.RateLimitRequestsPerMinute,
		server.RateLimitBurst,
	)

	// 監査ログ
	auditLogger, err := NewAuditLogger(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create audit logger: %w", err)
	}

	// バンドル認証ミドルウェア
	bundleAuth := NewBundleAuthMiddleware(cfg, auditLogger)

	return &Server{
		cfg:           cfg,
		accessControl: accessControl,
		ipRestriction: ipRestriction,
		rateLimiter:   rateLimiter,
		auditLogger:   auditLogger,
		bundleAuth:    bundleAuth,
	}, nil
}

// Handler はHTTPハンドラーを返す（Lambda等のサーバーレス環境用）
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	// エンドポイント登録
	mux.HandleFunc("GET /health", s.handleHealth)
	mux.HandleFunc("GET /.well-known/backlog-oauth-relay", s.handleWellKnown)
	mux.HandleFunc("GET /auth/start", s.handleAuthStart)
	mux.HandleFunc("GET /auth/callback", s.handleAuthCallback)
	mux.HandleFunc("POST /auth/token", s.handleAuthToken)
	mux.HandleFunc("GET /v1/relay/tenants/{domain}/info", s.handleRelayInfo)
	mux.HandleFunc("GET /v1/relay/tenants/{domain}/certs", s.handleRelayCerts)
	mux.HandleFunc("GET /v1/relay/tenants/{domain}/bundle", s.handleRelayBundle)

	// ポータルエンドポイント
	mux.HandleFunc("POST /api/v1/portal/verify", s.handlePortalVerify)
	mux.HandleFunc("GET /api/v1/portal/{domain}/bundle", s.handlePortalBundle)
	mux.HandleFunc("GET /portal/{domain}", s.handlePortalSPA)
	mux.HandleFunc("GET /portal/{domain}/", s.handlePortalSPA)

	// 静的アセット（ポータルSPA用）
	mux.HandleFunc("GET /assets/", s.handleStaticAssets)

	// ミドルウェアチェーン
	// Note: Lambda環境ではレートリミッターは無効化される（インメモリのため）
	return Chain(
		mux,
		RecoveryMiddleware,
		LoggingMiddleware,
		s.ipRestriction.Middleware,
		s.rateLimiter.Middleware,
		s.bundleAuth.Middleware,
	)
}

// Start はサーバーを起動する
func (s *Server) Start() error {
	handler := s.Handler()

	server := s.cfg.Server()
	addr := fmt.Sprintf("%s:%d", server.Host, server.Port)

	s.httpServer = &http.Server{
		Addr:         addr,
		Handler:      handler,
		ReadTimeout:  time.Duration(server.HTTPReadTimeout) * time.Second,
		WriteTimeout: time.Duration(server.HTTPWriteTimeout) * time.Second,
		IdleTimeout:  time.Duration(server.HTTPIdleTimeout) * time.Second,
	}

	// レートリミッタークリーンアップ
	go func() {
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			s.rateLimiter.Cleanup()
		}
	}()

	slog.Info("starting relay server", "addr", addr)
	return s.httpServer.ListenAndServe()
}

// Shutdown はサーバーを停止する
func (s *Server) Shutdown(ctx context.Context) error {
	if s.auditLogger != nil {
		_ = s.auditLogger.Close()
	}
	if s.httpServer != nil {
		return s.httpServer.Shutdown(ctx)
	}
	return nil
}

// handleHealth はヘルスチェック
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("OK"))
}
