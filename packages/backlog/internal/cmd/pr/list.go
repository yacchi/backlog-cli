package pr

import (
	"fmt"
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

var listCmd = &cobra.Command{
	Use:     "list",
	Aliases: []string{"ls"},
	Short:   "List pull requests",
	Long: `List pull requests in a repository.

Examples:
  # List open pull requests (default)
  backlog pr list --repo myrepo
  backlog pr ls -r myrepo

  # Filter by state
  backlog pr list --repo myrepo --state closed
  backlog pr list --repo myrepo --state merged
  backlog pr list --repo myrepo --state all

  # Open PR list in browser
  backlog pr list --repo myrepo --web`,
	RunE: runList,
}

var (
	listRepo     string
	listState    string
	listLimit    int
	listWeb      bool
	listCount    bool
	listAuthor   string
	listAssignee string
)

func init() {
	listCmd.Flags().StringVarP(&listRepo, "repo", "R", "", "Repository name (required)")
	listCmd.Flags().StringVarP(&listState, "state", "s", "open", "Filter by state: {open|closed|merged|all}")
	listCmd.Flags().IntVarP(&listLimit, "limit", "L", 30, "Maximum number of pull requests to fetch")
	listCmd.Flags().BoolVarP(&listWeb, "web", "w", false, "Open pull request list in browser")
	listCmd.Flags().BoolVar(&listCount, "count", false, "Show only the count of pull requests")
	listCmd.Flags().StringVarP(&listAuthor, "author", "A", "", "Filter by author (user ID or @me)")
	listCmd.Flags().StringVarP(&listAssignee, "assignee", "a", "", "Filter by assignee (user ID or @me)")
	_ = listCmd.MarkFlagRequired("repo")
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

	// ブラウザで開く
	if listWeb {
		url := fmt.Sprintf("https://%s.%s/git/%s/%s/pullRequests",
			profile.Space, profile.Domain, projectKey, listRepo)
		return browser.OpenURL(url)
	}

	opts := &api.PRListOptions{
		Count: listLimit,
	}

	// ステータスフィルター
	switch listState {
	case "open":
		opts.StatusIDs = []int{1}
	case "closed":
		opts.StatusIDs = []int{2}
	case "merged":
		opts.StatusIDs = []int{3}
	case "all":
		// フィルターなし
	default:
		return fmt.Errorf("invalid state: %s (must be open, closed, merged, or all)", listState)
	}

	ctx := c.Context()

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

	// 担当者フィルター
	if listAssignee == "@me" {
		me, err := client.GetCurrentUser(ctx)
		if err != nil {
			return fmt.Errorf("failed to get current user: %w", err)
		}
		opts.AssigneeIDs = []int{me.ID.Value}
	} else if listAssignee != "" {
		assigneeID, err := strconv.Atoi(listAssignee)
		if err != nil {
			return fmt.Errorf("invalid assignee ID: %s", listAssignee)
		}
		opts.AssigneeIDs = []int{assigneeID}
	}

	// 件数のみ表示
	if listCount {
		count, err := client.GetPullRequestsCount(ctx, projectKey, listRepo, opts)
		if err != nil {
			return fmt.Errorf("failed to get pull request count: %w", err)
		}
		fmt.Println(count)
		return nil
	}

	prs, err := client.GetPullRequests(ctx, projectKey, listRepo, opts)
	if err != nil {
		return fmt.Errorf("failed to get pull requests: %w", err)
	}

	if len(prs) == 0 {
		fmt.Println("No pull requests found")
		return nil
	}

	// 出力
	display := cfg.Display()
	switch profile.Output {
	case "json":
		return cmdutil.OutputJSONFromProfile(prs, profile.JSONFields, profile.JQ)
	default:
		outputPRTable(prs, profile, display, projectKey, listRepo)
		return nil
	}
}

func outputPRTable(prs []api.PullRequest, profile *config.ResolvedProfile, display *config.ResolvedDisplay, projectKey, repo string) {
	fields := display.PRListFields
	fieldConfig := display.PRFieldConfig

	// ハイパーリンク設定
	ui.SetHyperlinkEnabled(display.Hyperlink)

	// ヘッダー生成
	headers := make([]string, len(fields))
	for i, f := range fields {
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
	baseURL := fmt.Sprintf("https://%s.%s/git/%s/%s/pullRequests",
		profile.Space, profile.Domain, projectKey, repo)

	for _, pr := range prs {
		row := make([]string, len(fields))
		for i, f := range fields {
			row[i] = getPRFieldValue(pr, f, formatter, baseURL)
		}
		table.AddRow(row...)
	}

	table.RenderWithColor(os.Stdout, ui.IsColorEnabled())
}

func getPRFieldValue(pr api.PullRequest, field string, f *ui.FieldFormatter, baseURL string) string {
	switch field {
	case "number":
		url := fmt.Sprintf("%s/%d", baseURL, pr.Number)
		return ui.Hyperlink(url, fmt.Sprintf("%d", pr.Number))
	case "status":
		switch pr.Status.ID {
		case 1:
			return ui.Green("Open")
		case 2:
			return ui.Red("Closed")
		case 3:
			return ui.Blue("Merged")
		default:
			return pr.Status.Name
		}
	case "author":
		return f.FormatString(pr.CreatedUser.Name, field)
	case "branch":
		return f.FormatString(pr.Branch, field)
	case "summary":
		return f.FormatString(pr.Summary, field)
	case "base":
		return f.FormatString(pr.Base, field)
	case "created":
		return f.FormatDateTime(pr.Created, field)
	case "updated":
		return f.FormatDateTime(pr.Updated, field)
	case "url":
		return fmt.Sprintf("%s/%d", baseURL, pr.Number)
	default:
		return "-"
	}
}
