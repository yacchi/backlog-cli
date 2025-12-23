package issue

import (
	"context"
	"fmt"
	"io"
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

var viewCmd = &cobra.Command{
	Use:   "view <issue-key>",
	Short: "View an issue",
	Long: `View detailed information about an issue.

If project is configured, you can omit the project prefix:
  backlog issue view 123       # equivalent to PROJ-123 when project=PROJ

Examples:
  backlog issue view PROJ-123
  backlog issue view 123       # uses configured project
  backlog issue view PROJ-123 --comments
  backlog issue view PROJ-123 --summary`,
	Args: cobra.ExactArgs(1),
	RunE: runView,
}

var (
	viewComments            bool
	viewWeb                 bool
	viewSummary             bool
	viewSummaryWithComments bool
	viewSummaryCommentCount int
	viewBrief               bool
	viewMarkdown            bool
	viewRaw                 bool
	viewMarkdownWarn        bool
	viewMarkdownCache       bool
)

func init() {
	viewCmd.Flags().BoolVarP(&viewComments, "comments", "c", false, "Show comments")
	viewCmd.Flags().BoolVarP(&viewWeb, "web", "w", false, "Open in browser")
	viewCmd.Flags().BoolVar(&viewSummary, "summary", false, "Show AI summary (description only)")
	viewCmd.Flags().BoolVar(&viewSummaryWithComments, "summary-with-comments", false, "Include comments in AI summary")
	viewCmd.Flags().IntVar(&viewSummaryCommentCount, "summary-comment-count", -1, "Number of comments to use for summary")
	viewCmd.Flags().BoolVar(&viewBrief, "brief", false, "Show brief summary (key, summary, status, assignee, URL)")
	viewCmd.Flags().BoolVar(&viewMarkdown, "markdown", false, "Render markdown by converting Backlog notation to GFM")
	viewCmd.Flags().BoolVar(&viewRaw, "raw", false, "Render raw content without markdown conversion")
	viewCmd.Flags().BoolVar(&viewMarkdownWarn, "markdown-warn", false, "Show markdown conversion warnings")
	viewCmd.Flags().BoolVar(&viewMarkdownCache, "markdown-cache", false, "Cache markdown conversion analysis data")
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
	ctx := c.Context()
	issue, err := client.GetIssue(ctx, issueKey)
	if err != nil {
		return fmt.Errorf("failed to get issue: %w", err)
	}

	// 出力
	switch profile.Output {
	case "json":
		if viewBrief {
			return outputBriefJSON(issue, profile)
		}
		return cmdutil.OutputJSONFromProfile(issue, profile)
	default:
		if viewBrief {
			return renderIssueBrief(issue, profile)
		}
		cacheDir, cacheErr := cfg.GetCacheDir()
		markdownOpts := cmdutil.ResolveMarkdownViewOptions(c, display, cacheDir)
		if markdownOpts.Cache && cacheErr != nil {
			return fmt.Errorf("failed to resolve cache dir: %w", cacheErr)
		}
		projectKey := cmdutil.GetCurrentProject(cfg)
		return renderIssueDetail(ctx, client, issue, profile, display, projectKey, markdownOpts, c.OutOrStdout())
	}
}

// renderIssueBrief displays a brief summary of the issue
func renderIssueBrief(issue *backlog.Issue, profile *config.ResolvedProfile) error {
	key := issue.IssueKey.Value
	summary := issue.Summary.Value

	status := "-"
	if issue.Status.IsSet() && issue.Status.Value.Name.IsSet() {
		status = issue.Status.Value.Name.Value
	}

	assignee := "(unassigned)"
	if issue.Assignee.IsSet() && issue.Assignee.Value.Name.IsSet() {
		assignee = issue.Assignee.Value.Name.Value
	}

	url := fmt.Sprintf("https://%s.%s/view/%s", profile.Space, profile.Domain, key)

	fmt.Printf("%s: %s\n", key, summary)
	fmt.Printf("Status: %s | Assignee: %s\n", ui.StatusColor(status), assignee)
	fmt.Printf("URL: %s\n", url)

	return nil
}

// outputBriefJSON outputs a brief JSON representation of the issue
func outputBriefJSON(issue *backlog.Issue, profile *config.ResolvedProfile) error {
	key := issue.IssueKey.Value

	status := ""
	if issue.Status.IsSet() && issue.Status.Value.Name.IsSet() {
		status = issue.Status.Value.Name.Value
	}

	assignee := ""
	if issue.Assignee.IsSet() && issue.Assignee.Value.Name.IsSet() {
		assignee = issue.Assignee.Value.Name.Value
	}

	url := fmt.Sprintf("https://%s.%s/view/%s", profile.Space, profile.Domain, key)

	brief := map[string]string{
		"key":      key,
		"summary":  issue.Summary.Value,
		"status":   status,
		"assignee": assignee,
		"url":      url,
	}

	return cmdutil.OutputJSONFromProfile(brief, profile)
}

func renderIssueDetail(ctx context.Context, client *api.Client, issue *backlog.Issue, profile *config.ResolvedProfile, display *config.ResolvedDisplay, projectKey string, markdownOpts cmdutil.MarkdownViewOptions, out io.Writer) error {
	// フラグの調整: summary-with-comments が指定されたら summary も有効にする
	if viewSummaryWithComments {
		viewSummary = true
	}

	// ハイパーリンク設定
	ui.SetHyperlinkEnabled(display.Hyperlink)

	// フィールドフォーマッター
	formatter := ui.NewFieldFormatter(display.Timezone, display.DateTimeFormat, display.IssueFieldConfig)

	// URL生成
	key := issue.IssueKey.Value
	issueID := 0
	if issue.ID.IsSet() {
		issueID = issue.ID.Value
	}
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

	// コメント取得条件の決定
	summaryCommentCount := display.SummaryCommentCount
	if viewSummaryCommentCount >= 0 {
		summaryCommentCount = viewSummaryCommentCount
	}

	fetchCount := 0
	if viewComments {
		fetchCount = display.DefaultCommentCount
		if fetchCount == 0 {
			fetchCount = 10 // fallback
		}
	}
	// viewSummaryWithComments が有効な場合のみ、要約用のコメント取得を考慮する
	if viewSummaryWithComments && summaryCommentCount > 0 {
		if summaryCommentCount > fetchCount {
			fetchCount = summaryCommentCount
		}
	}

	// API上限(100)を超えないように制限
	if fetchCount > 100 {
		fetchCount = 100
	}

	// コメント取得
	var comments []api.Comment
	if fetchCount > 0 {
		comments, _ = client.GetComments(ctx, key, &api.CommentListOptions{
			Count: fetchCount,
			Order: "desc",
		})
		// コメント取得失敗は致命的ではないとする
	}

	// AI要約
	if viewSummary {
		fmt.Println()
		fmt.Println(ui.Bold("AI Summary"))
		fmt.Println(strings.Repeat("─", 60))

		fullText := ""
		if issue.Description.IsSet() {
			fullText += issue.Description.Value + "\n"
		}

		// 要約に使用するコメントを抽出
		// viewSummaryWithComments が有効な場合のみコメントを含める
		if viewSummaryWithComments {
			// APIからは新しい順(desc)で取得している
			// 古い順に結合したいので逆順にループ
			for i := len(comments) - 1; i >= 0; i-- {
				// summaryCommentCount の制限チェック
				if i >= summaryCommentCount {
					continue
				}

				if comments[i].Content != "" {
					fullText += comments[i].Content + "\n"
				}
			}
		}

		s, err := summary.Summarize(fullText, 3)
		if err != nil {
			fmt.Printf("Failed to generate summary: %v\n", err)
		} else if s != "" {
			fmt.Println(s)
		} else {
			fmt.Println(ui.Gray("(No summary available)"))
		}
	}

	// 説明
	if issue.Description.IsSet() && issue.Description.Value != "" {
		fmt.Println()
		fmt.Println(ui.Bold("Description"))
		fmt.Println(strings.Repeat("─", 60))
		content := issue.Description.Value
		if markdownOpts.Enable {
			rendered, err := cmdutil.RenderMarkdownContent(content, markdownOpts, "issue", issueID, 0, projectKey, key, issueURL, out)
			if err != nil {
				return err
			}
			content = rendered
		}
		fmt.Println(content)
	}

	// URL（常に表示、ハイパーリンク化）
	fmt.Println()
	fmt.Printf("URL: %s\n", ui.Hyperlink(issueURL, ui.Cyan(issueURL)))

	// コメント
	if viewComments && len(comments) > 0 {
		fmt.Println()
		fmt.Println(ui.Bold("Recent Comments"))
		fmt.Println(strings.Repeat("─", 60))

		// 表示件数はオプションで制御されていないが、APIで20件取ってきているのでそれを表示
		// 元のコードは10件固定だったが、要約のために20件にしたので、表示も20件になる
		for _, comment := range comments {
			fmt.Printf("\n%s %s\n", ui.Bold(comment.CreatedUser.Name), ui.Gray(formatter.FormatDateTime(comment.Created, "created")))
			content := comment.Content
			if markdownOpts.Enable {
				commentURL := fmt.Sprintf("%s#comment-%d", issueURL, comment.ID)
				rendered, err := cmdutil.RenderMarkdownContent(content, markdownOpts, "comment", comment.ID, issueID, projectKey, key, commentURL, out)
				if err != nil {
					return err
				}
				content = rendered
			}
			fmt.Println(content)
		}
	}

	return nil
}
