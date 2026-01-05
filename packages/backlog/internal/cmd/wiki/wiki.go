package wiki

import (
	"github.com/spf13/cobra"
)

var WikiCmd = &cobra.Command{
	Use:   "wiki",
	Short: "Manage wiki pages",
	Long:  "Work with Backlog wiki pages.",
}

func init() {
	WikiCmd.AddCommand(listCmd)
	WikiCmd.AddCommand(viewCmd)
	WikiCmd.AddCommand(createCmd)
}
