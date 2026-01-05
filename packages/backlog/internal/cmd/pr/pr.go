package pr

import (
	"github.com/spf13/cobra"
)

var PRCmd = &cobra.Command{
	Use:     "pr",
	Aliases: []string{"pull-request"},
	Short:   "Manage pull requests",
	Long:    "Work with Backlog Git pull requests.",
}

func init() {
	PRCmd.AddCommand(listCmd)
	PRCmd.AddCommand(viewCmd)
}
