package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/yacchi/backlog-cli/packages/backlog/internal/cmd/activity"
	"github.com/yacchi/backlog-cli/packages/backlog/internal/cmd/ai"
	apicmd "github.com/yacchi/backlog-cli/packages/backlog/internal/cmd/api"
	"github.com/yacchi/backlog-cli/packages/backlog/internal/cmd/auth"
	"github.com/yacchi/backlog-cli/packages/backlog/internal/cmd/category"
	configcmd "github.com/yacchi/backlog-cli/packages/backlog/internal/cmd/config"
	"github.com/yacchi/backlog-cli/packages/backlog/internal/cmd/customfield"
	"github.com/yacchi/backlog-cli/packages/backlog/internal/cmd/document"
	"github.com/yacchi/backlog-cli/packages/backlog/internal/cmd/file"
	"github.com/yacchi/backlog-cli/packages/backlog/internal/cmd/issue"
	"github.com/yacchi/backlog-cli/packages/backlog/internal/cmd/issue_type"
	"github.com/yacchi/backlog-cli/packages/backlog/internal/cmd/markdown"
	"github.com/yacchi/backlog-cli/packages/backlog/internal/cmd/milestone"
	"github.com/yacchi/backlog-cli/packages/backlog/internal/cmd/notification"
	"github.com/yacchi/backlog-cli/packages/backlog/internal/cmd/pr"
	"github.com/yacchi/backlog-cli/packages/backlog/internal/cmd/priority"
	"github.com/yacchi/backlog-cli/packages/backlog/internal/cmd/profile"
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

		// --profile と --space の排他チェック
		profileFlag, _ := cmd.Flags().GetString("profile")
		spaceFlag, _ := cmd.Flags().GetString("space")

		if profileFlag != "" && spaceFlag != "" {
			return fmt.Errorf("cannot use --profile and --space together")
		}

		if profileFlag != "" {
			cfg.SetActiveProfile(profileFlag)
		} else if spaceFlag != "" {
			profileName, created, err := cfg.EnsureSpaceProfile(ctx, spaceFlag)
			if err != nil {
				return err
			}
			cfg.SetActiveProfile(profileName)

			if created && !isAuthCommand(cmd) {
				if cfg.Credential(profileName) == nil {
					space, domain, _ := config.ParseSpaceHost(spaceFlag)
					if ui.IsInteractiveInput() {
						fmt.Printf("Authentication required for %s\n", spaceFlag)
						if err := auth.RunLoginForProfile(ctx, cfg); err != nil {
							return fmt.Errorf("authentication failed for %s: %w", spaceFlag, err)
						}
					} else {
						return fmt.Errorf("authentication required for %s\nRun: backlog auth login --space %s --domain %s", spaceFlag, space, domain)
					}
				}
			}
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
		// --json（値なし）は NoOptDefVal="*" により全フィールド出力
		if jsonFields, _ := cmd.Flags().GetString("json"); jsonFields != "" {
			activeProfile := cfg.GetActiveProfile()
			setOptions = append(setOptions, jubako.String(config.PathProfileOutput(activeProfile), "json"))
			if jsonFields != "*" {
				setOptions = append(setOptions, jubako.String(config.PathProfileJsonFields(activeProfile), jsonFields))
			}
		}

		// jqフラグはアクティブプロファイルに設定
		if jqFilter, _ := cmd.Flags().GetString("jq"); jqFilter != "" {
			activeProfile := cfg.GetActiveProfile()
			setOptions = append(setOptions, jubako.String(config.PathProfileJq(activeProfile), jqFilter))
		}

		// formatフラグはGo templateを使ったJSON出力を有効にする
		if tmpl, _ := cmd.Flags().GetString("format"); tmpl != "" {
			activeProfile := cfg.GetActiveProfile()
			setOptions = append(setOptions, jubako.String(config.PathProfileOutput(activeProfile), "json"))
			setOptions = append(setOptions, jubako.String(config.PathProfileTemplate(activeProfile), tmpl))
		}

		if len(setOptions) > 0 {
			return cfg.SetFlagsLayer(setOptions)
		}

		return nil
	},
}

func isAuthCommand(cmd *cobra.Command) bool {
	for c := cmd; c != nil; c = c.Parent() {
		if c.Name() == "auth" {
			return true
		}
	}
	return false
}

func Execute() error {
	rootCmd.Version = Version
	return rootCmd.Execute()
}

func init() {
	// グローバルフラグ
	rootCmd.PersistentFlags().String("profile", "", "Configuration profile to use")
	rootCmd.PersistentFlags().String("space", "", "Resolve profile by space host (e.g. myspace.backlog.jp)")
	rootCmd.PersistentFlags().StringP("project", "p", "", "Backlog project key")
	rootCmd.PersistentFlags().StringP("output", "o", "", "Output format (table, json)")
	rootCmd.PersistentFlags().String("json", "", "Output JSON with specified fields (comma-separated, omit for all)")
	rootCmd.PersistentFlags().Lookup("json").NoOptDefVal = "*"
	rootCmd.PersistentFlags().String("jq", "", "Filter JSON output using a jq expression")
	rootCmd.PersistentFlags().StringP("format", "f", "", "Format JSON output using a Go template (e.g. '{{.summary}}')")
	rootCmd.PersistentFlags().Bool("no-color", false, "Disable color output")
	rootCmd.PersistentFlags().Bool("debug", false, "Enable debug logging")
	rootCmd.PersistentFlags().BoolP("yes", "y", false, "Skip confirmation prompts (env: BACKLOG_ASSUME_YES)")

	// サブコマンド登録
	rootCmd.AddCommand(activity.ActivityCmd)
	rootCmd.AddCommand(ai.AICmd)
	rootCmd.AddCommand(apicmd.APICmd)
	rootCmd.AddCommand(auth.AuthCmd)
	rootCmd.AddCommand(category.CategoryCmd)
	rootCmd.AddCommand(configcmd.ConfigCmd)
	rootCmd.AddCommand(customfield.CustomFieldCmd)
	rootCmd.AddCommand(document.DocumentCmd)
	rootCmd.AddCommand(file.FileCmd)
	rootCmd.AddCommand(issue.IssueCmd)
	rootCmd.AddCommand(issue_type.IssueTypeCmd)
	rootCmd.AddCommand(markdown.MarkdownCmd)
	rootCmd.AddCommand(milestone.MilestoneCmd)
	rootCmd.AddCommand(notification.NotificationCmd)
	rootCmd.AddCommand(pr.PRCmd)
	rootCmd.AddCommand(priority.PriorityCmd)
	rootCmd.AddCommand(profile.ProfileCmd)
	rootCmd.AddCommand(project.ProjectCmd)
	rootCmd.AddCommand(repo.RepoCmd)
	rootCmd.AddCommand(resolution.ResolutionCmd)
	rootCmd.AddCommand(space.SpaceCmd)
	rootCmd.AddCommand(status.StatusCmd)
	rootCmd.AddCommand(user.UserCmd)
	rootCmd.AddCommand(watching.WatchingCmd)
	rootCmd.AddCommand(wiki.WikiCmd)

	// whoami: top-level alias for "auth me"
	whoamiCmd := &cobra.Command{
		Use:   "whoami",
		Short: "Show current authenticated user (alias for 'auth me')",
		Long: `Display detailed information about the currently authenticated Backlog user.
This is an alias for 'backlog auth me'.

Examples:
  backlog whoami
  backlog whoami -o json
  backlog whoami -o json --jq .id`,
		RunE: auth.RunMe,
	}
	rootCmd.AddCommand(whoamiCmd)
}
