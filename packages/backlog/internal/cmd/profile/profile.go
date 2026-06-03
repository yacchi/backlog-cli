package profile

import (
	"github.com/spf13/cobra"
)

var ProfileCmd = &cobra.Command{
	Use:   "profile",
	Short: "Manage configuration profiles",
	Long: `Manage configuration profiles for multiple Backlog spaces and accounts.

Examples:
  backlog profile list
  backlog profile set-primary --profile default`,
}

func init() {
	ProfileCmd.AddCommand(listCmd)
	ProfileCmd.AddCommand(setPrimaryCmd)
}
