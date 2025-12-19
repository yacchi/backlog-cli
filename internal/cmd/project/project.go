package project

import (
	"github.com/spf13/cobra"
)

var ProjectCmd = &cobra.Command{
	Use:   "project",
	Short: "Manage Backlog projects",
	Long:  "Work with Backlog projects.",
}

func init() {
	ProjectCmd.AddCommand(listCmd)
	ProjectCmd.AddCommand(viewCmd)
	ProjectCmd.AddCommand(initCmd)
}
