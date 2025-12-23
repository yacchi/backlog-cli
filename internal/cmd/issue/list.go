package issue

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/pkg/browser"
	"github.com/spf13/cobra"
	"github.com/yacchi/backlog-cli/internal/api"
	"github.com/yacchi/backlog-cli/internal/backlog"
	"github.com/yacchi/backlog-cli/internal/cmdutil"
	"github.com/yacchi/backlog-cli/internal/config"
	"github.com/yacchi/backlog-cli/internal/summary"
	"github.com/yacchi/backlog-cli/internal/ui"
)

var listCmd = &cobra.Command{
	Use:     "list",
	Aliases: []string{"ls"},
	Short:   "List issues",
	Long: `List issues in a project.

Examples:
  # List open issues (default)
  backlog issue list
  backlog issue ls

  # Filter by assignee
  backlog issue list --assignee @me
  backlog issue list --mine

  # Filter by state
  backlog issue list --state closed
  backlog issue list --state all

  # Search issues
  backlog issue list --search "bug fix"

  # Open issue list in browser
  backlog issue list --web`,
	RunE: runList,
}

var (
	listAssignee            string
	listState               string
	listLimit               int
	listSearch              string
	listMine                bool
	listWeb                 bool
	listSummary             bool
	listSummaryWithComments bool
	listSummaryCommentCount int
)

func init() {
	listCmd.Flags().StringVarP(&listAssignee, "assignee", "a", "", "Filter by assignee (user ID or @me)")
	listCmd.Flags().StringVarP(&listState, "state", "s", "open", "Filter by state: {open|closed|all}")
	listCmd.Flags().IntVarP(&listLimit, "limit", "L", 30, "Maximum number of issues to fetch")
	listCmd.Flags().StringVarP(&listSearch, "search", "S", "", "Search issues with keyword")
	listCmd.Flags().BoolVar(&listMine, "mine", false, "Show only my issues")
	listCmd.Flags().BoolVarP(&listWeb, "web", "w", false, "Open issue list in browser")
	listCmd.Flags().BoolVar(&listSummary, "summary", false, "Show AI summary column (description only)")
	listCmd.Flags().BoolVar(&listSummaryWithComments, "summary-with-comments", false, "Include comments in AI summary")
	listCmd.Flags().IntVar(&listSummaryCommentCount, "summary-comment-count", -1, "Number of comments to use for summary")
}

