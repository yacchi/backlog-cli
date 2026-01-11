package pr

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/pkg/browser"
	"github.com/spf13/cobra"
	"github.com/yacchi/backlog-cli/packages/backlog/internal/api"
	"github.com/yacchi/backlog-cli/packages/backlog/internal/cmdutil"
	"github.com/yacchi/backlog-cli/packages/backlog/internal/config"
	"github.com/yacchi/backlog-cli/packages/backlog/internal/ui"
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
	viewRepo          string
	viewWeb           bool
	viewComments      bool
	viewMarkdown      bool
	viewRaw           bool
	viewMarkdownWarn  bool
	viewMarkdownCache bool
)

func init() {
	viewCmd.Flags().StringVarP(&viewRepo, "repo", "R", "", "Repository name (required)")
	viewCmd.Flags().BoolVarP(&viewWeb, "web", "w", false, "Open in browser")
	viewCmd.Flags().BoolVarP(&viewComments, "comments", "c", false, "View comments")
	viewCmd.Flags().BoolVar(&viewMarkdown, "markdown", false, "Render markdown by converting Backlog notation to GFM")
	viewCmd.Flags().BoolVar(&viewRaw, "raw", false, "Render raw content without markdown conversion")
	viewCmd.Flags().BoolVar(&viewMarkdownWarn, "markdown-warn", false, "Show markdown conversion warnings")
	viewCmd.Flags().BoolVar(&viewMarkdownCache, "markdown-cache", false, "Cache markdown conversion analysis data")
	_ = viewCmd.MarkFlagRequired("repo")
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

	ctx := c.Context()

	// PR取得
	pr, err := client.GetPullRequest(ctx, projectKey, viewRepo, number)
	if err != nil {
		return fmt.Errorf("failed to get pull request: %w", err)
	}

	// コメント取得（オプション指定時）
	var comments []api.PRComment
	if viewComments {
		comments, err = client.GetPullRequestComments(ctx, projectKey, viewRepo, number, &api.PRCommentListOptions{
			Count: 100,
			Order: "asc",
		})
		if err != nil {
			return fmt.Errorf("failed to get pull request comments: %w", err)
		}
	}

	// 出力
	switch profile.Output {
	case "json":
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if viewComments {
			// コメント付きでJSON出力
			return enc.Encode(struct {
				*api.PullRequest
				Comments []api.PRComment `json:"comments"`
			}{pr, comments})
		}
		return enc.Encode(pr)
	default:
		cacheDir, cacheErr := cfg.GetCacheDir()
		markdownOpts := cmdutil.ResolveMarkdownViewOptions(c, display, cacheDir)
		if markdownOpts.Cache && cacheErr != nil {
			return fmt.Errorf("failed to resolve cache dir: %w", cacheErr)
		}
		return renderPRDetail(pr, comments, profile, display, projectKey, markdownOpts, c.OutOrStdout())
	}
}

func renderPRDetail(pr *api.PullRequest, comments []api.PRComment, profile *config.ResolvedProfile, display *config.ResolvedDisplay, projectKey string, markdownOpts cmdutil.MarkdownViewOptions, out io.Writer) error {
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
		content := pr.Description
		if markdownOpts.Enable {
			rendered, err := cmdutil.RenderMarkdownContent(content, markdownOpts, "pr", pr.Number, 0, projectKey, fmt.Sprintf("#%d", pr.Number), prURL, nil, out)
			if err != nil {
				return err
			}
			content = rendered
		}
		fmt.Println(content)
	}

	// コメント表示
	if len(comments) > 0 {
		fmt.Println()
		fmt.Println(ui.Bold(fmt.Sprintf("Comments (%d)", len(comments))))
		fmt.Println(strings.Repeat("─", 60))
		for i, comment := range comments {
			if i > 0 {
				fmt.Println()
			}
			// コメントヘッダー
			fmt.Printf("%s - %s\n", ui.Bold(comment.CreatedUser.Name), ui.Gray(formatter.FormatDateTime(comment.Created, "created")))
			// コメント内容
			if comment.Content != "" {
				content := comment.Content
				if markdownOpts.Enable {
					rendered, err := cmdutil.RenderMarkdownContent(content, markdownOpts, "pr_comment", pr.Number, comment.ID, projectKey, fmt.Sprintf("#%d", pr.Number), prURL, nil, out)
					if err != nil {
						return err
					}
					content = rendered
				}
				fmt.Println(content)
			}
			// 変更ログがある場合は表示
			for _, cl := range comment.ChangeLog {
				if cl.Field != "" {
					if cl.OriginalValue != "" {
						fmt.Printf("  %s: %s -> %s\n", ui.Gray(cl.Field), cl.OriginalValue, cl.NewValue)
					} else {
						fmt.Printf("  %s: %s\n", ui.Gray(cl.Field), cl.NewValue)
					}
				}
			}
		}
	}

	// URL（常に表示、ハイパーリンク化）
	fmt.Println()
	fmt.Printf("URL: %s\n", ui.Hyperlink(prURL, ui.Cyan(prURL)))

	return nil
}
