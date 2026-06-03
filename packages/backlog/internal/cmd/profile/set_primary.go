package profile

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/yacchi/backlog-cli/packages/backlog/internal/config"
)

var setPrimaryCmd = &cobra.Command{
	Use:   "set-primary",
	Short: "Set primary profile for a space",
	Long: `Set a profile as the primary for its space.

When multiple profiles share the same space, --space resolves to the primary profile.
Only one profile per space can be primary.

Examples:
  backlog profile set-primary --profile default
  backlog profile set-primary --profile readonly`,
	RunE: runSetPrimary,
}

func runSetPrimary(cmd *cobra.Command, args []string) error {
	profileName, _ := cmd.Flags().GetString("profile")
	if profileName == "" {
		return fmt.Errorf("--profile is required")
	}

	cfg, err := config.Load(cmd.Context())
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	profiles := cfg.Profiles()

	target, ok := profiles[profileName]
	if !ok {
		return fmt.Errorf("profile %q not found", profileName)
	}

	if target.Space == "" || target.Domain == "" {
		return fmt.Errorf("profile %q has no space/domain configured", profileName)
	}

	targetSpace := target.Space
	targetDomain := target.Domain

	// 同一スペース内の他プロファイルから primary を除去
	for name, p := range profiles {
		if name == profileName {
			continue
		}
		if p.Space == targetSpace && p.Domain == targetDomain && p.Primary {
			if err := cfg.SetProfileValue(config.LayerUser, name, "primary", false); err != nil {
				return fmt.Errorf("failed to clear primary from profile %q: %w", name, err)
			}
		}
	}

	// 対象プロファイルに primary を設定
	if err := cfg.SetProfileValue(config.LayerUser, profileName, "primary", true); err != nil {
		return fmt.Errorf("failed to set primary on profile %q: %w", profileName, err)
	}

	if err := cfg.Save(cmd.Context()); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	fmt.Printf("Set %q as primary profile for %s.%s\n", profileName, targetSpace, targetDomain)
	return nil
}
