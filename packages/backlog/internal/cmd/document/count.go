package document

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/yacchi/backlog-cli/packages/backlog/internal/cmdutil"
)

var countCmd = &cobra.Command{
	Use:   "count",
	Short: "Count documents in a project",
	Long: `Display the number of documents in the current project.

Examples:
  backlog document count`,
	RunE: runCount,
}

func runCount(c *cobra.Command, args []string) error {
	client, cfg, err := cmdutil.GetAPIClient(c)
	if err != nil {
		return err
	}

	if err := cmdutil.RequireProject(cfg); err != nil {
		return err
	}

	projectKey := cmdutil.GetCurrentProject(cfg)

	count, err := client.GetDocumentCount(c.Context(), projectKey)
	if err != nil {
		return fmt.Errorf("failed to get document count: %w", err)
	}

	fmt.Println(count)
	return nil
}
