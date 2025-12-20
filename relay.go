package backlog

import (
	"net/http"

	"github.com/yacchi/backlog-cli/internal/relay"
)

// AuthRelayServer is the OAuth relay server for Backlog CLI.
// It handles the OAuth 2.0 authorization code flow without exposing
// client secrets to the CLI application.
type AuthRelayServer = relay.Server

// NewAuthRelayServer creates a new OAuth relay server with the given configuration.
// The server provides HTTP handlers for:
//   - GET /health - Health check endpoint
//   - GET /.well-known/backlog-oauth-relay - Server discovery
//   - GET /auth/start - Start OAuth authorization flow
//   - GET /auth/callback - OAuth callback from Backlog
//   - POST /auth/token - Token exchange and refresh
func NewAuthRelayServer(cfg *Config) (*AuthRelayServer, error) {
	return relay.NewServer(cfg)
}

// AuthRelayHandler creates an HTTP handler for the OAuth relay server.
// This is a convenience function for serverless environments like AWS Lambda.
func AuthRelayHandler(cfg *Config) (http.Handler, error) {
	server, err := NewAuthRelayServer(cfg)
	if err != nil {
		return nil, err
	}
	return server.Handler(), nil
}
