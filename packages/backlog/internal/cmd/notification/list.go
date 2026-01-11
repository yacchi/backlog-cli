package notification

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/yacchi/backlog-cli/packages/backlog/internal/api"
	"github.com/yacchi/backlog-cli/packages/backlog/internal/cmdutil"
	"github.com/yacchi/backlog-cli/packages/backlog/internal/config"
	"github.com/yacchi/backlog-cli/packages/backlog/internal/ui"
)

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List notifications",
	Long: `List your notifications.

Examples:
  backlog notification list
  backlog notification list --unread
  backlog notification list --count 50`,
	RunE: runList,
}

var (
	listCount  int
	listUnread bool
)

func init() {
	listCmd.Flags().IntVarP(&listCount, "count", "c", 20, "Number of notifications to show")
	listCmd.Flags().BoolVar(&listUnread, "unread", false, "Show only unread notifications")
}

func runList(c *cobra.Command, args []string) error {
	client, cfg, err := cmdutil.GetAPIClient(c)
	if err != nil {
		return err
	}

	profile := cfg.CurrentProfile()
	display := cfg.Display()

	// 通知一覧取得
	opts := &api.NotificationListOptions{
		Count: listCount,
		Order: "desc",
	}

	notifications, err := client.GetNotifications(c.Context(), opts)
	if err != nil {
		return fmt.Errorf("failed to get notifications: %w", err)
	}

	// 未読のみフィルタ
	if listUnread {
		var unread []api.UserNotification
		for _, n := range notifications {
			if !n.AlreadyRead {
				unread = append(unread, n)
			}
		}
		notifications = unread
	}

	// 出力
	switch profile.Output {
	case "json":
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(notifications)
	default:
		return renderNotificationList(notifications, profile, display)
	}
}

func renderNotificationList(notifications []api.UserNotification, profile *config.ResolvedProfile, display *config.ResolvedDisplay) error {
	if len(notifications) == 0 {
		fmt.Println("No notifications")
		return nil
	}

	// ハイパーリンク設定
	ui.SetHyperlinkEnabled(display.Hyperlink)

	// フィールドフォーマッター
	formatter := ui.NewFieldFormatter(display.Timezone, display.DateTimeFormat, nil)

	for _, n := range notifications {
		// 既読/未読マーカー
		marker := " "
		if !n.AlreadyRead {
			marker = ui.Yellow("●")
		}

		// 通知理由
		reason := formatReason(n.Reason.ID)

		// ターゲット情報
		var target, targetURL string
		if n.Issue != nil {
			target = fmt.Sprintf("%s %s", n.Issue.IssueKey, truncate(n.Issue.Summary, 40))
			targetURL = fmt.Sprintf("https://%s.%s/view/%s", profile.Space, profile.Domain, n.Issue.IssueKey)
		} else if n.PullRequest != nil {
			target = fmt.Sprintf("PR #%d", n.PullRequest.Number)
			targetURL = "" // PRのURLは project/repo が必要で複雑なためスキップ
		}

		// 日時
		created := formatter.FormatDateTime(n.Created, "created")

		// 出力
		if targetURL != "" {
			fmt.Printf("%s %s %s %s %s\n",
				marker,
				ui.Cyan(fmt.Sprintf("[%d]", n.ID)),
				reason,
				ui.Hyperlink(targetURL, target),
				ui.Gray(created))
		} else {
			fmt.Printf("%s %s %s %s %s\n",
				marker,
				ui.Cyan(fmt.Sprintf("[%d]", n.ID)),
				reason,
				target,
				ui.Gray(created))
		}

		// コメント内容（あれば）
		if n.Comment != nil && n.Comment.Content != "" {
			content := truncate(strings.ReplaceAll(n.Comment.Content, "\n", " "), 60)
			fmt.Printf("    %s\n", ui.Gray(content))
		}
	}

	return nil
}

func formatReason(reasonID int) string {
	switch reasonID {
	case 1:
		return ui.Green("Assigned")
	case 2:
		return ui.Blue("Commented")
	case 3:
		return ui.Yellow("Updated")
	case 4:
		return ui.Cyan("Mentioned")
	case 5:
		return ui.Blue("Watching")
	default:
		return "Notification"
	}
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-3] + "..."
}
