package issue

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/yacchi/backlog-cli/internal/cmdutil"
	"github.com/yacchi/backlog-cli/internal/ui"
)

var commentCmd = &cobra.Command{
	Use:   "comment <issue-key> [message]",
	Short: "Add a comment to an issue",
	Long: `Add a comment to an issue.

If no message is provided, opens an editor.

Examples:
  backlog issue comment PROJ-123 "This is fixed"
  backlog issue comment PROJ-123 --editor`,
	Args: cobra.MinimumNArgs(1),
	RunE: runComment,
}

var commentEditor bool

func init() {
	commentCmd.Flags().BoolVarP(&commentEditor, "editor", "e", false, "Open editor")
}

func runComment(c *cobra.Command, args []string) error {
	issueKey := args[0]

	var message string
	if len(args) > 1 {
		message = args[1]
	}

	client, _, err := cmdutil.GetAPIClient(c)
	if err != nil {
		return err
	}

	// メッセージ取得
	if message == "" {
		if commentEditor {
			message, err = openEditor("")
			if err != nil {
				return fmt.Errorf("failed to open editor: %w", err)
			}
		} else {
			message, err = ui.Input("Comment:", "")
			if err != nil {
				return err
			}
		}
	}

	if message == "" {
		return fmt.Errorf("comment cannot be empty")
	}

	comment, err := client.AddComment(issueKey, message, nil)
	if err != nil {
		return fmt.Errorf("failed to add comment: %w", err)
	}

	ui.Success("Added comment #%d to %s", comment.ID, issueKey)
	return nil
}
