package user

import (
	"github.com/spf13/cobra"
)

// UserCmd is the root command for user operations
var UserCmd = &cobra.Command{
	Use:   "user",
	Short: "Manage users",
	Long:  `List and view users in the space.`,
}

func init() {
	UserCmd.AddCommand(listCmd)
	UserCmd.AddCommand(viewCmd)
}
