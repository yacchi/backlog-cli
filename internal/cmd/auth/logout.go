package auth

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/yacchi/backlog-cli/internal/config"
	"github.com/yacchi/backlog-cli/internal/ui"
)

var logoutAll bool

func init() {
	logoutCmd.Flags().BoolVar(&logoutAll, "all", false, "Log out from all accounts")
}

var logoutCmd = &cobra.Command{
	Use:   "logout",
	Short: "Log out from Backlog",
	Long:  "Remove stored authentication credentials.",
	RunE:  runLogout,
}

func runLogout(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load(cmd.Context())
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	resolved := cfg.Resolved()
	credentials := resolved.Credentials
	if len(credentials) == 0 {
		fmt.Println("Not logged in to any account")
		return nil
	}

	// プロファイル情報を取得
	profiles := resolved.Profiles

	ctx := context.Background()

	if logoutAll {
		for profileName := range credentials {
			if err := cfg.DeleteCredential(profileName); err != nil {
				return fmt.Errorf("failed to delete credential for %s: %w", profileName, err)
			}
		}
		if err := cfg.Save(ctx); err != nil {
			return fmt.Errorf("failed to save config: %w", err)
		}
		fmt.Println("Logged out from all accounts")
		return nil
	}

	// アカウント選択（表示用のラベルを作成）
	profileNames := make([]string, 0, len(credentials))
	labels := make([]string, 0, len(credentials))
	for profileName, cred := range credentials {
		profileNames = append(profileNames, profileName)

		// プロファイルからhost情報を取得
		var host string
		if profile, ok := profiles[profileName]; ok && profile.Space != "" && profile.Domain != "" {
			host = profile.Space + "." + profile.Domain
		} else {
			host = "(not configured)"
		}

		labels = append(labels, fmt.Sprintf("[%s] %s (%s)", profileName, host, cred.UserName))
	}

	var selectedProfile string
	var selectedLabel string
	if len(profileNames) == 1 {
		selectedProfile = profileNames[0]
		selectedLabel = labels[0]
	} else {
		selectedLabel, err = ui.Select("Select account to log out:", labels)
		if err != nil {
			return err
		}
		// 選択されたラベルからプロファイル名を取得
		for i, label := range labels {
			if label == selectedLabel {
				selectedProfile = profileNames[i]
				break
			}
		}
	}

	if err := cfg.DeleteCredential(selectedProfile); err != nil {
		return fmt.Errorf("failed to delete credential: %w", err)
	}

	if err := cfg.Save(ctx); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	fmt.Printf("Logged out from %s\n", selectedLabel)
	return nil
}
