package relay

import (
	"encoding/json"
	"net/http"
	"strings"

	"golang.org/x/crypto/bcrypt"

	"github.com/yacchi/backlog-cli/internal/config"
	"github.com/yacchi/backlog-cli/internal/domain"
	"github.com/yacchi/backlog-cli/internal/ui"
)

// PortalVerifyRequest はパスフレーズ検証リクエスト
type PortalVerifyRequest struct {
	Domain     string `json:"domain"`
	Passphrase string `json:"passphrase"`
}

// PortalVerifyResponse はパスフレーズ検証レスポンス
type PortalVerifyResponse struct {
	Success       bool   `json:"success"`
	Domain        string `json:"domain,omitempty"`
	RelayURL      string `json:"relay_url,omitempty"`
	Space         string `json:"space,omitempty"`
	BacklogDomain string `json:"backlog_domain,omitempty"`
	Error         string `json:"error,omitempty"`
}

// AuditAction constants for portal
const (
	AuditActionPortalVerify   = "portal_verify"
	AuditActionPortalDownload = "portal_download"
)

// handlePortalSPA はポータルSPAを配信する
func (s *Server) handlePortalSPA(w http.ResponseWriter, r *http.Request) {
	assets, err := ui.Assets()
	if err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	ui.SPAHandler(assets).ServeHTTP(w, r)
}

// handleStaticAssets は静的アセットを配信する
func (s *Server) handleStaticAssets(w http.ResponseWriter, r *http.Request) {
	assets, err := ui.Assets()
	if err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	// /assets/xxx.js -> assets/xxx.js
	path := strings.TrimPrefix(r.URL.Path, "/")
	http.ServeFileFS(w, r, assets, path)
}

// handlePortalVerify はパスフレーズ検証を行う
func (s *Server) handlePortalVerify(w http.ResponseWriter, r *http.Request) {
	clientIP := ""
	if ip := getClientIP(r); ip != nil {
		clientIP = ip.String()
	}

	var req PortalVerifyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writePortalError(w, http.StatusBadRequest, "invalid_request: "+err.Error())
		return
	}

	if req.Domain == "" || req.Passphrase == "" {
		s.writePortalError(w, http.StatusBadRequest, "domain and passphrase are required")
		return
	}

	tenant := s.findTenantByDomain(req.Domain)
	if tenant == nil {
		s.auditLogger.Log(AuditEvent{
			Action:    AuditActionPortalVerify,
			Domain:    req.Domain,
			ClientIP:  clientIP,
			UserAgent: r.UserAgent(),
			Result:    "error",
			Error:     "tenant not found",
		})
		s.writePortalError(w, http.StatusNotFound, "tenant not found")
		return
	}

	switch s.verifyPassphrase(tenant, req.Passphrase) {
	case PassphraseNotConfigured:
		s.auditLogger.Log(AuditEvent{
			Action:    AuditActionPortalVerify,
			Domain:    req.Domain,
			ClientIP:  clientIP,
			UserAgent: r.UserAgent(),
			Result:    "error",
			Error:     "portal not enabled",
		})
		s.writePortalError(w, http.StatusForbidden, "portal_not_enabled")
		return
	case PassphraseInvalid:
		s.auditLogger.Log(AuditEvent{
			Action:    AuditActionPortalVerify,
			Domain:    req.Domain,
			ClientIP:  clientIP,
			UserAgent: r.UserAgent(),
			Result:    "error",
			Error:     "invalid passphrase",
		})
		s.writePortalError(w, http.StatusUnauthorized, "invalid_passphrase")
		return
	}

	space, backlogDomain := domain.SplitDomain(req.Domain)
	relayURL := s.buildRelayURL(r)

	s.auditLogger.Log(AuditEvent{
		Action:    AuditActionPortalVerify,
		Space:     space,
		Domain:    backlogDomain,
		ClientIP:  clientIP,
		UserAgent: r.UserAgent(),
		Result:    "success",
	})

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(PortalVerifyResponse{
		Success:       true,
		Domain:        req.Domain,
		RelayURL:      relayURL,
		Space:         space,
		BacklogDomain: backlogDomain,
	})
}

