package relay

import (
	"net/http"
	"strings"

	"github.com/yacchi/backlog-cli/internal/config"
)

// AuditAction constants for bundle
const (
	AuditActionRelayBundle = "relay_bundle"
)

// handleRelayBundle は自動更新用のバンドルを返す
func (s *Server) handleRelayBundle(w http.ResponseWriter, r *http.Request) {
	domain := strings.TrimSpace(r.PathValue("domain"))
	if domain == "" {
		http.Error(w, "domain is required", http.StatusBadRequest)
		return
	}

	clientIP := ""
	if ip := getClientIP(r); ip != nil {
		clientIP = ip.String()
	}

	tenant, ok := findTenantByAllowedDomain(s.cfg.Server().Tenants, domain)
	if !ok {
		s.auditLogger.Log(AuditEvent{
			Action:    AuditActionRelayBundle,
			Domain:    domain,
			ClientIP:  clientIP,
			UserAgent: r.UserAgent(),
			Result:    "error",
			Error:     "tenant not found",
		})
		http.Error(w, "tenant not found", http.StatusNotFound)
		return
	}

	relayURL := s.buildRelayURL(r)
	bundleData, err := config.CreatePortalBundle(tenant, domain, relayURL)
	if err != nil {
		s.auditLogger.Log(AuditEvent{
			Action:    AuditActionRelayBundle,
			Domain:    domain,
			ClientIP:  clientIP,
			UserAgent: r.UserAgent(),
			Result:    "error",
			Error:     err.Error(),
		})
		http.Error(w, "failed to create bundle", http.StatusInternalServerError)
		return
	}

	space, backlogDomain := splitDomain(domain)
	s.auditLogger.Log(AuditEvent{
		Action:    AuditActionRelayBundle,
		Space:     space,
		Domain:    backlogDomain,
		ClientIP:  clientIP,
		UserAgent: r.UserAgent(),
		Result:    "success",
	})

	filename := domain + ".backlog-cli.zip"
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", "attachment; filename=\""+filename+"\"")
	_, _ = w.Write(bundleData)
}
