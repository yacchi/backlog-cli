package notification

import (
	"github.com/spf13/cobra"
)

var NotificationCmd = &cobra.Command{
	Use:     "notification",
	Aliases: []string{"notif", "notifications"},
	Short:   "Manage notifications",
	Long:    "Work with Backlog notifications.",
}

func init() {
	NotificationCmd.AddCommand(listCmd)
	NotificationCmd.AddCommand(readCmd)
}
