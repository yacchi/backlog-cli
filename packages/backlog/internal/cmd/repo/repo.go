package repo

import (
	"github.com/spf13/cobra"
)

var RepoCmd = &cobra.Command{
	Use:   "repo",
	Short: "Manage Git repositories",
	Long:  "Work with Backlog Git repositories.",
}

func init() {
	RepoCmd.AddCommand(listCmd)
	RepoCmd.AddCommand(viewCmd)
}
