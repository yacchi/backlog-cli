package issue

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/yacchi/backlog-cli/internal/cmdutil"
	"github.com/yacchi/backlog-cli/internal/ui"
)

var commentCmd = &cobra.Command{
	Use:   "comment <issue-key>",
	Short: "Add a comment to an issue",
	Long: `Add a comment to an issue.

Without the body text supplied through flags, the command will interactively
prompt for the comment text.

Examples:
  # Add a comment using --body flag
  backlog issue comment PROJ-123 --body "This is fixed"
  backlog issue comment PROJ-123 -b "Quick fix applied"

  # Read comment from a file
  backlog issue comment PROJ-123 --body-file comment.txt

  # Read comment from standard input
  echo "Comment from stdin" | backlog issue comment PROJ-123 --body-file -

  # Open editor to write comment
  backlog issue comment PROJ-123 --editor`,
	Args: cobra.ExactArgs(1),
	RunE: runComment,
}

var (
	commentBody     string
	commentBodyFile string
	commentEditor   bool
)

func init() {
	commentCmd.Flags().StringVarP(&commentBody, "body", "b", "", "The comment body text")
	commentCmd.Flags().StringVarP(&commentBodyFile, "body-file", "F", "", "Read body text from file (use \"-\" to read from standard input)")
	commentCmd.Flags().BoolVarP(&commentEditor, "editor", "e", false, "Open editor to write the comment")
}

func runComment(c *cobra.Command, args []string) error {
	issueKey := args[0]

	client, cfg, err := cmdutil.GetAPIClient(c)
	if err != nil {
		return err
	}

	// メッセージ取得
	message, err := cmdutil.ResolveBody(
		commentBody,
		commentBodyFile,
		commentEditor,
		openEditor,
		func() (string, error) {
			return ui.Input("Comment:", "")
		},
	)
	if err != nil {
		return fmt.Errorf("failed to get comment: %w", err)
	}

	if message == "" {
		return fmt.Errorf("comment cannot be empty")
	}

	comment, err := client.AddComment(c.Context(), issueKey, message, nil)
	if err != nil {
		return fmt.Errorf("failed to add comment: %w", err)
	}

	ui.Success("Added comment #%d to %s", comment.ID, issueKey)

	profile := cfg.CurrentProfile()
	url := fmt.Sprintf("https://%s.%s/view/%s#comment-%d", profile.Space, profile.Domain, issueKey, comment.ID)
	fmt.Printf("URL: %s\n", ui.Cyan(url))

	return nil
}
