package team

import (
	"github.com/spf13/cobra"
)

// TeamCmd は `backlog team` コマンド
var TeamCmd = &cobra.Command{
	Use:   "team",
	Short: "Manage teams",
	Long: `Work with Backlog teams — named groups of users defined at the space level
and reusable as a single unit (for example as an issue assignee, or on a
project's member list) across projects.

'-p/--project' changes what 'backlog team list' shows: without it, every
team defined in the space is listed; with it, only the teams assigned to
that one project are listed. A team can exist in the space without being
assigned to any given project, so pick '--project' when the question is
"which teams can this project use" and omit it for the full space roster.`,
}

func init() {
	TeamCmd.AddCommand(listCmd)
	TeamCmd.AddCommand(viewCmd)
}
