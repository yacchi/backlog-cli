package category

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/yacchi/backlog-cli/packages/backlog/internal/cmdutil"
)

var (
	createName string
)

var createCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a category",
	Long: `Create a new category in the project.

Examples:
  backlog category create --name "Bug"
  backlog category create --name "Feature" -p PROJECT_KEY`,
	RunE: runCreate,
}

func init() {
	createCmd.Flags().StringVarP(&createName, "name", "n", "", "Category name (required)")
	_ = createCmd.MarkFlagRequired("name")
}

func runCreate(cmd *cobra.Command, args []string) error {
	client, cfg, err := cmdutil.GetAPIClient(cmd)
	if err != nil {
		return err
	}

	if err := cmdutil.RequireProject(cfg); err != nil {
		return err
	}

	projectKey := cmdutil.GetCurrentProject(cfg)

	category, err := client.CreateCategory(cmd.Context(), projectKey, createName)
	if err != nil {
		return fmt.Errorf("failed to create category: %w", err)
	}

	profile := cfg.CurrentProfile()
	switch profile.Output {
	case "json":
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(category)
	default:
		fmt.Printf("Created category: %s (ID: %d)\n", category.Name.Value, category.ID.Value)
		return nil
	}
}