// handlePortalBundle はバンドルをダウンロードさせる
func (s *Server) handlePortalBundle(w http.ResponseWriter, r *http.Request) {
	clientIP := ""
	if ip := getClientIP(r); ip != nil {
		clientIP = ip.String()
	}

	allowedDomain := r.PathValue("domain")
	var req PortalVerifyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writePortalError(w, http.StatusBadRequest, "invalid_request: "+err.Error())
		return
	}
	passphrase := req.Passphrase

	if allowedDomain == "" || passphrase == "" {
		s.writePortalError(w, http.StatusBadRequest, "domain and passphrase are required")
		return
	}

	tenant := s.findTenantByDomain(allowedDomain)
	if tenant == nil {
		s.auditLogger.Log(AuditEvent{
			Action:    AuditActionPortalDownload,
			Domain:    allowedDomain,
			ClientIP:  clientIP,
			UserAgent: r.UserAgent(),
			Result:    "error",
			Error:     "tenant not found",
		})
		s.writePortalError(w, http.StatusNotFound, "tenant not found")
		return
	}

	switch s.verifyPassphrase(tenant, passphrase) {
	case PassphraseNotConfigured:
		s.auditLogger.Log(AuditEvent{
			Action:    AuditActionPortalDownload,
			Domain:    allowedDomain,
			ClientIP:  clientIP,
			UserAgent: r.UserAgent(),
			Result:    "error",
			Error:     "portal not enabled",
		})
		s.writePortalError(w, http.StatusForbidden, "portal_not_enabled")
		return
	case PassphraseInvalid:
		s.auditLogger.Log(AuditEvent{
			Action:    AuditActionPortalDownload,
			Domain:    allowedDomain,
			ClientIP:  clientIP,
			UserAgent: r.UserAgent(),
			Result:    "error",
			Error:     "invalid passphrase",
		})
		s.writePortalError(w, http.StatusUnauthorized, "invalid_passphrase")
		return
	}

	relayURL := s.buildRelayURL(r)
	bundleData, err := config.CreatePortalBundle(tenant, allowedDomain, relayURL)
	if err != nil {
		s.auditLogger.Log(AuditEvent{
			Action:    AuditActionPortalDownload,
			Domain:    allowedDomain,
			ClientIP:  clientIP,
			UserAgent: r.UserAgent(),
			Result:    "error",
			Error:     err.Error(),
		})
		s.writePortalError(w, http.StatusInternalServerError, "failed to create bundle")
		return
	}

	space, backlogDomain := domain.SplitDomain(allowedDomain)
	s.auditLogger.Log(AuditEvent{
		Action:    AuditActionPortalDownload,
		Space:     space,
		Domain:    backlogDomain,
		ClientIP:  clientIP,
		UserAgent: r.UserAgent(),
		Result:    "success",
	})

	filename := allowedDomain + ".backlog-cli.zip"
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", "attachment; filename=\""+filename+"\"")
	_, _ = w.Write(bundleData)
}

// findTenantByDomain はドメインからテナント設定を検索する
func (s *Server) findTenantByDomain(domain string) *config.ResolvedTenant {
	tenants := s.cfg.Server().Tenants
	for _, tenant := range tenants {
		if tenant.AllowedDomain == domain {
			return &tenant
		}
	}
	return nil
}

// PassphraseVerifyResult はパスフレーズ検証の結果
type PassphraseVerifyResult int

const (
	PassphraseValid PassphraseVerifyResult = iota
	PassphraseInvalid
	PassphraseNotConfigured
)

// verifyPassphrase はパスフレーズを検証する
func (s *Server) verifyPassphrase(tenant *config.ResolvedTenant, passphrase string) PassphraseVerifyResult {
	if tenant.PassphraseHash == "" {
		return PassphraseNotConfigured
	}
	err := bcrypt.CompareHashAndPassword(
		[]byte(tenant.PassphraseHash),
		[]byte(passphrase),
	)
	if err != nil {
		return PassphraseInvalid
	}
	return PassphraseValid
}

// buildRelayURL はリクエストからRelay URLを構築する
func (s *Server) buildRelayURL(r *http.Request) string {
	server := s.cfg.Server()
	if server.BaseURL != "" {
		return server.BaseURL
	}

	scheme := "https"
	host := r.Host

	if proto := r.Header.Get("X-Forwarded-Proto"); proto != "" {
		scheme = proto
	}
	// X-Original-Host を優先（CloudFront Function で設定）
	if origHost := r.Header.Get("X-Original-Host"); origHost != "" {
		host = origHost
	} else if fwdHost := r.Header.Get("X-Forwarded-Host"); fwdHost != "" {
		host = fwdHost
	}

	return scheme + "://" + host
}

// writePortalError はポータルエラーレスポンスを書き込む
func (s *Server) writePortalError(w http.ResponseWriter, status int, errMsg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(PortalVerifyResponse{
		Success: false,
		Error:   errMsg,
	})
}
