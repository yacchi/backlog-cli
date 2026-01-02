package auth

import (
	"github.com/spf13/cobra"
)

var AuthCmd = &cobra.Command{
	Use:   "auth",
	Short: "Authenticate with Backlog",
	Long:  "Manage authentication state for Backlog CLI.",
}

func init() {
	AuthCmd.AddCommand(loginCmd)
	AuthCmd.AddCommand(logoutCmd)
	AuthCmd.AddCommand(statusCmd)
	AuthCmd.AddCommand(meCmd)
}
