package issue

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/pkg/browser"
	"github.com/spf13/cobra"
	"github.com/yacchi/backlog-cli/packages/backlog/internal/api"
	"github.com/yacchi/backlog-cli/packages/backlog/internal/cmdutil"
	"github.com/yacchi/backlog-cli/packages/backlog/internal/config"
	"github.com/yacchi/backlog-cli/packages/backlog/internal/debug"
	"github.com/yacchi/backlog-cli/packages/backlog/internal/gen/backlog"
	"github.com/yacchi/backlog-cli/packages/backlog/internal/summary"
	"github.com/yacchi/backlog-cli/packages/backlog/internal/ui"
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

  # Filter by issue type
  backlog issue list --type Bug

  # Sort by priority
  backlog issue list --sort priority --order asc

  # Open issue list in browser
  backlog issue list --web`,
	RunE: runList,
}

var (
	listAssignee            string
	listAuthor              string
	listState               string
	listLimit               int
	listSearch              string
	listMine                bool
	listWeb                 bool
	listSummary             bool
	listSummaryWithComments bool
	listSummaryCommentCount int
	listMarkdown            bool
	listRaw                 bool
	listMarkdownWarn        bool
	listMarkdownCache       bool
	listCount               bool
	listCategory            string
	listMilestone           string
	listIssueType           string
	listSort                string
	listOrder               string
)

func init() {
	listCmd.Flags().StringVarP(&listAssignee, "assignee", "a", "", "Filter by assignee (user ID or @me)")
	listCmd.Flags().StringVarP(&listAuthor, "author", "A", "", "Filter by author/creator (user ID or @me)")
	listCmd.Flags().StringVarP(&listState, "state", "s", "open", "Filter by state: {open|closed|all}")
	listCmd.Flags().IntVarP(&listLimit, "limit", "L", 30, "Maximum number of issues to fetch")
	listCmd.Flags().StringVarP(&listSearch, "search", "S", "", "Search issues with keyword")
	listCmd.Flags().BoolVar(&listMine, "mine", false, "Show only my issues")
	listCmd.Flags().BoolVarP(&listWeb, "web", "w", false, "Open issue list in browser")
	listCmd.Flags().BoolVar(&listSummary, "summary", false, "Show AI summary column (description only)")
	listCmd.Flags().BoolVar(&listSummaryWithComments, "summary-with-comments", false, "Include comments in AI summary")
	listCmd.Flags().IntVar(&listSummaryCommentCount, "summary-comment-count", -1, "Number of comments to use for summary")
	listCmd.Flags().BoolVar(&listMarkdown, "markdown", false, "Render markdown by converting Backlog notation to GFM")
	listCmd.Flags().BoolVar(&listRaw, "raw", false, "Render raw content without markdown conversion")
	listCmd.Flags().BoolVar(&listMarkdownWarn, "markdown-warn", false, "Show markdown conversion warnings")
	listCmd.Flags().BoolVar(&listMarkdownCache, "markdown-cache", false, "Cache markdown conversion analysis data")
	listCmd.Flags().BoolVar(&listCount, "count", false, "Show only the count of issues")
	listCmd.Flags().StringVarP(&listCategory, "category", "l", "", "Filter by category IDs or names (comma-separated, like gh --label)")
	listCmd.Flags().StringVarP(&listMilestone, "milestone", "m", "", "Filter by milestone IDs or names (comma-separated)")
	listCmd.Flags().StringVarP(&listIssueType, "type", "T", "", "Filter by issue type name (e.g., Bug, タスク)")
	listCmd.Flags().StringVar(&listSort, "sort", "updated", "Sort field: created, updated, issueType, category, priority, dueDate, etc.")
	listCmd.Flags().StringVar(&listOrder, "order", "desc", "Sort order: asc or desc")
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
		Sort:       listSort,
		Order:      listOrder,
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

	// 作成者フィルター
	if listAuthor == "@me" {
		me, err := client.GetCurrentUser(ctx)
		if err != nil {
			return fmt.Errorf("failed to get current user: %w", err)
		}
		opts.CreatedUserIDs = []int{me.ID.Value}
	} else if listAuthor != "" {
		authorID, err := strconv.Atoi(listAuthor)
		if err != nil {
			return fmt.Errorf("invalid author ID: %s", listAuthor)
		}
		opts.CreatedUserIDs = []int{authorID}
	}

	// カテゴリフィルター（--category オプション、ghの--labelに相当）
	if listCategory != "" {
		categoryIDs, err := resolveCategoryIDs(ctx, client, projectKey, listCategory)
		if err != nil {
			return fmt.Errorf("failed to resolve categories: %w", err)
		}
		opts.CategoryIDs = categoryIDs
	}

	// マイルストーンフィルター（--milestone オプション）
	if listMilestone != "" {
		milestoneIDs, err := resolveMilestoneIDs(ctx, client, projectKey, listMilestone)
		if err != nil {
			return fmt.Errorf("failed to resolve milestones: %w", err)
		}
		opts.MilestoneIDs = milestoneIDs
	}

	// 課題種別フィルター（--type オプション）
	if listIssueType != "" {
		issueTypeIDs, err := resolveIssueTypeIDs(ctx, client, projectKey, listIssueType)
		if err != nil {
			return fmt.Errorf("failed to resolve issue types: %w", err)
		}
		opts.IssueTypeIDs = issueTypeIDs
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

	// 件数のみ表示
	if listCount {
		count, err := client.GetIssuesCount(ctx, opts)
		if err != nil {
			return fmt.Errorf("failed to get issue count: %w", err)
		}
		fmt.Println(count)
		return nil
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
		return cmdutil.OutputJSONFromProfile(issues, profile.JSONFields, profile.JQ)
	default:
		cacheDir, cacheErr := cfg.GetCacheDir()
		markdownOpts := cmdutil.ResolveMarkdownViewOptions(c, display, cacheDir)
		if markdownOpts.Cache && cacheErr != nil {
			return fmt.Errorf("failed to resolve cache dir: %w", cacheErr)
		}
		outputTable(ctx, client, issues, profile, display, cfg, projectKey, markdownOpts)
		return nil
	}
}

func outputTable(ctx context.Context, client *api.Client, issues []backlog.Issue, profile *config.ResolvedProfile, display *config.ResolvedDisplay, cfg *config.Store, projectKey string, markdownOpts cmdutil.MarkdownViewOptions) {
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

	// AI要約を一括取得
	summaryMap := make(map[string]string)
	if listSummary {
		summaryMap = fetchAISummaries(ctx, client, issues, cfg, summaryCommentCount, listSummaryWithComments, projectKey, baseURL, markdownOpts)
	}

	for _, issue := range issues {
		row := make([]string, len(fields))
		for i, f := range fields {
			row[i] = getIssueFieldValue(ctx, client, issue, f, formatter, baseURL, summaryMap, projectKey, markdownOpts)
		}
		table.AddRow(row...)
	}

	table.RenderWithColor(os.Stdout, ui.IsColorEnabled())
}

// fetchAISummaries はAI要約を一括取得する
func fetchAISummaries(ctx context.Context, client *api.Client, issues []backlog.Issue, cfg *config.Store, summaryCommentCount int, withComments bool, projectKey, baseURL string, markdownOpts cmdutil.MarkdownViewOptions) map[string]string {
	aiCfg := cfg.AISummary()
	if !aiCfg.Enabled {
		// AI要約が無効な場合は空のマップを返す
		fmt.Fprintln(os.Stderr, "Warning: AI summary is not enabled. Use 'backlog config set ai_summary.enabled true' to enable.")
		return make(map[string]string)
	}

	debug.Log("AI summary: starting",
		"provider", aiCfg.Provider,
		"issue_count", len(issues),
		"with_comments", withComments,
	)

	// Summarizerを作成
	summarizer, err := summary.NewSummarizer(aiCfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: AI summary unavailable: %v\n", err)
		return make(map[string]string)
	}

	// 課題データを準備
	inputs := make([]summary.IssueInput, 0, len(issues))
	for _, issue := range issues {
		input := summary.IssueInput{
			Key:   issue.IssueKey.Value,
			Title: issue.Summary.Value,
		}

		if issue.Description.IsSet() {
			input.Description = issue.Description.Value
		}

		// コメント取得
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
						input.Comments = append(input.Comments, comments[i].Content)
					}
				}
			}
		}

		// Markdown変換
		if markdownOpts.Enable {
			issueID := 0
			if issue.ID.IsSet() {
				issueID = issue.ID.Value
			}
			issueKey := issue.IssueKey.Value
			issueURL := fmt.Sprintf("%s/view/%s", baseURL, issueKey)
			converted, err := cmdutil.RenderMarkdownContent(input.Description, markdownOpts, "issue", issueID, 0, projectKey, issueKey, issueURL, nil, nil)
			if err == nil {
				input.Description = converted
			}
		}

		inputs = append(inputs, input)
	}

	debug.Log("AI summary: prepared inputs",
		"input_count", len(inputs),
	)

	// 一括要約（進捗表示付き）
	stopProgress := ui.StartProgress("AI要約を生成中...")
	result, err := summarizer.SummarizeBatch(ctx, inputs)
	stopProgress()

	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: AI summary failed: %v\n", err)
		return make(map[string]string)
	}

	debug.Log("AI summary: completed",
		"result_count", len(result),
	)

	return result
}

func getIssueFieldValue(ctx context.Context, client *api.Client, issue backlog.Issue, field string, f *ui.FieldFormatter, baseURL string, summaryMap map[string]string, projectKey string, markdownOpts cmdutil.MarkdownViewOptions) string {
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
		// 事前に取得した要約マップから取得
		s, ok := summaryMap[issue.IssueKey.Value]
		if !ok || s == "" {
			return "-"
		}
		// 長すぎる場合は省略（テーブル表示のため）
		return summary.TruncateSummary(s, 50)
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
