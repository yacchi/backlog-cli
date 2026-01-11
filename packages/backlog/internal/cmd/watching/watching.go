package watching

import (
	"github.com/spf13/cobra"
)

var WatchingCmd = &cobra.Command{
	Use:     "watching",
	Aliases: []string{"watch"},
	Short:   "Manage watchings",
	Long:    "Work with Backlog issue watchings.",
}

func init() {
	WatchingCmd.AddCommand(listCmd)
	WatchingCmd.AddCommand(addCmd)
	WatchingCmd.AddCommand(removeCmd)
}
