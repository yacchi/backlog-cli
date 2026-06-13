package user

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/yacchi/backlog-cli/packages/backlog/internal/cmdutil"
	"github.com/yacchi/backlog-cli/packages/backlog/internal/gen/backlog"
	"github.com/yacchi/backlog-cli/packages/backlog/internal/ui"
)

var userListSearch string

var listCmd = &cobra.Command{
	Use:     "list",
	Aliases: []string{"ls"},
	Short:   "List users",
	Long: `List users in the space.

Examples:
  backlog user list
  backlog user list --search 藤江
  backlog user list --output json
  backlog user list --json id,userId,name`,
	RunE: runList,
}

func init() {
	listCmd.Flags().StringVarP(&userListSearch, "search", "s", "", "Filter users by name or userId (case-insensitive substring match)")
}

func runList(cmd *cobra.Command, args []string) error {
	client, cfg, err := cmdutil.GetAPIClient(cmd)
	if err != nil {
		return err
	}

	users, err := client.GetUsers(cmd.Context())
	if err != nil {
		return fmt.Errorf("failed to get users: %w", err)
	}

	if userListSearch != "" {
		query := strings.ToLower(userListSearch)
		filtered := users[:0]
		for _, u := range users {
			if strings.Contains(strings.ToLower(u.Name.Value), query) ||
				strings.Contains(strings.ToLower(u.UserId.Value), query) ||
				strings.Contains(strings.ToLower(u.MailAddress.Value), query) {
				filtered = append(filtered, u)
			}
		}
		users = filtered
	}

	profile := cfg.CurrentProfile()
	switch profile.Output {
	case "json":
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(users)
	default:
		if len(users) == 0 {
			fmt.Println("No users found")
			return nil
		}
		outputUserTable(users)
		return nil
	}
}

func outputUserTable(users []backlog.User) {
	table := ui.NewTable("ID", "USER ID", "NAME", "MAIL", "ROLE")

	for _, u := range users {
		role := roleTypeName(u.RoleType.Value)
		table.AddRow(
			fmt.Sprintf("%d", u.ID.Value),
			u.UserId.Value,
			u.Name.Value,
			u.MailAddress.Value,
			role,
		)
	}

	table.RenderWithColor(os.Stdout, ui.IsColorEnabled())
}

func roleTypeName(roleType int) string {
	switch roleType {
	case 1:
		return "Admin"
	case 2:
		return "User"
	case 3:
		return "Reporter"
	case 4:
		return "Viewer"
	case 5:
		return "GuestReporter"
	case 6:
		return "GuestViewer"
	default:
		return fmt.Sprintf("Unknown(%d)", roleType)
	}
}
