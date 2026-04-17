package document

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
	Use:   "delete <document-id>",
	Short: "Delete a document",
	Long: `Delete a document permanently.

This action cannot be undone.

Examples:
  backlog document delete 01HXXXXXXXX
  backlog document delete 01HXXXXXXXX --yes`,
	Args: cobra.ExactArgs(1),
	RunE: runDelete,
}

var deleteYes bool

func init() {
	deleteCmd.Flags().BoolVar(&deleteYes, "yes", false, "Skip confirmation prompt")
}

func runDelete(c *cobra.Command, args []string) error {
	documentID := args[0]

	client, cfg, err := cmdutil.GetAPIClient(c)
	if err != nil {
		return err
	}

	ctx := c.Context()

	doc, err := client.GetDocument(ctx, documentID)
	if err != nil {
		return fmt.Errorf("failed to get document: %w", err)
	}

	if !deleteYes {
		var confirm bool
		prompt := &survey.Confirm{
			Message: fmt.Sprintf("Are you sure you want to delete document: %s (ID: %s)?", doc.Title, doc.ID),
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

	deleted, err := client.DeleteDocument(ctx, documentID)
	if err != nil {
		return fmt.Errorf("failed to delete document: %w", err)
	}

	profile := cfg.CurrentProfile()
	switch profile.Output {
	case "json":
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(deleted)
	default:
		ui.Success("Deleted document: %s (ID: %s)", deleted.Title, deleted.ID)
		return nil
	}
}
