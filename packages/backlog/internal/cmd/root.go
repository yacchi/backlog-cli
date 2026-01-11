package cmd

import (
	"strings"

	"github.com/spf13/cobra"
	apicmd "github.com/yacchi/backlog-cli/packages/backlog/internal/cmd/api"
	"github.com/yacchi/backlog-cli/packages/backlog/internal/cmd/auth"
	"github.com/yacchi/backlog-cli/packages/backlog/internal/cmd/category"
	configcmd "github.com/yacchi/backlog-cli/packages/backlog/internal/cmd/config"
	"github.com/yacchi/backlog-cli/packages/backlog/internal/cmd/customfield"
	"github.com/yacchi/backlog-cli/packages/backlog/internal/cmd/issue"
	"github.com/yacchi/backlog-cli/packages/backlog/internal/cmd/issue_type"
	"github.com/yacchi/backlog-cli/packages/backlog/internal/cmd/markdown"
	"github.com/yacchi/backlog-cli/packages/backlog/internal/cmd/milestone"
	"github.com/yacchi/backlog-cli/packages/backlog/internal/cmd/notification"
	"github.com/yacchi/backlog-cli/packages/backlog/internal/cmd/pr"
	"github.com/yacchi/backlog-cli/packages/backlog/internal/cmd/priority"
	"github.com/yacchi/backlog-cli/packages/backlog/internal/cmd/project"
	"github.com/yacchi/backlog-cli/packages/backlog/internal/cmd/repo"
	"github.com/yacchi/backlog-cli/packages/backlog/internal/cmd/resolution"
	"github.com/yacchi/backlog-cli/packages/backlog/internal/cmd/space"
	"github.com/yacchi/backlog-cli/packages/backlog/internal/cmd/status"
	"github.com/yacchi/backlog-cli/packages/backlog/internal/cmd/user"
	"github.com/yacchi/backlog-cli/packages/backlog/internal/cmd/watching"
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

		// jsonフラグはJSON出力を有効にし、フィールドを設定
		if jsonFields, _ := cmd.Flags().GetString("json"); jsonFields != "" {
			activeProfile := cfg.GetActiveProfile()
			setOptions = append(setOptions, jubako.String(config.PathProfileOutput(activeProfile), "json"))
			setOptions = append(setOptions, jubako.String(config.PathProfileJsonFields(activeProfile), jsonFields))
		}

		// jqフラグはアクティブプロファイルに設定
		if jqFilter, _ := cmd.Flags().GetString("jq"); jqFilter != "" {
			activeProfile := cfg.GetActiveProfile()
			setOptions = append(setOptions, jubako.String(config.PathProfileJq(activeProfile), jqFilter))
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
	rootCmd.PersistentFlags().String("output", "", "Output format (table, json)")
	rootCmd.PersistentFlags().String("json", "", "Output JSON with specified fields (comma-separated)")
	rootCmd.PersistentFlags().String("jq", "", "Filter JSON output using a jq expression")
	rootCmd.PersistentFlags().Bool("no-color", false, "Disable color output")
	rootCmd.PersistentFlags().Bool("debug", false, "Enable debug logging")

	// サブコマンド登録
	rootCmd.AddCommand(apicmd.APICmd)
	rootCmd.AddCommand(auth.AuthCmd)
	rootCmd.AddCommand(category.CategoryCmd)
	rootCmd.AddCommand(configcmd.ConfigCmd)
	rootCmd.AddCommand(customfield.CustomFieldCmd)
	rootCmd.AddCommand(issue.IssueCmd)
	rootCmd.AddCommand(issue_type.IssueTypeCmd)
	rootCmd.AddCommand(markdown.MarkdownCmd)
	rootCmd.AddCommand(milestone.MilestoneCmd)
	rootCmd.AddCommand(notification.NotificationCmd)
	rootCmd.AddCommand(pr.PRCmd)
	rootCmd.AddCommand(priority.PriorityCmd)
	rootCmd.AddCommand(project.ProjectCmd)
	rootCmd.AddCommand(repo.RepoCmd)
	rootCmd.AddCommand(resolution.ResolutionCmd)
	rootCmd.AddCommand(space.SpaceCmd)
	rootCmd.AddCommand(status.StatusCmd)
	rootCmd.AddCommand(user.UserCmd)
	rootCmd.AddCommand(watching.WatchingCmd)
	rootCmd.AddCommand(wiki.WikiCmd)
}
