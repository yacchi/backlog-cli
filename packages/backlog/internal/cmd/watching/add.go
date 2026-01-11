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

var addCmd = &cobra.Command{
	Use:   "add <issue-key>",
	Short: "Add an issue to watchings",
	Long: `Add an issue to your watching list.

Examples:
  backlog watching add PROJ-123
  backlog watching add PROJ-123 --note "Track progress"`,
	Args: cobra.ExactArgs(1),
	RunE: runAdd,
}

var addNote string

func init() {
	addCmd.Flags().StringVarP(&addNote, "note", "n", "", "Note for the watching")
}

func runAdd(c *cobra.Command, args []string) error {
	client, cfg, err := cmdutil.GetAPIClient(c)
	if err != nil {
		return err
	}

	profile := cfg.CurrentProfile()
	issueKey := args[0]

	// 課題キーを解決
	resolvedKey, _ := cmdutil.ResolveIssueKey(issueKey, cmdutil.GetCurrentProject(cfg))

	// ウォッチ追加
	input := &api.AddWatchingInput{
		IssueIDOrKey: resolvedKey,
		Note:         addNote,
	}

	watching, err := client.AddWatching(c.Context(), input)
	if err != nil {
		return fmt.Errorf("failed to add watching: %w", err)
	}

	// 出力
	switch profile.Output {
	case "json":
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(watching)
	default:
		fmt.Printf("%s Added to watchings: %s\n",
			ui.Green("✓"), watching.Issue.IssueKey)
		return nil
	}
}
