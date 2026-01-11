package issue

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/AlecAivazis/survey/v2"
	"github.com/spf13/cobra"
	"github.com/yacchi/backlog-cli/packages/backlog/internal/cmdutil"
	"github.com/yacchi/backlog-cli/packages/backlog/internal/ui"
)

var deleteCmd = &cobra.Command{
	Use:   "delete <issue-key>",
	Short: "Delete an issue",
	Long: `Delete an issue permanently.

This action cannot be undone. You must have administrator or project administrator permissions.

Examples:
  backlog issue delete PROJ-123
  backlog issue delete PROJ-123 --yes  # Skip confirmation`,
	Args: cobra.ExactArgs(1),
	RunE: runDelete,
}

var (
	deleteYes bool
)

func init() {
	deleteCmd.Flags().BoolVar(&deleteYes, "yes", false, "Skip confirmation prompt")
}

func runDelete(c *cobra.Command, args []string) error {
	issueKey := args[0]

	client, cfg, err := cmdutil.GetAPIClient(c)
	if err != nil {
		return err
	}

	ctx := c.Context()

	// まず課題が存在するか確認
	issue, err := client.GetIssue(ctx, issueKey)
	if err != nil {
		return fmt.Errorf("failed to get issue: %w", err)
	}

	// 確認プロンプト
	if !deleteYes {
		var confirm bool
		prompt := &survey.Confirm{
			Message: fmt.Sprintf("Are you sure you want to delete %s: %s?", issue.IssueKey.Value, issue.Summary.Value),
			Default: false,
		}
		if err := survey.AskOne(prompt, &confirm); err != nil {
			return err
		}
		if !confirm {
			fmt.Println("Aborted")
			return nil
		}
	}

	// 課題を削除
	deletedIssue, err := client.DeleteIssue(ctx, issueKey)
	if err != nil {
		return fmt.Errorf("failed to delete issue: %w", err)
	}

	// 出力
	profile := cfg.CurrentProfile()
	switch profile.Output {
	case "json":
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(deletedIssue)
	default:
		ui.Success("Deleted %s: %s", deletedIssue.IssueKey.Value, deletedIssue.Summary.Value)
		return nil
	}
}
