package issue

import (
	"fmt"
	"strings"

	"github.com/pkg/browser"
	"github.com/spf13/cobra"
	"github.com/yacchi/backlog-cli/internal/api"
	"github.com/yacchi/backlog-cli/internal/backlog"
	"github.com/yacchi/backlog-cli/internal/cmdutil"
	"github.com/yacchi/backlog-cli/internal/config"
	"github.com/yacchi/backlog-cli/internal/ui"
)

var viewCmd = &cobra.Command{
	Use:   "view <issue-key>",
	Short: "View an issue",
	Long: `View detailed information about an issue.

If project is configured, you can omit the project prefix:
  backlog issue view 123       # equivalent to PROJ-123 when project=PROJ

Examples:
  backlog issue view PROJ-123
  backlog issue view 123       # uses configured project
  backlog issue view PROJ-123 --comments`,
	Args: cobra.ExactArgs(1),
	RunE: runView,
}

var (
	viewComments bool
	viewWeb      bool
)

func init() {
	viewCmd.Flags().BoolVarP(&viewComments, "comments", "c", false, "Show comments")
	viewCmd.Flags().BoolVarP(&viewWeb, "web", "w", false, "Open in browser")
}

func runView(c *cobra.Command, args []string) error {
	issueKey := args[0]

	client, cfg, err := cmdutil.GetAPIClient(c)
	if err != nil {
		return err
	}

	profile := cfg.CurrentProfile()
	display := cfg.Display()

	// 課題キーの解決（プロジェクトキーの補完または抽出）
	issueKey, _ = cmdutil.ResolveIssueKey(issueKey, cmdutil.GetCurrentProject(cfg))

	// ブラウザで開く
	if viewWeb {
		url := fmt.Sprintf("https://%s.%s/view/%s", profile.Space, profile.Domain, issueKey)
		return browser.OpenURL(url)
	}

	// 課題取得
	issue, err := client.GetIssue(issueKey)
	if err != nil {
		return fmt.Errorf("failed to get issue: %w", err)
	}

	// 出力
	switch profile.Output {
	case "json":
		return outputJSON(issue)
	default:
		return renderIssueDetail(client, issue, profile, display)
	}
}

func renderIssueDetail(client *api.Client, issue *backlog.Issue, profile *config.ResolvedProfile, display *config.ResolvedDisplay) error {
	// ハイパーリンク設定
	ui.SetHyperlinkEnabled(display.Hyperlink)

	// フィールドフォーマッター
	formatter := ui.NewFieldFormatter(display.Timezone, display.DateTimeFormat, display.IssueFieldConfig)

	// URL生成
	key := issue.IssueKey.Value
	issueURL := fmt.Sprintf("https://%s.%s/view/%s", profile.Space, profile.Domain, key)

	// ヘッダー（キーをハイパーリンク化）
	fmt.Printf("%s %s\n", ui.Bold(ui.Hyperlink(issueURL, key)), issue.Summary.Value)
	fmt.Println(strings.Repeat("─", 60))

	// メタ情報
	if issue.Status.IsSet() && issue.Status.Value.Name.IsSet() {
		fmt.Printf("Status:     %s\n", ui.StatusColor(issue.Status.Value.Name.Value))
	}
	if issue.IssueType.IsSet() && issue.IssueType.Value.Name.IsSet() {
		fmt.Printf("Type:       %s\n", issue.IssueType.Value.Name.Value)
	}
	if issue.Priority.IsSet() && issue.Priority.Value.Name.IsSet() {
		fmt.Printf("Priority:   %s\n", ui.PriorityColor(issue.Priority.Value.Name.Value))
	}

	if issue.Assignee.IsSet() && issue.Assignee.Value.Name.IsSet() {
		fmt.Printf("Assignee:   %s\n", issue.Assignee.Value.Name.Value)
	} else {
		fmt.Printf("Assignee:   %s\n", ui.Gray("(unassigned)"))
	}

	created := issue.Created.Value
	createdUser := ""
	if issue.CreatedUser.IsSet() && issue.CreatedUser.Value.Name.IsSet() {
		createdUser = issue.CreatedUser.Value.Name.Value
	}
	fmt.Printf("Created:    %s by %s\n", formatter.FormatDateTime(created, "created"), createdUser)

	if issue.UpdatedUser.IsSet() && issue.UpdatedUser.Value.Name.IsSet() {
		fmt.Printf("Updated:    %s by %s\n", formatter.FormatDateTime(issue.Updated.Value, "updated"), issue.UpdatedUser.Value.Name.Value)
	}

	if issue.DueDate.IsSet() && !issue.DueDate.IsNull() {
		fmt.Printf("Due Date:   %s\n", formatter.FormatDate(issue.DueDate.Value, "due_date"))
	}

	// カテゴリー
	if len(issue.Category) > 0 {
		cats := make([]string, len(issue.Category))
		for i, c := range issue.Category {
			if c.Name.IsSet() {
				cats[i] = c.Name.Value
			}
		}
		fmt.Printf("Categories: %s\n", strings.Join(cats, ", "))
	}

	// マイルストーン
	if len(issue.Milestone) > 0 {
		milestones := make([]string, len(issue.Milestone))
		for i, m := range issue.Milestone {
			if m.Name.IsSet() {
				milestones[i] = m.Name.Value
			}
		}
		fmt.Printf("Milestone:  %s\n", strings.Join(milestones, ", "))
	}

	// 説明
	if issue.Description.IsSet() && issue.Description.Value != "" {
		fmt.Println()
		fmt.Println(ui.Bold("Description"))
		fmt.Println(strings.Repeat("─", 60))
		fmt.Println(issue.Description.Value)
	}

	// URL（常に表示、ハイパーリンク化）
	fmt.Println()
	fmt.Printf("URL: %s\n", ui.Hyperlink(issueURL, ui.Cyan(issueURL)))

	// コメント
	if viewComments {
		comments, err := client.GetComments(key, &api.CommentListOptions{
			Count: 10,
			Order: "desc",
		})
		if err == nil && len(comments) > 0 {
			fmt.Println()
			fmt.Println(ui.Bold("Recent Comments"))
			fmt.Println(strings.Repeat("─", 60))

			for _, comment := range comments {
				fmt.Printf("\n%s %s\n", ui.Bold(comment.CreatedUser.Name), ui.Gray(formatter.FormatDateTime(comment.Created, "created")))
				fmt.Println(comment.Content)
			}
		}
	}

	return nil
}