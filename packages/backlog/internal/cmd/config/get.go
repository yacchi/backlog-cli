package config

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/yacchi/backlog-cli/packages/backlog/internal/config"
)

var getCmd = &cobra.Command{
	Use:   "get <key>",
	Short: "Get a configuration value",
	Long: `Get a configuration value.

Examples:
  backlog config get client.default.space
  backlog config get client.default.project
  backlog config get display.default_issue_limit`,
	Args: cobra.ExactArgs(1),
	RunE: runGet,
}

func runGet(c *cobra.Command, args []string) error {
	key := args[0]

	cfg, err := config.Load(c.Context())
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	value := cfg.Get(key)
	if value == nil {
		return fmt.Errorf("unknown config key: %s", key)
	}

	fmt.Println(value)
	return nil
}
