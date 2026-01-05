package config

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/yacchi/backlog-cli/packages/backlog/internal/config"
)

var pathCmd = &cobra.Command{
	Use:   "path",
	Short: "Show configuration file path",
	RunE:  runPath,
}

func runPath(cmd *cobra.Command, _ []string) error {
	cfg, err := config.Load(cmd.Context())
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// ユーザー設定ファイル
	fmt.Printf("Config:      %s\n", cfg.GetUserConfigPath())

	// クレデンシャルファイル
	fmt.Printf("Credentials: %s\n", cfg.GetCredentialsPath())

	// プロジェクトローカル設定
	if projectPath := cfg.GetProjectConfigPath(); projectPath != "" {
		fmt.Printf("Project:     %s\n", projectPath)
	}

	// キャッシュディレクトリ
	if cacheDir, err := cfg.GetCacheDir(); err == nil {
		fmt.Printf("Cache:       %s\n", cacheDir)
	}

	return nil
}
