package watching

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/yacchi/backlog-cli/packages/backlog/internal/api"
	"github.com/yacchi/backlog-cli/packages/backlog/internal/cmdutil"
	"github.com/yacchi/backlog-cli/packages/backlog/internal/ui"
)

var removeCmd = &cobra.Command{
	Use:     "remove <issue-key>",
	Aliases: []string{"rm", "delete"},
	Short:   "Remove an issue from watchings",
	Long: `Remove an issue from your watching list.

Examples:
  backlog watching remove PROJ-123`,
	Args: cobra.ExactArgs(1),
	RunE: runRemove,
}

func runRemove(c *cobra.Command, args []string) error {
	client, cfg, err := cmdutil.GetAPIClient(c)
	if err != nil {
		return err
	}

	profile := cfg.CurrentProfile()
	issueKey := args[0]
	ctx := c.Context()

	// 課題キーを解決
	resolvedKey, _ := cmdutil.ResolveIssueKey(issueKey, cmdutil.GetCurrentProject(cfg))

	// 自分のユーザーIDを取得
	myself, err := client.GetCurrentUser(ctx)
	if err != nil {
		return fmt.Errorf("failed to get current user: %w", err)
	}

	// ウォッチ一覧から該当する課題を探す
	watchings, err := client.GetWatchingList(ctx, myself.ID.Value, &api.WatchingListOptions{
		Count: 100,
	})
	if err != nil {
		return fmt.Errorf("failed to get watching list: %w", err)
	}

	var targetWatching *api.Watching
	for i := range watchings {
		if watchings[i].Issue.IssueKey == resolvedKey {
			targetWatching = &watchings[i]
			break
		}
	}

	if targetWatching == nil {
		return fmt.Errorf("issue %s is not in your watching list", resolvedKey)
	}

	// ウォッチ削除
	deleted, err := client.DeleteWatching(ctx, targetWatching.ID)
	if err != nil {
		return fmt.Errorf("failed to remove watching: %w", err)
	}

	// 出力
	switch profile.Output {
	case "json":
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(deleted)
	default:
		fmt.Printf("%s Removed from watchings: %s\n",
			ui.Green("✓"), deleted.Issue.IssueKey)
		return nil
	}
}
