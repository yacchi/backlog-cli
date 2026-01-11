package priority

import (
	"github.com/spf13/cobra"
)

// PriorityCmd is the root command for priority operations
var PriorityCmd = &cobra.Command{
	Use:   "priority",
	Short: "Manage priorities",
	Long:  `List available priorities for issues.`,
}

func init() {
	PriorityCmd.AddCommand(listCmd)
}
