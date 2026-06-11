package config

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"
	"github.com/yacchi/backlog-cli/packages/backlog/internal/config"
	"github.com/yacchi/backlog-cli/packages/backlog/internal/ui"
)

var (
	bundleName       string
	bundleExpiresIn  time.Duration
	bundleFiles      []string
	bundleOutputPath string
	bundleIncludeAll bool
)

var bundleCmd = &cobra.Command{
	Use:   "bundle",
	Short: "Create relay config bundles",
}

var bundleCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a relay config bundle from config and env",
	Args:  cobra.NoArgs,
	RunE:  runBundleCreate,
}

func init() {
	bundleCreateCmd.Flags().StringVar(&bundleName, "name", "", "Tenant name to create a bundle for (defaults to the sole tenant)")
	bundleCreateCmd.Flags().DurationVar(&bundleExpiresIn, "expires-in", 30*24*time.Hour, "Bundle expiry duration (e.g. 720h)")
	bundleCreateCmd.Flags().StringArrayVar(&bundleFiles, "file", nil, "Additional file to include (repeatable)")
	bundleCreateCmd.Flags().StringVar(&bundleOutputPath, "output", "", "Output bundle path (default: <name>.backlog-cli.zip)")
	bundleCreateCmd.Flags().BoolVar(&bundleIncludeAll, "include-all-keys", false, "Include all keys from JWKS in relay_keys")

	bundleCmd.AddCommand(bundleCreateCmd)
}

func runBundleCreate(cmd *cobra.Command, _ []string) error {
	cfg, err := config.Load(cmd.Context())
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	output, err := config.CreateRelayBundleFromConfig(cmd.Context(), cfg, config.BundleCreateOptions{
		Name:       bundleName,
		ExpiresIn:  bundleExpiresIn,
		Files:      bundleFiles,
		OutputPath: bundleOutputPath,
		IncludeAll: bundleIncludeAll,
	})
	if err != nil {
		return fmt.Errorf("failed to create bundle: %w", err)
	}

	ui.Success("Created relay bundle: %s", output)
	return nil
}
