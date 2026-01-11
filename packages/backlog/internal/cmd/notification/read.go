package notification

import (
	"fmt"
	"strconv"

	"github.com/spf13/cobra"
	"github.com/yacchi/backlog-cli/packages/backlog/internal/cmdutil"
	"github.com/yacchi/backlog-cli/packages/backlog/internal/ui"
)

var readCmd = &cobra.Command{
	Use:   "read [id]",
	Short: "Mark notification as read",
	Long: `Mark a notification as read, or mark all notifications as read.

Examples:
  backlog notification read 12345    # Mark single notification as read
  backlog notification read --all    # Mark all notifications as read`,
	RunE: runRead,
}

var readAll bool

func init() {
	readCmd.Flags().BoolVar(&readAll, "all", false, "Mark all notifications as read")
}

func runRead(c *cobra.Command, args []string) error {
	client, _, err := cmdutil.GetAPIClient(c)
	if err != nil {
		return err
	}

	ctx := c.Context()

	// 全て既読
	if readAll {
		count, err := client.ResetUnreadNotificationCount(ctx)
		if err != nil {
			return fmt.Errorf("failed to mark all as read: %w", err)
		}
		fmt.Printf("%s Marked %d notifications as read\n", ui.Green("✓"), count)
		return nil
	}

	// 個別既読
	if len(args) == 0 {
		return fmt.Errorf("notification ID required. Use --all to mark all as read")
	}

	notificationID, err := strconv.Atoi(args[0])
	if err != nil {
		return fmt.Errorf("invalid notification ID: %s", args[0])
	}

	if err := client.MarkNotificationAsRead(ctx, notificationID); err != nil {
		return fmt.Errorf("failed to mark as read: %w", err)
	}

	fmt.Printf("%s Notification %d marked as read\n", ui.Green("✓"), notificationID)
	return nil
}
