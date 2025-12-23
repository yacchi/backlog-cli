package pr

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/pkg/browser"
	"github.com/spf13/cobra"
	"github.com/yacchi/backlog-cli/internal/api"
	"github.com/yacchi/backlog-cli/internal/cmdutil"
	"github.com/yacchi/backlog-cli/internal/config"
	"github.com/yacchi/backlog-cli/internal/ui"
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
	listRepo  string
	listState string
	listLimit int
	listWeb   bool
)

func init() {
	listCmd.Flags().StringVarP(&listRepo, "repo", "r", "", "Repository name (required)")
	listCmd.Flags().StringVarP(&listState, "state", "s", "open", "Filter by state: {open|closed|merged|all}")
	listCmd.Flags().IntVarP(&listLimit, "limit", "L", 30, "Maximum number of pull requests to fetch")
	listCmd.Flags().BoolVarP(&listWeb, "web", "w", false, "Open pull request list in browser")
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

	prs, err := client.GetPullRequests(c.Context(), projectKey, listRepo, opts)
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
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(prs)
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
