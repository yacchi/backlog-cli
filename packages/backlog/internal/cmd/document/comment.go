package document

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/yacchi/backlog-cli/packages/backlog/internal/api"
	"github.com/yacchi/backlog-cli/packages/backlog/internal/cmdutil"
	"github.com/yacchi/backlog-cli/packages/backlog/internal/ui"
)

var commentCmd = &cobra.Command{
	Use:   "comment",
	Short: "Manage document comments",
}

var commentListCmd = &cobra.Command{
	Use:   "list <document-id>",
	Short: "List comments on a document",
	Long: `List comments on a document.

Examples:
  backlog document comment list 01HXXXXXXXX`,
	Args: cobra.ExactArgs(1),
	RunE: runCommentList,
}

func init() {
	commentCmd.AddCommand(commentListCmd)
}

func runCommentList(c *cobra.Command, args []string) error {
	documentID := args[0]

	client, cfg, err := cmdutil.GetAPIClient(c)
	if err != nil {
		return err
	}

	comments, err := client.GetDocumentComments(c.Context(), documentID)
	if err != nil {
		return fmt.Errorf("failed to get document comments: %w", err)
	}

	if len(comments) == 0 {
		fmt.Fprintln(os.Stderr, "No comments found")
		return nil
	}

	profile := cfg.CurrentProfile()
	switch profile.Output {
	case "json":
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(comments)
	default:
		outputCommentTable(comments)
		return nil
	}
}

func outputCommentTable(comments []api.DocumentComment) {
	table := ui.NewTable("ID", "AUTHOR", "PLAIN", "CREATED")

	for _, cm := range comments {
		plain := truncate(cm.Plain, 50)
		if plain == "" {
			plain = truncate(cm.Content, 50)
		}
		created := ""
		if len(cm.Created) >= 10 {
			created = cm.Created[:10]
		}
		table.AddRow(
			fmt.Sprintf("%d", cm.ID),
			cm.CreatedUser.Name,
			plain,
			created,
		)
	}

	table.RenderWithColor(os.Stdout, ui.IsColorEnabled())
}
