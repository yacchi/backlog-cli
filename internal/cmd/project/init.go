package project

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/yacchi/backlog-cli/internal/cmdutil"
	"github.com/yacchi/backlog-cli/internal/config"
	"github.com/yacchi/backlog-cli/internal/ui"
)

var initCmd = &cobra.Command{
	Use:   "init [project-key]",
	Short: "Initialize project configuration",
	Long: `Create a .backlog.yaml file in the current directory.

This file can be committed to your repository to share project settings
with your team.

Examples:
  backlog project init
  backlog project init PROJ
  backlog project init --space other-space --domain backlog.com`,
	RunE: runInit,
}

var (
	initSpace  string
	initDomain string
	initForce  bool
)

func init() {
	initCmd.Flags().StringVar(&initSpace, "space", "", "Backlog space (optional)")
	initCmd.Flags().StringVar(&initDomain, "domain", "", "Backlog domain (optional)")
	initCmd.Flags().BoolVarP(&initForce, "force", "f", false, "Overwrite existing file")
}

func runInit(cmd *cobra.Command, args []string) error {
	// 既存ファイルチェック
	configPath := ".backlog.yaml"
	if _, err := os.Stat(configPath); err == nil && !initForce {
		return fmt.Errorf(".backlog.yaml already exists\nUse --force to overwrite")
	}

	var projectKey string

	if len(args) > 0 {
		projectKey = args[0]
	} else {
		// プロジェクト選択
		client, cfg, err := cmdutil.GetAPIClient(cmd)
		if err != nil {
			// 認証されていない場合は手動入力
			projectKey, err = ui.Input("Project key:", "")
			if err != nil {
				return err
			}
		} else {
			// プロジェクト一覧から選択
			projects, err := client.GetProjects(cmd.Context())
			if err != nil {
				return fmt.Errorf("failed to get projects: %w", err)
			}

			if len(projects) == 0 {
				return fmt.Errorf("no projects found")
			}

			opts := make([]ui.SelectOption, len(projects))
			for i, p := range projects {
				opts[i] = ui.SelectOption{
					Value:       p.ProjectKey,
					Description: p.Name,
				}
			}

			projectKey, err = ui.SelectWithDesc("Select project:", opts)
			if err != nil {
				return err
			}

			// デフォルト値を設定
			profile := cfg.CurrentProfile()
			if initSpace == "" && profile != nil && profile.Space != "" {
				initSpace = profile.Space
			}
			if initDomain == "" && profile != nil && profile.Domain != "" {
				initDomain = profile.Domain
			}
		}
	}

	if projectKey == "" {
		return fmt.Errorf("project key is required")
	}

	// Store API を使用してプロジェクト設定を保存
	ctx := cmd.Context()
	cfg, err := config.Load(ctx)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// カレントディレクトリをプロジェクトルートとして設定
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get current directory: %w", err)
	}
	cfg.SetProjectConfigPath(config.GetProjectConfigPathForRoot(cwd))

	// プロジェクト設定を書き込み
	if err := cfg.SetProjectValue(config.LayerProject, "name", projectKey); err != nil {
		return fmt.Errorf("failed to set project name: %w", err)
	}

	if err := cfg.Save(ctx); err != nil {
		return fmt.Errorf("failed to write config: %w", err)
	}

	ui.Success("Created %s", configPath)

	// 内容表示
	fmt.Println()
	fmt.Printf("project:\n")
	fmt.Printf("  name: %s\n", projectKey)

	// .gitignore チェック
	gitignorePath := ".gitignore"
	if _, err := os.Stat(gitignorePath); err == nil {
		fmt.Println()
		fmt.Println(ui.Gray("Note: .backlog.yaml can be committed to share settings with your team"))
	}

	return nil
}
