package category

import (
	"github.com/spf13/cobra"
)

// CategoryCmd is the root command for category operations
var CategoryCmd = &cobra.Command{
	Use:   "category",
	Short: "Manage categories",
	Long:  `List, create, and delete categories in a project.`,
}

func init() {
	CategoryCmd.AddCommand(listCmd)
	CategoryCmd.AddCommand(createCmd)
	CategoryCmd.AddCommand(deleteCmd)
}
