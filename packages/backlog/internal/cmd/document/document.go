package document

import (
	"github.com/spf13/cobra"
)

var DocumentCmd = &cobra.Command{
	Use:     "document",
	Aliases: []string{"doc", "docs"},
	Short:   "Manage Backlog documents",
	Long:    "Work with Backlog documents. Note: use 'backlog wiki' for legacy wiki pages.",
}

func init() {
	DocumentCmd.AddCommand(listCmd)
	DocumentCmd.AddCommand(viewCmd)
	DocumentCmd.AddCommand(treeCmd)
	DocumentCmd.AddCommand(countCmd)
	DocumentCmd.AddCommand(createCmd)
	DocumentCmd.AddCommand(deleteCmd)
	DocumentCmd.AddCommand(commentCmd)
	DocumentCmd.AddCommand(tagCmd)
	DocumentCmd.AddCommand(attachmentCmd)
}
