package milestone

import (
	"github.com/spf13/cobra"
)

var MilestoneCmd = &cobra.Command{
	Use:     "milestone",
	Aliases: []string{"ms", "version"},
	Short:   "Manage milestones",
	Long:    "Work with Backlog milestones (versions).",
}

func init() {
	MilestoneCmd.AddCommand(listCmd)
	MilestoneCmd.AddCommand(viewCmd)
	MilestoneCmd.AddCommand(createCmd)
	MilestoneCmd.AddCommand(editCmd)
	MilestoneCmd.AddCommand(deleteCmd)
}
