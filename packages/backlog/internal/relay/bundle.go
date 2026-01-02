package relay

import (
	"net/http"
	"strings"

	"github.com/yacchi/backlog-cli/packages/backlog/internal/config"
	"github.com/yacchi/backlog-cli/packages/backlog/internal/domain"
)

// AuditAction constants for bundle
const (
	AuditActionRelayBundle = "relay_bundle"
)

// handleRelayBundle は自動更新用のバンドルを返す
func (s *Server) handleRelayBundle(w http.ResponseWriter, r *http.Request) {
	allowedDomain := strings.TrimSpace(r.PathValue("domain"))
	if allowedDomain == "" {
		http.Error(w, "domain is required", http.StatusBadRequest)
		return
	}

	reqCtx := ExtractRequestContext(r)

	tenant, ok := findTenantByAllowedDomain(s.cfg.Server().Tenants, allowedDomain)
	if !ok {
		s.auditLogger.Log(AuditEvent{
			Action:    AuditActionRelayBundle,
			Domain:    allowedDomain,
			ClientIP:  reqCtx.ClientIP,
			UserAgent: reqCtx.UserAgent,
			Result:    "error",
			Error:     "tenant not found",
		})
		http.Error(w, "tenant not found", http.StatusNotFound)
		return
	}

	relayURL := s.buildRelayURL(reqCtx)
	bundleData, err := config.CreatePortalBundle(tenant, allowedDomain, relayURL)
	if err != nil {
		s.auditLogger.Log(AuditEvent{
			Action:    AuditActionRelayBundle,
			Domain:    allowedDomain,
			ClientIP:  reqCtx.ClientIP,
			UserAgent: reqCtx.UserAgent,
			Result:    "error",
			Error:     err.Error(),
		})
		http.Error(w, "failed to create bundle", http.StatusInternalServerError)
		return
	}

	space, backlogDomain := domain.SplitDomain(allowedDomain)
	s.auditLogger.Log(AuditEvent{
		Action:    AuditActionRelayBundle,
		Space:     space,
		Domain:    backlogDomain,
		ClientIP:  reqCtx.ClientIP,
		UserAgent: reqCtx.UserAgent,
		Result:    "success",
	})

	filename := allowedDomain + ".backlog-cli.zip"
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", "attachment; filename=\""+filename+"\"")
	SetCacheHeaders(w, CacheTypeNone, s.cfg)
	_, _ = w.Write(bundleData)
}
