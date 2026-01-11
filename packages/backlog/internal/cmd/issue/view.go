package issue

import (
	"context"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/pkg/browser"
	"github.com/spf13/cobra"
	"github.com/yacchi/backlog-cli/packages/backlog/internal/api"
	"github.com/yacchi/backlog-cli/packages/backlog/internal/cmdutil"
	"github.com/yacchi/backlog-cli/packages/backlog/internal/config"
	"github.com/yacchi/backlog-cli/packages/backlog/internal/gen/backlog"
	"github.com/yacchi/backlog-cli/packages/backlog/internal/summary"
	"github.com/yacchi/backlog-cli/packages/backlog/internal/ui"
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
  backlog issue view PROJ-123 -c           # show comments (default count)
  backlog issue view PROJ-123 -c=50        # show 50 comments
  backlog issue view PROJ-123 -c=all       # show all comments
  backlog issue view PROJ-123 --summary`,
	Args: cobra.ExactArgs(1),
	RunE: runView,
}

var (
	viewComments            string // empty: no comments, "default": use default count, number: specific count, "all": fetch all
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
	viewCmd.Flags().StringVarP(&viewComments, "comments", "c", "", "Show comments: -c (default count), -c=N (N comments), -c=all (all comments)")
	viewCmd.Flags().Lookup("comments").NoOptDefVal = "default"
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

	// コメント取得条件の決定
	showComments := viewComments != ""
	fetchAll := false
	fetchCount := 0

	if showComments {
		switch viewComments {
		case "default":
			fetchCount = display.DefaultCommentCount
			if fetchCount == 0 {
				fetchCount = 10 // fallback
			}
		case "all", "0":
			fetchAll = true
		default:
			// 数値として解析
			if n, err := strconv.Atoi(viewComments); err == nil && n > 0 {
				fetchCount = n
			} else {
				// 不正な値の場合はデフォルト
				fetchCount = display.DefaultCommentCount
				if fetchCount == 0 {
					fetchCount = 10
				}
			}
		}
	}

	// コメント取得
	var comments []api.Comment
	if fetchAll {
		comments, _ = fetchAllComments(ctx, client, issueKey)
	} else if fetchCount > 0 {
		if fetchCount > 100 {
			fetchCount = 100
		}
		comments, _ = client.GetComments(ctx, issueKey, &api.CommentListOptions{
			Count: fetchCount,
			Order: "desc",
		})
	}

	// 出力
	switch profile.Output {
	case "json":
		if viewBrief {
			return outputBriefJSON(issue, profile)
		}
		return outputIssueJSON(issue, comments, showComments, profile)
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
		return renderIssueDetail(issue, comments, showComments, profile, display, projectKey, markdownOpts, c.OutOrStdout())
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

func issueAttachmentNames(attachments []backlog.Attachment) []string {
	if len(attachments) == 0 {
		return nil
	}
	names := make([]string, 0, len(attachments))
	for _, attachment := range attachments {
		if attachment.Name.IsSet() && attachment.Name.Value != "" {
			names = append(names, attachment.Name.Value)
		}
	}
	return names
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

	return cmdutil.OutputJSONFromProfile(brief, profile.JSONFields, profile.JQ)
}

// IssueWithComments is a wrapper for issue with comments for JSON output
type IssueWithComments struct {
	Issue    *backlog.Issue `json:"issue"`
	Comments []api.Comment  `json:"comments,omitempty"`
}

// outputIssueJSON outputs issue with optional comments as JSON
func outputIssueJSON(issue *backlog.Issue, comments []api.Comment, showComments bool, profile *config.ResolvedProfile) error {
	if showComments {
		return cmdutil.OutputJSONFromProfile(IssueWithComments{
			Issue:    issue,
			Comments: comments,
		}, profile.JSONFields, profile.JQ)
	}
	return cmdutil.OutputJSONFromProfile(issue, profile.JSONFields, profile.JQ)
}

func renderIssueDetail(issue *backlog.Issue, comments []api.Comment, showComments bool, profile *config.ResolvedProfile, display *config.ResolvedDisplay, projectKey string, markdownOpts cmdutil.MarkdownViewOptions, out io.Writer) error {
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

	// AI要約用のコメント数設定
	summaryCommentCount := display.SummaryCommentCount
	if viewSummaryCommentCount >= 0 {
		summaryCommentCount = viewSummaryCommentCount
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
			attachments := issueAttachmentNames(issue.Attachments)
			rendered, err := cmdutil.RenderMarkdownContent(content, markdownOpts, "issue", issueID, 0, projectKey, key, issueURL, attachments, out)
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
	if showComments && len(comments) > 0 {
		fmt.Println()
		fmt.Println(ui.Bold("Comments"))
		fmt.Println(strings.Repeat("─", 60))

		// 表示件数はオプションで制御されていないが、APIで20件取ってきているのでそれを表示
		// 元のコードは10件固定だったが、要約のために20件にしたので、表示も20件になる
		for _, comment := range comments {
			fmt.Printf("\n%s %s\n", ui.Bold(comment.CreatedUser.Name), ui.Gray(formatter.FormatDateTime(comment.Created, "created")))
			content := comment.Content
			if markdownOpts.Enable {
				commentURL := fmt.Sprintf("%s#comment-%d", issueURL, comment.ID)
				rendered, err := cmdutil.RenderMarkdownContent(content, markdownOpts, "comment", comment.ID, issueID, projectKey, key, commentURL, nil, out)
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

// fetchAllComments は課題の全コメントをページネーションで取得する
func fetchAllComments(ctx context.Context, client *api.Client, issueKey string) ([]api.Comment, error) {
	const batchSize = 100
	var allComments []api.Comment
	maxID := 0

	for {
		opts := &api.CommentListOptions{
			Count: batchSize,
			Order: "desc",
		}
		if maxID > 0 {
			opts.MaxID = maxID
		}

		batch, err := client.GetComments(ctx, issueKey, opts)
		if err != nil {
			return allComments, err
		}

		if len(batch) == 0 {
			break
		}

		allComments = append(allComments, batch...)

		// 次のページ用に最小のIDを取得
		lastComment := batch[len(batch)-1]
		maxID = lastComment.ID

		// 取得件数がbatchSizeより少なければ、これ以上ない
		if len(batch) < batchSize {
			break
		}
	}

	return allComments, nil
}
