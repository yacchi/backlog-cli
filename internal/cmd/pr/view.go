package pr

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/pkg/browser"
	"github.com/spf13/cobra"
	"github.com/yacchi/backlog-cli/internal/api"
	"github.com/yacchi/backlog-cli/internal/cmdutil"
	"github.com/yacchi/backlog-cli/internal/config"
	"github.com/yacchi/backlog-cli/internal/ui"
)

var viewCmd = &cobra.Command{
	Use:   "view <number>",
	Short: "View a pull request",
	Long: `View detailed information about a pull request.

Examples:
  backlog pr view 123 --repo myrepo
  backlog pr view 123 --repo myrepo --web`,
	Args: cobra.ExactArgs(1),
	RunE: runView,
}

var (
	viewRepo string
	viewWeb  bool
)

func init() {
	viewCmd.Flags().StringVarP(&viewRepo, "repo", "r", "", "Repository name (required)")
	viewCmd.Flags().BoolVarP(&viewWeb, "web", "w", false, "Open in browser")
	viewCmd.MarkFlagRequired("repo")
}

func runView(c *cobra.Command, args []string) error {
	number, err := strconv.Atoi(args[0])
	if err != nil {
		return fmt.Errorf("invalid pull request number: %s", args[0])
	}

	client, cfg, err := cmdutil.GetAPIClient(c)
	if err != nil {
		return err
	}

	if err := cmdutil.RequireProject(cfg); err != nil {
		return err
	}

	profile := cfg.CurrentProfile()
	display := cfg.Display()
	projectKey := cmdutil.GetCurrentProject(cfg)

	// ブラウザで開く
	if viewWeb {
		url := fmt.Sprintf("https://%s.%s/git/%s/%s/pullRequests/%d",
			profile.Space, profile.Domain, projectKey, viewRepo, number)
		return browser.OpenURL(url)
	}

	// PR取得
	pr, err := client.GetPullRequest(projectKey, viewRepo, number)
	if err != nil {
		return fmt.Errorf("failed to get pull request: %w", err)
	}

	// 出力
	switch profile.Output {
	case "json":
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(pr)
	default:
		return renderPRDetail(pr, profile, display, projectKey)
	}
}

func renderPRDetail(pr *api.PullRequest, profile *config.ResolvedProfile, display *config.ResolvedDisplay, projectKey string) error {
	// ハイパーリンク設定
	ui.SetHyperlinkEnabled(display.Hyperlink)

	// フィールドフォーマッター
	formatter := ui.NewFieldFormatter(display.Timezone, display.DateTimeFormat, display.PRFieldConfig)

	// URL生成
	prURL := fmt.Sprintf("https://%s.%s/git/%s/%s/pullRequests/%d",
		profile.Space, profile.Domain, projectKey, viewRepo, pr.Number)

	// ヘッダー（PR番号をハイパーリンク化）
	fmt.Printf("%s %s\n", ui.Hyperlink(prURL, fmt.Sprintf("#%d", pr.Number)), ui.Bold(pr.Summary))
	fmt.Println(strings.Repeat("─", 60))

	// ステータス
	status := pr.Status.Name
	switch pr.Status.ID {
	case 1:
		status = ui.Green("Open")
	case 2:
		status = ui.Red("Closed")
	case 3:
		status = ui.Blue("Merged")
	}
	fmt.Printf("Status:   %s\n", status)

	// ブランチ情報
	fmt.Printf("Branch:   %s -> %s\n", ui.Cyan(pr.Branch), ui.Cyan(pr.Base))

	// 担当者
	if pr.Assignee != nil {
		fmt.Printf("Assignee: %s\n", pr.Assignee.Name)
	}

	// 作成者と日時
	fmt.Printf("Created:  %s by %s\n", formatter.FormatDateTime(pr.Created, "created"), pr.CreatedUser.Name)
	if pr.UpdatedUser != nil {
		fmt.Printf("Updated:  %s by %s\n", formatter.FormatDateTime(pr.Updated, "updated"), pr.UpdatedUser.Name)
	}

	// 関連課題（ハイパーリンク化）
	if pr.Issue != nil {
		issueURL := fmt.Sprintf("https://%s.%s/view/%s", profile.Space, profile.Domain, pr.Issue.IssueKey)
		fmt.Printf("Issue:    %s %s\n", ui.Hyperlink(issueURL, pr.Issue.IssueKey), pr.Issue.Summary)
	}

	// 説明
	if pr.Description != "" {
		fmt.Println()
		fmt.Println(ui.Bold("Description"))
		fmt.Println(strings.Repeat("─", 60))
		fmt.Println(pr.Description)
	}

	// URL（常に表示、ハイパーリンク化）
	fmt.Println()
	fmt.Printf("URL: %s\n", ui.Hyperlink(prURL, ui.Cyan(prURL)))

	return nil
}
