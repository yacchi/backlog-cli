package auth

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/yacchi/backlog-cli/internal/auth"
	"github.com/yacchi/backlog-cli/internal/config"
)

var setupCmd = &cobra.Command{
	Use:   "setup <relay-server-url>",
	Short: "Configure relay server",
	Long: `Configure the OAuth relay server URL.

The relay server handles OAuth authentication, keeping the client_id
and client_secret secure on the server side.

After setup, run 'backlog auth login' to authenticate and configure
your Backlog space.

Example:
  backlog auth setup https://relay.example.com
  backlog --profile work auth setup https://relay.example.com`,
	Args: cobra.ExactArgs(1),
	RunE: runSetup,
}

func runSetup(cmd *cobra.Command, args []string) error {
	relayServer := args[0]

	// well-known を取得して確認
	fmt.Printf("Fetching relay server information from %s...\n", relayServer)

	client := auth.NewClient(relayServer)
	meta, err := client.FetchWellKnown()
	if err != nil {
		return fmt.Errorf("failed to connect to relay server: %w", err)
	}

	fmt.Println()
	fmt.Printf("Relay server: %s\n", relayServer)
	if meta.Name != "" {
		fmt.Printf("Name: %s\n", meta.Name)
	}
	fmt.Printf("Supported domains: %v\n", meta.SupportedDomains)

	if len(meta.SupportedDomains) == 0 {
		return fmt.Errorf("relay server has no supported domains configured")
	}

	// 設定に保存
	ctx := cmd.Context()
	cfg, err := config.Load(ctx)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// 指定されたプロファイルに書き込み（未指定の場合はdefault）
	profileName, _ := cmd.Flags().GetString("profile")
	if profileName == "" {
		profileName = config.DefaultProfile
	}
	cfg.SetProfileValue(config.LayerUser, profileName, "relay_server", relayServer)

	if err := cfg.Reload(ctx); err != nil {
		return fmt.Errorf("failed to reload config: %w", err)
	}
	if err := cfg.Save(ctx); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	fmt.Println()
	if profileName != config.DefaultProfile {
		fmt.Printf("Configuration saved to profile '%s'\n", profileName)
	} else {
		fmt.Println("Configuration saved")
	}
	fmt.Println()
	if profileName != config.DefaultProfile {
		fmt.Printf("Run 'backlog auth login --profile %s' to authenticate\n", profileName)
	} else {
		fmt.Println("Run 'backlog auth login' to authenticate")
	}

	return nil
}
