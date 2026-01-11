package category

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/yacchi/backlog-cli/packages/backlog/internal/api"
	"github.com/yacchi/backlog-cli/packages/backlog/internal/cmdutil"
	"github.com/yacchi/backlog-cli/packages/backlog/internal/ui"
)

var listCmd = &cobra.Command{
	Use:     "list",
	Aliases: []string{"ls"},
	Short:   "List categories",
	Long: `List categories in the project.

Examples:
  backlog category list
  backlog category list -p PROJECT_KEY
  backlog category list --output json`,
	RunE: runList,
}

func runList(cmd *cobra.Command, args []string) error {
	client, cfg, err := cmdutil.GetAPIClient(cmd)
	if err != nil {
		return err
	}

	if err := cmdutil.RequireProject(cfg); err != nil {
		return err
	}

	projectKey := cmdutil.GetCurrentProject(cfg)

	categories, err := client.GetCategories(cmd.Context(), projectKey)
	if err != nil {
		return fmt.Errorf("failed to get categories: %w", err)
	}

	if len(categories) == 0 {
		fmt.Println("No categories found")
		return nil
	}

	profile := cfg.CurrentProfile()
	switch profile.Output {
	case "json":
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(categories)
	default:
		outputCategoryTable(categories)
		return nil
	}
}

func outputCategoryTable(categories []api.Category) {
	table := ui.NewTable("ID", "NAME", "ORDER")

	for _, c := range categories {
		table.AddRow(
			fmt.Sprintf("%d", c.ID),
			c.Name,
			fmt.Sprintf("%d", c.DisplayOrder),
		)
	}

	table.RenderWithColor(os.Stdout, ui.IsColorEnabled())
}
