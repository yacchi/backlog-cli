package config

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/yacchi/backlog-cli/internal/config"
	"github.com/yacchi/backlog-cli/internal/ui"
)

var (
	allowNameMismatch bool
	noDefaults        bool
)

var importCmd = &cobra.Command{
	Use:   "import <bundle.zip>",
	Short: "Import a relay config bundle",
	Long: `Import a relay config bundle.

Examples:
  backlog config import bundle.zip
  backlog config import --allow-name-mismatch bundle.zip
  backlog config import --no-defaults bundle.zip`,
	Args: cobra.ExactArgs(1),
	RunE: runImport,
}

func init() {
	importCmd.Flags().BoolVar(&allowNameMismatch, "allow-name-mismatch", false, "Allow bundle filename mismatch")
	importCmd.Flags().BoolVar(&noDefaults, "no-defaults", false, "Do not update default profile values")
}

func runImport(cmd *cobra.Command, args []string) error {
	bundlePath := args[0]

	cfg, err := config.Load(cmd.Context())
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// キャッシュディレクトリを取得（certsキャッシュ用）
	cacheDir, err := cfg.GetCacheDir()
	if err != nil {
		return fmt.Errorf("failed to get cache directory: %w", err)
	}

	imported, err := config.ImportRelayBundle(cmd.Context(), cfg, bundlePath, config.BundleImportOptions{
		AllowNameMismatch: allowNameMismatch,
		NoDefaults:        noDefaults,
		CacheDir:          cacheDir,
		CertsCacheTTL:     cfg.Resolved().Cache.CertsTTL,
	})
	if err != nil {
		return fmt.Errorf("failed to import bundle: %w", err)
	}

	if err := cfg.Save(cmd.Context()); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	ui.Success("Imported relay bundle for %s", imported.AllowedDomain)
	return nil
}
