package config

import (
	"context"
	"fmt"
	"syscall"

	"github.com/spf13/cobra"
	"github.com/yacchi/backlog-cli/internal/config"
	"github.com/yacchi/backlog-cli/internal/ui"
	"golang.org/x/term"
)

var (
	assumeYes  bool
	noDefaults bool
)

var importCmd = &cobra.Command{
	Use:   "import <bundle.zip>",
	Short: "Import a relay config bundle",
	Long: `Import a relay config bundle.

Examples:
  backlog config import bundle.zip
  backlog config import --yes bundle.zip
  backlog config import --no-defaults bundle.zip`,
	Args: cobra.ExactArgs(1),
	RunE: runImport,
}

func init() {
	importCmd.Flags().BoolVarP(&assumeYes, "yes", "y", false, "Skip confirmation and import immediately")
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

	var approvalHandler config.BundleApprovalHandler
	if !assumeYes && term.IsTerminal(int(syscall.Stdin)) {
		approvalHandler = func(_ context.Context, info config.BundleApprovalInfo) (bool, error) {
			fmt.Println("Bundle information:")
			fmt.Printf("  Allowed domain: %s\n", info.AllowedDomain)
			fmt.Printf("  Relay URL:      %s\n", info.RelayURL)
			fmt.Printf("  Keys:           %d key(s)\n", info.RelayKeyCount)
			fmt.Printf("  Issued at:      %s\n", info.IssuedAt)
			fmt.Printf("  Expires at:     %s\n", info.ExpiresAt)
			fmt.Printf("  File name:      %s\n", info.FileName)
			fmt.Printf("  SHA256:         %s\n", info.SHA256)
			fmt.Println()
			return ui.Confirm("Import this bundle?", false)
		}
	}

	imported, err := config.ImportRelayBundle(cmd.Context(), cfg, bundlePath, config.BundleImportOptions{
		ApprovalHandler: approvalHandler,
		NoDefaults:      noDefaults,
		CacheDir:        cacheDir,
	})
	if err != nil {
		return fmt.Errorf("failed to import bundle: %w", err)
	}

	if err := cfg.Save(cmd.Context()); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	ui.Success("Imported relay bundle for %s", imported.AllowedDomain)
	fmt.Printf("  Relay URL:   %s\n", imported.RelayURL)
	fmt.Printf("  Keys:        %d key(s)\n", len(imported.RelayKeys))
	fmt.Printf("  Expires at:  %s\n", imported.ExpiresAt)
	fmt.Printf("  Imported at: %s\n", imported.ImportedAt)
	return nil
}
