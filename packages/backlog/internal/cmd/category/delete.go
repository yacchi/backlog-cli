package category

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"

	"github.com/spf13/cobra"
	"github.com/yacchi/backlog-cli/packages/backlog/internal/cmdutil"
)

var deleteCmd = &cobra.Command{
	Use:   "delete <category-id>",
	Short: "Delete a category",
	Long: `Delete a category from the project.

Examples:
  backlog category delete 12345
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

	categoryID, err := strconv.Atoi(args[0])
	if err != nil {
		return fmt.Errorf("invalid category ID: %s", args[0])
	}

	category, err := client.DeleteCategory(cmd.Context(), projectKey, categoryID)
	if err != nil {
		return fmt.Errorf("failed to delete category: %w", err)
	}

	profile := cfg.CurrentProfile()
	switch profile.Output {
	case "json":
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(category)
	default:
		fmt.Printf("Deleted category: %s (ID: %d)\n", category.Name.Value, category.ID.Value)
		return nil
	}
}
