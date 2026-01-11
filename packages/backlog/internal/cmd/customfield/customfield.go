package customfield

import (
	"github.com/spf13/cobra"
)

// CustomFieldCmd is the root command for custom field operations
var CustomFieldCmd = &cobra.Command{
	Use:     "custom-field",
	Aliases: []string{"cf"},
	Short:   "Manage custom fields",
	Long:    `List custom fields in a project.`,
}

func init() {
	CustomFieldCmd.AddCommand(listCmd)
}
