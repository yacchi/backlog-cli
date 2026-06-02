package activity

import (
	"github.com/spf13/cobra"
)

// ActivityCmd is the root command for activity operations.
var ActivityCmd = &cobra.Command{
	Use:   "activity",
	Short: "Show user activity feed",
	Long: `Show a user's recent activity feed across all accessible projects.

Backlog records each action a user takes (issue created/updated/commented,
wiki changes, git pushes, pull requests, etc.) as an activity. This command
exposes that cross-project feed as a first-class command.`,
}

func init() {
	ActivityCmd.AddCommand(listCmd)
}
