package relay

import (
	"net/http"
	"strings"

	"github.com/yacchi/backlog-cli/internal/config"
)

// BundleAuthMiddleware はbundle_tokenによる認証ミドルウェア
type BundleAuthMiddleware struct {
	cfg         *config.Store
	auditLogger *AuditLogger
}

// NewBundleAuthMiddleware は新しいBundleAuthMiddlewareを作成する
func NewBundleAuthMiddleware(cfg *config.Store, auditLogger *AuditLogger) *BundleAuthMiddleware {
	return &BundleAuthMiddleware{
		cfg:         cfg,
		auditLogger: auditLogger,
	}
}

// AuditAction constants for bundle auth
const (
	AuditActionBundleAuth = "bundle_auth"
)

// Middleware はHTTPミドルウェアを返す
func (m *BundleAuthMiddleware) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// /v1/relay/tenants/{domain}/... のパスからdomainを抽出
		path := r.URL.Path
		if !strings.HasPrefix(path, "/v1/relay/tenants/") {
			next.ServeHTTP(w, r)
			return
		}

		// パスからドメインを抽出
		remaining := strings.TrimPrefix(path, "/v1/relay/tenants/")
		parts := strings.SplitN(remaining, "/", 2)
		if len(parts) == 0 || parts[0] == "" {
			http.Error(w, "invalid tenant path", http.StatusBadRequest)
			return
		}
		domain := parts[0]

		// certsエンドポイントは認証不要（公開鍵の配布用）
		// infoエンドポイントも認証不要（機密情報を含まず、CloudFrontキャッシュ可能にするため）
		if len(parts) == 2 && (parts[1] == "certs" || parts[1] == "info") {
			next.ServeHTTP(w, r)
			return
		}

		// テナント設定を取得
		tenant := m.findTenantByDomain(domain)
		if tenant == nil {
			m.logAuthFailure(r, domain, "tenant not found")
			http.Error(w, "tenant not found", http.StatusNotFound)
			return
		}

		// Authorization ヘッダーからトークンを取得
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			m.logAuthFailure(r, domain, "missing authorization header")
			http.Error(w, "authorization required", http.StatusUnauthorized)
			return
		}

		// Bearer トークンを抽出
		if !strings.HasPrefix(authHeader, "Bearer ") {
			m.logAuthFailure(r, domain, "invalid authorization format")
			http.Error(w, "invalid authorization format", http.StatusUnauthorized)
			return
		}
		token := strings.TrimPrefix(authHeader, "Bearer ")

		// トークン検証
		if err := config.VerifyBundleTokenWithTenant(token, domain, tenant); err != nil {
			m.logAuthFailure(r, domain, "token verification failed")
			http.Error(w, "invalid token", http.StatusUnauthorized)
			return
		}

		// 認証成功
		next.ServeHTTP(w, r)
	})
}

// findTenantByDomain はドメインからテナント設定を検索する
func (m *BundleAuthMiddleware) findTenantByDomain(domain string) *config.ResolvedTenant {
	tenants := m.cfg.Server().Tenants
	for _, tenant := range tenants {
		if tenant.AllowedDomain == domain {
			return &tenant
		}
	}
	return nil
}

// logAuthFailure は認証失敗をログに記録する
func (m *BundleAuthMiddleware) logAuthFailure(r *http.Request, domain, reason string) {
	clientIP := ""
	if ip := getClientIP(r); ip != nil {
		clientIP = ip.String()
	}

	m.auditLogger.Log(AuditEvent{
		Action:    AuditActionBundleAuth,
		Domain:    domain,
		ClientIP:  clientIP,
		UserAgent: r.UserAgent(),
		Result:    "error",
		Error:     reason,
	})
}
