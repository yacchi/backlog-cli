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
	server := s.cfg.Server()
	domains := make([]string, 0, len(server.Backlog))
	for _, b := range server.Backlog {
		if b.ClientID() != "" && b.ClientSecret() != "" {
			domains = append(domains, b.Domain())
		}
	}

	resp := WellKnownResponse{
		Version:          "1",
		SupportedDomains: domains,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}
