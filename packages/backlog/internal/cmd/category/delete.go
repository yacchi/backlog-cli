package category

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/yacchi/backlog-cli/packages/backlog/internal/cmdutil"
)

var deleteCmd = &cobra.Command{
	Use:   "delete <category-id-or-name>",
	Short: "Delete a category",
	Long: `Delete a category from the project.

Examples:
  backlog category delete 12345
  backlog category delete Bug
  backlog category delete 12345 -p PROJECT_KEY`,
	Args: cobra.ExactArgs(1),
	RunE: runDelete,
}

func runDelete(cmd *cobra.Command, args []string) error {
	client, cfg, err := cmdutil.GetAPIClient(cmd)
	if err != nil {
		return err
	}

	if err := cmdutil.RequireProject(cfg); err != nil {
		return err
	}

	projectKey := cmdutil.GetCurrentProject(cfg)
	category, err := cmdutil.ResolveCategory(cmd.Context(), client, projectKey, args[0])
	if err != nil {
		return fmt.Errorf("failed to resolve category: %w", err)
	}
	if category == nil {
		return fmt.Errorf("category not found: %s", args[0])
	}

	deletedCategory, err := client.DeleteCategory(cmd.Context(), projectKey, category.ID)
	if err != nil {
		return fmt.Errorf("failed to delete category: %w", err)
	}

	profile := cfg.CurrentProfile()
	switch profile.Output {
	case "json":
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(deletedCategory)
	default:
		fmt.Printf("Deleted category: %s (ID: %d)\n", deletedCategory.Name.Value, deletedCategory.ID.Value)
		return nil
	}
}
