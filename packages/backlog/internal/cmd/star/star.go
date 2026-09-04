package star

import (
	"github.com/spf13/cobra"
)

// StarCmd は `backlog star` コマンド
var StarCmd = &cobra.Command{
	Use:   "star",
	Short: "Manage stars",
	Long: `Work with Backlog stars.

A star is a lightweight marker (visible in the Backlog web UI as the item's
star count) that a user can attach to exactly one of: an issue, a comment,
a wiki page, or a pull request. It is not the same as 'backlog watching':
watching subscribes an issue for update notifications and can be listed or
removed by issue key, while a star is a one-off marker on any of the four
item types above and is always removed by its own star ID (see
'backlog star remove --help').`,
}

func init() {
	StarCmd.AddCommand(addCmd)
	StarCmd.AddCommand(removeCmd)
	StarCmd.AddCommand(listCmd)
	StarCmd.AddCommand(countCmd)
}
