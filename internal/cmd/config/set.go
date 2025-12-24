package config

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/yacchi/backlog-cli/internal/config"
	"github.com/yacchi/backlog-cli/internal/ui"
)

var (
	setGlobal  bool
	setProject bool
)

var setCmd = &cobra.Command{
	Use:   "set <key> <value>",
	Short: "Set a configuration value",
	Long: `Set a configuration value.

By default, saves to user config (~/.config/backlog/config.yaml).
Use --project to save to project config (.backlog.yaml in project root).

Examples:
  backlog config set profile.default.space mycompany
  backlog config set profile.default.project PROJ
  backlog config set --project profile.default.project PROJ`,
	Args: cobra.ExactArgs(2),
	RunE: runSet,
}

func init() {
	setCmd.Flags().BoolVarP(&setGlobal, "global", "g", false, "Save to user config (default)")
	setCmd.Flags().BoolVarP(&setProject, "project", "p", false, "Save to project config (.backlog.yaml)")
	setCmd.MarkFlagsMutuallyExclusive("global", "project")
}

func runSet(cmd *cobra.Command, args []string) error {
	key := args[0]
	value := parseConfigValue(args[1])

	ctx := cmd.Context()
	cfg, err := config.Load(ctx)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// 書き込み先の決定と保存
	if setProject {
		projectRoot, err := resolveProjectRoot(cfg)
		if err != nil {
			return err
		}
		configPath := config.GetProjectConfigPathForRoot(projectRoot)
		ui.Info("Writing to project config: %s", configPath)
		cfg.SetProjectConfigPath(configPath)

		if err := cfg.SetToLayer(config.LayerProject, key, value); err != nil {
			return err
		}
	} else {
		if err := cfg.Set(key, value); err != nil {
			return err
		}
	}

	if err := cfg.Save(ctx); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	ui.Success("Set %s = %s", key, value)
	return nil
}

func parseConfigValue(value string) any {
	normalized := strings.ToLower(strings.TrimSpace(value))
	switch normalized {
	case "true":
		return true
	case "false":
		return false
	default:
		return value
	}
}

// resolveProjectRoot はプロジェクトルートを解決する
// 自動検出できない場合はユーザーに選択させる
func resolveProjectRoot(cfg *config.Store) (string, error) {
	// 1. 既存のプロジェクト設定または.gitを探す
	root, err := cfg.GetProjectRoot()
	if err != nil {
		return "", fmt.Errorf("failed to find project root: %w", err)
	}

	if root != "" {
		return root, nil
	}

	// 2. 見つからない場合はユーザーに選択させる
	fmt.Println("Could not detect project root automatically.")
	fmt.Println("Please select the project root directory.")

	cwd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("failed to get current directory: %w", err)
	}

	selectedDir, err := ui.SelectDirectory(cwd)
	if err != nil {
		return "", fmt.Errorf("directory selection cancelled: %w", err)
	}

	return selectedDir, nil
}
