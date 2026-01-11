package user

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"

	"github.com/spf13/cobra"
	"github.com/yacchi/backlog-cli/packages/backlog/internal/cmdutil"
	"github.com/yacchi/backlog-cli/packages/backlog/internal/gen/backlog"
	"github.com/yacchi/backlog-cli/packages/backlog/internal/ui"
)

var viewCmd = &cobra.Command{
	Use:   "view <user-id>",
	Short: "View user details",
	Long: `View details of a specific user.

Examples:
  backlog user view 12345
  backlog user view 12345 --output json`,
	Args: cobra.ExactArgs(1),
	RunE: runView,
}

func runView(cmd *cobra.Command, args []string) error {
	client, cfg, err := cmdutil.GetAPIClient(cmd)
	if err != nil {
		return err
	}

	userID, err := strconv.Atoi(args[0])
	if err != nil {
		return fmt.Errorf("invalid user ID: %s", args[0])
	}

	user, err := client.GetUser(cmd.Context(), userID)
	if err != nil {
		return fmt.Errorf("failed to get user: %w", err)
	}

	profile := cfg.CurrentProfile()
	switch profile.Output {
	case "json":
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(user)
	default:
		outputUserDetail(user)
		return nil
	}
}

func outputUserDetail(user *backlog.User) {
	fmt.Printf("%s %d\n", ui.Bold("ID:"), user.ID.Value)
	fmt.Printf("%s %s\n", ui.Bold("User ID:"), user.UserId.Value)
	fmt.Printf("%s %s\n", ui.Bold("Name:"), user.Name.Value)
	if user.MailAddress.IsSet() {
		fmt.Printf("%s %s\n", ui.Bold("Mail:"), user.MailAddress.Value)
	}
	fmt.Printf("%s %s\n", ui.Bold("Role:"), roleTypeName(user.RoleType.Value))
	if user.Lang.IsSet() {
		fmt.Printf("%s %s\n", ui.Bold("Language:"), user.Lang.Value)
	}
}
