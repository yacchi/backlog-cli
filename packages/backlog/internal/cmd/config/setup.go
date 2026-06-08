package config

import (
	"fmt"
	"syscall"

	"github.com/spf13/cobra"
	"github.com/yacchi/backlog-cli/packages/backlog/internal/cmdutil"
	"github.com/yacchi/backlog-cli/packages/backlog/internal/config"
	"github.com/yacchi/backlog-cli/packages/backlog/internal/ui"
	"golang.org/x/term"
)

var setupCmd = &cobra.Command{
	Use:   "setup <provisioning-key>",
	Short: "Set up CLI using a provisioning key",
	Long: `Set up the Backlog CLI using a provisioning key obtained from the portal.

The provisioning key contains all the information needed to automatically
download and import a relay config bundle.

Examples:
  backlog config setup eyJhbGci...
  backlog config setup --no-defaults eyJhbGci...`,
	Args: cobra.ExactArgs(1),
	RunE: runSetup,
}

func init() {
	setupCmd.Flags().BoolVar(&noDefaults, "no-defaults", false, "Do not update default profile values")
}

func runSetup(cmd *cobra.Command, args []string) error {
	token := args[0]

	claims, err := config.DecodeProvisioningToken(token)
	if err != nil {
		return fmt.Errorf("invalid provisioning key: %w", err)
	}

	fmt.Println("Provisioning key information:")
	fmt.Printf("  Space:        %s\n", claims.Space)
	fmt.Printf("  Domain:       %s\n", claims.Domain)
	fmt.Printf("  Relay server: %s\n", claims.RelayURL)
	fmt.Println()

	if !cmdutil.SkipConfirmation(cmd) && term.IsTerminal(int(syscall.Stdin)) {
		approved, err := ui.Confirm("Import configuration from this relay server?", false)
		if err != nil {
			return err
		}
		if !approved {
			return fmt.Errorf("setup cancelled")
		}
		fmt.Println()
	}

	cfg, err := config.Load(cmd.Context())
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	ui.Info("Downloading and importing config bundle...")

	imported, err := config.ProvisionFromToken(cmd.Context(), cfg, token, config.ProvisionOptions{
		NoDefaults: noDefaults,
	})
	if err != nil {
		return fmt.Errorf("provisioning failed: %w", err)
	}

	if err := cfg.Save(cmd.Context()); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	ui.Success("Setup complete for %s", imported.AllowedDomain)
	fmt.Printf("  Relay URL:   %s\n", imported.RelayURL)
	fmt.Printf("  Keys:        %d key(s)\n", len(imported.RelayKeys))
	fmt.Printf("  Expires at:  %s\n", imported.ExpiresAt)
	fmt.Println()
	fmt.Println("You can now authenticate with:")
	fmt.Printf("  backlog auth login\n")
	return nil
}
