package watching

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/yacchi/backlog-cli/packages/backlog/internal/api"
	"github.com/yacchi/backlog-cli/packages/backlog/internal/cmdutil"
	"github.com/yacchi/backlog-cli/packages/backlog/internal/config"
	"github.com/yacchi/backlog-cli/packages/backlog/internal/ui"
)

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List watched issues",
	Long: `List issues you are watching.

Examples:
  backlog watching list
  backlog watching list -L 50
  backlog watching list --unread
  backlog watching list --sort created --order asc
  backlog watching list --issue PROJ-123`,
	RunE: runList,
}

var (
	listLimit  int
	listSort   string
	listOrder  string
	listUnread bool
	listIssue  string
)

func init() {
	listCmd.Flags().IntVarP(&listLimit, "limit", "L", 20, "Maximum number of items to fetch")
	listCmd.Flags().StringVar(&listSort, "sort", "issueUpdated", "Sort field: created, updated, or issueUpdated")
	listCmd.Flags().StringVar(&listOrder, "order", "desc", "Sort order: asc or desc")
	listCmd.Flags().BoolVar(&listUnread, "unread", false, "Show only items with unread updates")
	listCmd.Flags().StringVar(&listIssue, "issue", "", "Filter by issue IDs or keys (comma-separated)")
}

func runList(c *cobra.Command, args []string) error {
	client, cfg, err := cmdutil.GetAPIClient(c)
	if err != nil {
		return err
	}

	profile := cfg.CurrentProfile()
	display := cfg.Display()
	ctx := c.Context()

	// 自分のユーザーIDを取得
	myself, err := client.GetCurrentUser(ctx)
	if err != nil {
		return fmt.Errorf("failed to get current user: %w", err)
	}

	// ウォッチ一覧取得
	opts := &api.WatchingListOptions{
		Count: listLimit,
		Order: listOrder,
		Sort:  listSort,
	}

	// 未読のみ
	if listUnread {
		unread := false
		opts.ResourceAlreadyRead = &unread
	}

	// 課題フィルター
	if listIssue != "" {
		issueIDs, err := cmdutil.ResolveIssueIDs(ctx, client, listIssue)
		if err != nil {
			return fmt.Errorf("failed to resolve issues: %w", err)
		}
		opts.IssueIDs = issueIDs
	}

	watchings, err := client.GetWatchingList(ctx, myself.ID.Value, opts)
	if err != nil {
		return fmt.Errorf("failed to get watching list: %w", err)
	}

	// 出力
	switch profile.Output {
	case "json":
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(watchings)
	default:
		return renderWatchingList(watchings, profile, display)
	}
}

func renderWatchingList(watchings []api.Watching, profile *config.ResolvedProfile, display *config.ResolvedDisplay) error {
	if len(watchings) == 0 {
		fmt.Println("No watched issues")
		return nil
	}

	// ハイパーリンク設定
	ui.SetHyperlinkEnabled(display.Hyperlink)

	// フィールドフォーマッター
	formatter := ui.NewFieldFormatter(display.Timezone, display.DateTimeFormat, nil)

	for _, w := range watchings {
		// 未読/既読マーカー
		marker := " "
		if !w.ResourceAlreadyRead {
			marker = ui.Yellow("●")
		}

		// URL生成
		issueURL := fmt.Sprintf("https://%s.%s/view/%s",
			profile.Space, profile.Domain, w.Issue.IssueKey)

		// 更新日時
		updated := formatter.FormatDateTime(w.LastContentUpdated, "updated")

		// 出力
		fmt.Printf("%s %s %s %s\n",
			marker,
			ui.Hyperlink(issueURL, ui.Cyan(w.Issue.IssueKey)),
			truncate(w.Issue.Summary, 50),
			ui.Gray(updated))

		// メモ（あれば）
		if w.Note != "" {
			fmt.Printf("    %s %s\n", ui.Gray("Note:"), w.Note)
		}
	}

	return nil
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-3] + "..."
}