func runList(c *cobra.Command, args []string) error {
	client, cfg, err := cmdutil.GetAPIClient(c)
	if err != nil {
		return err
	}

	if err := cmdutil.RequireProject(cfg); err != nil {
		return err
	}

	projectKey := cmdutil.GetCurrentProject(cfg)
	profile := cfg.CurrentProfile()
	ctx := c.Context()

	// ブラウザで開く
	if listWeb {
		url := fmt.Sprintf("https://%s.%s/find/%s", profile.Space, profile.Domain, projectKey)
		return browser.OpenURL(url)
	}

	// プロジェクト情報取得
	project, err := client.GetProject(ctx, projectKey)
	if err != nil {
		return fmt.Errorf("failed to get project: %w", err)
	}

	// オプション構築
	opts := &api.IssueListOptions{
		ProjectIDs: []int{project.ID},
		Count:      listLimit,
		Sort:       "updated",
		Order:      "desc",
	}

	if listSearch != "" {
		opts.Keyword = listSearch
	}

	// 担当者フィルター
	if listMine || listAssignee == "@me" {
		// 自分の課題
		me, err := client.GetCurrentUser(ctx)
		if err != nil {
			return fmt.Errorf("failed to get current user: %w", err)
		}
		opts.AssigneeIDs = []int{me.ID.Value}
	} else if listAssignee != "" {
		// 指定ユーザー
		assigneeID, err := strconv.Atoi(listAssignee)
		if err != nil {
			return fmt.Errorf("invalid assignee ID: %s", listAssignee)
		}
		opts.AssigneeIDs = []int{assigneeID}
	}

	// ステータスフィルター（--state オプション）
	switch listState {
	case "open":
		// Backlogのステータス: 1=未対応, 2=処理中, 3=処理済み
		// open = 完了以外（4=完了を除く）
		statuses, err := client.GetStatuses(ctx, strconv.Itoa(project.ID))
		if err == nil {
			var openStatusIDs []int
			for _, s := range statuses {
				// "完了" または "Closed" 以外を含める
				if s.Name != "完了" && s.Name != "Closed" && s.Name != "Done" {
					openStatusIDs = append(openStatusIDs, s.ID)
				}
			}
			if len(openStatusIDs) > 0 {
				opts.StatusIDs = openStatusIDs
			}
		}
	case "closed":
		// closed = 完了のみ
		statuses, err := client.GetStatuses(ctx, strconv.Itoa(project.ID))
		if err == nil {
			for _, s := range statuses {
				if s.Name == "完了" || s.Name == "Closed" || s.Name == "Done" {
					opts.StatusIDs = []int{s.ID}
					break
				}
			}
		}
	case "all":
		// all = フィルターなし
	default:
		return fmt.Errorf("invalid state: %s (must be open, closed, or all)", listState)
	}

	// 課題取得
	issues, err := client.GetIssues(ctx, opts)
	if err != nil {
		return fmt.Errorf("failed to get issues: %w", err)
	}

	if len(issues) == 0 {
		fmt.Println("No issues found")
		return nil
	}

	// 出力
	display := cfg.Display()
	switch profile.Output {
	case "json":
		return cmdutil.OutputJSONFromProfile(issues, profile)
	default:
		outputTable(ctx, client, issues, profile, display)
		return nil
	}
}

func outputTable(ctx context.Context, client *api.Client, issues []backlog.Issue, profile *config.ResolvedProfile, display *config.ResolvedDisplay) {
	// フラグ調整
	if listSummaryWithComments {
		listSummary = true
	}

	summaryCommentCount := display.SummaryCommentCount
	if listSummaryCommentCount >= 0 {
		summaryCommentCount = listSummaryCommentCount
	}

	// フィールドリストをコピーして操作
	fields := make([]string, len(display.IssueListFields))
	copy(fields, display.IssueListFields)

	if listSummary {
		fields = append(fields, "ai_summary")
	}

	fieldConfig := display.IssueFieldConfig

	// ハイパーリンク設定
	ui.SetHyperlinkEnabled(display.Hyperlink)

	// ヘッダー生成
	headers := make([]string, len(fields))
	for i, f := range fields {
		if f == "ai_summary" {
			headers[i] = "AI SUMMARY"
			continue
		}
		if cfg, ok := fieldConfig[f]; ok && cfg.Header != "" {
			headers[i] = cfg.Header
		} else {
			headers[i] = strings.ToUpper(f)
		}
	}

	table := ui.NewTable(headers...)

	// フィールドフォーマッターを作成
	formatter := ui.NewFieldFormatter(display.Timezone, display.DateTimeFormat, fieldConfig)

	// ベースURL生成
	baseURL := fmt.Sprintf("https://%s.%s", profile.Space, profile.Domain)

	for _, issue := range issues {
		row := make([]string, len(fields))
		for i, f := range fields {
			row[i] = getIssueFieldValue(ctx, client, issue, f, formatter, baseURL, summaryCommentCount, listSummaryWithComments)
		}
		table.AddRow(row...)
	}

	table.RenderWithColor(os.Stdout, ui.IsColorEnabled())
}

