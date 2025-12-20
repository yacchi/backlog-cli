// Package backlog provides public APIs for the Backlog CLI.
//
// This package exposes minimal entry points for external use,
// such as Lambda handlers, while keeping implementation details in internal packages.
package backlog

import (
	"context"

	"github.com/yacchi/backlog-cli/internal/config"
)

// Config is the configuration store for Backlog CLI.
// It provides access to all configuration values with layer-based resolution.
type Config = config.Store

// LoadConfig loads the configuration from all available sources.
// Sources are resolved in the following priority order:
//   - Command line arguments (highest)
//   - Environment variables
//   - .backlog.yaml (project local)
//   - ~/.config/backlog/config.yaml (user config)
//   - Defaults (lowest)
func LoadConfig(ctx context.Context) (*Config, error) {
	return config.Load(ctx)
}
