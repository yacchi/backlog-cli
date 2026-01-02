package cmd

import (
	"strings"

	"github.com/spf13/cobra"
	"github.com/yacchi/backlog-cli/packages/backlog/internal/cmd/auth"
	configcmd "github.com/yacchi/backlog-cli/packages/backlog/internal/cmd/config"
	"github.com/yacchi/backlog-cli/packages/backlog/internal/cmd/issue"
	"github.com/yacchi/backlog-cli/packages/backlog/internal/cmd/issue_type"
	"github.com/yacchi/backlog-cli/packages/backlog/internal/cmd/markdown"
	"github.com/yacchi/backlog-cli/packages/backlog/internal/cmd/pr"
	"github.com/yacchi/backlog-cli/packages/backlog/internal/cmd/project"
	"github.com/yacchi/backlog-cli/packages/backlog/internal/cmd/serve"
	"github.com/yacchi/backlog-cli/packages/backlog/internal/cmd/wiki"
	"github.com/yacchi/backlog-cli/packages/backlog/internal/config"
	"github.com/yacchi/backlog-cli/packages/backlog/internal/debug"
	"github.com/yacchi/backlog-cli/packages/backlog/internal/ui"
	"github.com/yacchi/jubako"
)

var (
	Version   = "dev"
	Commit    = "unknown"
	BuildDate = "unknown"
)

var rootCmd = &cobra.Command{
	Use:   "backlog",
	Short: "Backlog CLI - A command line interface for Backlog",
	Long: `Backlog CLI brings Backlog to your terminal.

Work with issues, pull requests, wikis, and more, all from the command line.`,
	SilenceUsage:  true,
	SilenceErrors: true,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		// デバッグモードの有効化
		if debugFlag, _ := cmd.Flags().GetBool("debug"); debugFlag {
			debug.Enable()
		}

		ctx := cmd.Context()
		cfg, err := config.Load(ctx)
		if err != nil {
			return err
		}

		// profileフラグが指定されていればアクティブプロファイルを切り替え
		if profile, _ := cmd.Flags().GetString("profile"); profile != "" {
			cfg.SetActiveProfile(profile)
		}

		// カラー設定
		if noColor, _ := cmd.Flags().GetBool("no-color"); noColor {
			ui.SetColorEnabled(false)
		} else if profile := cfg.CurrentProfile(); profile != nil {
			switch strings.ToLower(profile.Color) {
			case "never":
				ui.SetColorEnabled(false)
			case "always":
				ui.SetColorEnabled(true)
			}
		}

		// グローバルフラグを取得してArgsレイヤーに適用
		var setOptions []jubako.SetOption

		// projectフラグは /project/name に設定（プロジェクト設定を優先する設計に合わせる）
		if project, _ := cmd.Flags().GetString("project"); project != "" {
			setOptions = append(setOptions, jubako.String(config.PathProjectName, project))
		}

		// outputフラグはアクティブプロファイルに設定
		if output, _ := cmd.Flags().GetString("output"); output != "" {
			activeProfile := cfg.GetActiveProfile()
			setOptions = append(setOptions, jubako.String(config.PathProfileOutput(activeProfile), output))
		}

		// formatフラグはアクティブプロファイルに設定
		if format, _ := cmd.Flags().GetString("format"); format != "" {
			activeProfile := cfg.GetActiveProfile()
			setOptions = append(setOptions, jubako.String(config.PathProfileFormat(activeProfile), format))
		}

		if len(setOptions) > 0 {
			return cfg.SetFlagsLayer(setOptions)
		}

		return nil
	},
}

func Execute() error {
	return rootCmd.Execute()
}

func init() {
	// グローバルフラグ
	rootCmd.PersistentFlags().String("profile", "", "Configuration profile to use")
	rootCmd.PersistentFlags().StringP("project", "p", "", "Backlog project key")
	rootCmd.PersistentFlags().StringP("output", "o", "", "Output format (table, json)")
	rootCmd.PersistentFlags().StringP("format", "f", "", "Format output using a Go template")
	rootCmd.PersistentFlags().Bool("no-color", false, "Disable color output")
	rootCmd.PersistentFlags().Bool("debug", false, "Enable debug logging")

	// サブコマンド登録
	rootCmd.AddCommand(auth.AuthCmd)
	rootCmd.AddCommand(configcmd.ConfigCmd)
	rootCmd.AddCommand(issue.IssueCmd)
	rootCmd.AddCommand(issue_type.IssueTypeCmd)
	rootCmd.AddCommand(markdown.MarkdownCmd)
	rootCmd.AddCommand(pr.PRCmd)
	rootCmd.AddCommand(project.ProjectCmd)
	rootCmd.AddCommand(serve.ServeCmd)
	rootCmd.AddCommand(wiki.WikiCmd)
}