func getIssueFieldValue(ctx context.Context, client *api.Client, issue backlog.Issue, field string, f *ui.FieldFormatter, baseURL string, summaryCommentCount int, withComments bool) string {
	switch field {
	case "key":
		key := issue.IssueKey.Value
		url := fmt.Sprintf("%s/view/%s", baseURL, key)
		return ui.Hyperlink(url, key)
	case "status":
		if issue.Status.IsSet() && issue.Status.Value.Name.IsSet() {
			return ui.StatusColor(issue.Status.Value.Name.Value)
		}
		return "-"
	case "priority":
		if issue.Priority.IsSet() && issue.Priority.Value.Name.IsSet() {
			return ui.PriorityColor(issue.Priority.Value.Name.Value)
		}
		return "-"
	case "assignee":
		if issue.Assignee.IsSet() && issue.Assignee.Value.Name.IsSet() {
			return f.FormatString(issue.Assignee.Value.Name.Value, field)
		}
		return "-"
	case "summary":
		return f.FormatString(issue.Summary.Value, field)
	case "ai_summary":
		fullText := ""
		if issue.Description.IsSet() {
			fullText = issue.Description.Value
		}

		if withComments && summaryCommentCount > 0 {
			fetchCount := summaryCommentCount
			if fetchCount > 100 {
				fetchCount = 100
			}
			comments, err := client.GetComments(ctx, issue.IssueKey.Value, &api.CommentListOptions{
				Count: fetchCount,
				Order: "desc",
			})
			if err == nil {
				for i := len(comments) - 1; i >= 0; i-- {
					if comments[i].Content != "" {
						fullText += "\n" + comments[i].Content
					}
				}
			}
		}

		if strings.TrimSpace(fullText) == "" {
			return "-"
		}

		s, err := summary.Summarize(fullText, 1)
		if err != nil {
			return ""
		}
		// 改行を除去
		s = strings.ReplaceAll(s, "\n", " ")
		// 長すぎる場合は省略（テーブル表示のため）
		runes := []rune(s)
		if len(runes) > 50 {
			return string(runes[:50]) + "..."
		}
		return s
	case "type":
		if issue.IssueType.IsSet() && issue.IssueType.Value.Name.IsSet() {
			return issue.IssueType.Value.Name.Value
		}
		return "-"
	case "created":
		return f.FormatDateTime(issue.Created.Value, field)
	case "updated":
		return f.FormatDateTime(issue.Updated.Value, field)
	case "created_user":
		if issue.CreatedUser.IsSet() && issue.CreatedUser.Value.Name.IsSet() {
			return f.FormatString(issue.CreatedUser.Value.Name.Value, field)
		}
		return "-"
	case "due_date":
		if issue.DueDate.IsSet() && !issue.DueDate.IsNull() {
			return f.FormatDate(issue.DueDate.Value, field)
		}
		return "-"
	case "start_date":
		if issue.StartDate.IsSet() && !issue.StartDate.IsNull() {
			return f.FormatDate(issue.StartDate.Value, field)
		}
		return "-"
	case "category":
		if len(issue.Category) > 0 {
			names := make([]string, len(issue.Category))
			for i, c := range issue.Category {
				if c.Name.IsSet() {
					names[i] = c.Name.Value
				}
			}
			return f.FormatString(strings.Join(names, ", "), field)
		}
		return "-"
	case "milestone":
		if len(issue.Milestone) > 0 {
			names := make([]string, len(issue.Milestone))
			for i, m := range issue.Milestone {
				if m.Name.IsSet() {
					names[i] = m.Name.Value
				}
			}
			return f.FormatString(strings.Join(names, ", "), field)
		}
		return "-"
	case "version":
		if len(issue.Versions) > 0 {
			names := make([]string, len(issue.Versions))
			for i, v := range issue.Versions {
				if v.Name.IsSet() {
					names[i] = v.Name.Value
				}
			}
			return f.FormatString(strings.Join(names, ", "), field)
		}
		return "-"
	case "url":
		return fmt.Sprintf("%s/view/%s", baseURL, issue.IssueKey.Value)
	default:
		return "-"
	}
}
