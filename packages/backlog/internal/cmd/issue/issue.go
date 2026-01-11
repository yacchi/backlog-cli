package issue

import (
	"github.com/spf13/cobra"
)

var IssueCmd = &cobra.Command{
	Use:   "issue",
	Short: "Manage Backlog issues",
	Long:  "Work with Backlog issues.",
}

func init() {
	IssueCmd.AddCommand(listCmd)
	IssueCmd.AddCommand(viewCmd)
	IssueCmd.AddCommand(createCmd)
	IssueCmd.AddCommand(editCmd)
	IssueCmd.AddCommand(closeCmd)
	IssueCmd.AddCommand(reopenCmd)
	IssueCmd.AddCommand(commentCmd)
	IssueCmd.AddCommand(deleteCmd)
	IssueCmd.AddCommand(statusCmd)
}
