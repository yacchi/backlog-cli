package status

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

var StatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show your status summary",
	Long: `Show a summary of your notifications, watched issues, and assigned issues.

Similar to 'gh status' for GitHub.

Examples:
  backlog status
  backlog status -o json`,
	RunE: runStatus,
}

// StatusSummary はステータスサマリー
type StatusSummary struct {
	UnreadNotifications int                    `json:"unreadNotifications"`
	Notifications       []api.UserNotification `json:"notifications,omitempty"`
	WatchingCount       int                    `json:"watchingCount"`
	Watchings           []api.Watching         `json:"watchings,omitempty"`
	AssignedIssuesCount int                    `json:"assignedIssuesCount"`
	AssignedIssues      []AssignedIssue        `json:"assignedIssues,omitempty"`
}

// AssignedIssue は担当課題
type AssignedIssue struct {
	IssueKey string `json:"issueKey"`
	Summary  string `json:"summary"`
	Status   string `json:"status"`
}

func runStatus(c *cobra.Command, args []string) error {
	client, cfg, err := cmdutil.GetAPIClient(c)
	if err != nil {
		return err
	}

	profile := cfg.CurrentProfile()
	display := cfg.Display()
	ctx := c.Context()

	// 自分のユーザーID取得
	myself, err := client.GetCurrentUser(ctx)
	if err != nil {
		return fmt.Errorf("failed to get current user: %w", err)
	}

	summary := &StatusSummary{}

	// 通知取得
	unreadCount, err := client.GetNotificationsCount(ctx)
	if err != nil {
		return fmt.Errorf("failed to get notification count: %w", err)
	}
	summary.UnreadNotifications = unreadCount

	notifications, err := client.GetNotifications(ctx, &api.NotificationListOptions{
		Count: 5,
		Order: "desc",
	})
	if err != nil {
		return fmt.Errorf("failed to get notifications: %w", err)
	}
	// 未読のみ抽出
	for _, n := range notifications {
		if !n.AlreadyRead {
			summary.Notifications = append(summary.Notifications, n)
		}
	}

	// ウォッチ取得
	watchingCount, err := client.GetWatchingCount(ctx, myself.ID.Value, nil)
	if err != nil {
		return fmt.Errorf("failed to get watching count: %w", err)
	}
	summary.WatchingCount = watchingCount

	watchings, err := client.GetWatchingList(ctx, myself.ID.Value, &api.WatchingListOptions{
		Count: 5,
		Order: "desc",
		Sort:  "issueUpdated",
	})
	if err != nil {
		return fmt.Errorf("failed to get watchings: %w", err)
	}
	summary.Watchings = watchings

	// 担当課題取得（プロジェクト指定がある場合のみ）
	if projectKey := cmdutil.GetCurrentProject(cfg); projectKey != "" {
		// プロジェクトIDを取得
		project, err := client.GetProject(ctx, projectKey)
		if err == nil {
			issues, err := client.GetIssues(ctx, &api.IssueListOptions{
				ProjectIDs:  []int{project.ID},
				AssigneeIDs: []int{myself.ID.Value},
				StatusIDs:   []int{1, 2, 3}, // Open, In Progress, Resolved
				Count:       5,
			})
			if err == nil {
				summary.AssignedIssuesCount = len(issues)
				for _, issue := range issues {
					var statusName string
					if issue.Status.IsSet() && issue.Status.Value.Name.IsSet() {
						statusName = issue.Status.Value.Name.Value
					}
					summary.AssignedIssues = append(summary.AssignedIssues, AssignedIssue{
						IssueKey: issue.IssueKey.Value,
						Summary:  issue.Summary.Value,
						Status:   statusName,
					})
				}
			}
		}
	}

	// 出力
	switch profile.Output {
	case "json":
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(summary)
	default:
		return renderStatus(summary, profile, display)
	}
}

func renderStatus(summary *StatusSummary, profile *config.ResolvedProfile, display *config.ResolvedDisplay) error {
	ui.SetHyperlinkEnabled(display.Hyperlink)

	// 通知セクション
	fmt.Printf("%s (%d unread)\n", ui.Bold("Notifications"), summary.UnreadNotifications)
	if len(summary.Notifications) == 0 {
		fmt.Println("  No unread notifications")
	} else {
		for _, n := range summary.Notifications {
			if n.Issue != nil {
				url := fmt.Sprintf("https://%s.%s/view/%s",
					profile.Space, profile.Domain, n.Issue.IssueKey)
				fmt.Printf("  %s %s\n",
					ui.Hyperlink(url, ui.Cyan(n.Issue.IssueKey)),
					truncate(n.Issue.Summary, 50))
			}
		}
	}
	fmt.Println()

	// ウォッチセクション
	fmt.Printf("%s (%d items)\n", ui.Bold("Watching"), summary.WatchingCount)
	if len(summary.Watchings) == 0 {
		fmt.Println("  No watched issues")
	} else {
		for _, w := range summary.Watchings {
			marker := " "
			if !w.ResourceAlreadyRead {
				marker = ui.Yellow("●")
			}
			url := fmt.Sprintf("https://%s.%s/view/%s",
				profile.Space, profile.Domain, w.Issue.IssueKey)
			fmt.Printf(" %s %s %s\n",
				marker,
				ui.Hyperlink(url, ui.Cyan(w.Issue.IssueKey)),
				truncate(w.Issue.Summary, 50))
		}
	}
	fmt.Println()

	// 担当課題セクション
	if len(summary.AssignedIssues) > 0 {
		fmt.Printf("%s (%d issues)\n", ui.Bold("Assigned to me"), summary.AssignedIssuesCount)
		for _, issue := range summary.AssignedIssues {
			url := fmt.Sprintf("https://%s.%s/view/%s",
				profile.Space, profile.Domain, issue.IssueKey)
			statusColor := formatStatus(issue.Status)
			fmt.Printf("  %s %s %s\n",
				ui.Hyperlink(url, ui.Cyan(issue.IssueKey)),
				statusColor,
				truncate(issue.Summary, 40))
		}
	}

	return nil
}

func formatStatus(status string) string {
	switch status {
	case "Open", "未対応":
		return ui.Yellow("[Open]")
	case "In Progress", "処理中":
		return ui.Blue("[In Progress]")
	case "Resolved", "処理済み":
		return ui.Green("[Resolved]")
	case "Closed", "完了":
		return ui.Gray("[Closed]")
	default:
		return fmt.Sprintf("[%s]", status)
	}
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-3] + "..."
}
