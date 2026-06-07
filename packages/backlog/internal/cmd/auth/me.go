package auth

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/yacchi/backlog-cli/packages/backlog/internal/api"
	"github.com/yacchi/backlog-cli/packages/backlog/internal/cmdutil"
	"github.com/yacchi/backlog-cli/packages/backlog/internal/config"
)

// RunMe is the exported handler for "auth me" / "whoami".
var RunMe = runMe

var meCmd = &cobra.Command{
	Use:   "me",
	Short: "Show current authenticated user information",
	Long: `Display detailed information about the currently authenticated Backlog user.

Examples:
  backlog auth me
  backlog auth me --quiet

  # Output as JSON (includes numeric user id)
  backlog auth me -o json
  backlog auth me -o json --jq .id`,
	RunE: runMe,
}

var meQuiet bool

func init() {
	meCmd.Flags().BoolVarP(&meQuiet, "quiet", "q", false, "Exit with code 0 if authenticated, 1 otherwise (no output)")
}

func runMe(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load(cmd.Context())
	if err != nil {
		if meQuiet {
			os.Exit(1)
		}
		return fmt.Errorf("failed to load config: %w", err)
	}

	client, err := api.NewClientFromConfig(cfg)
	if err != nil {
		if meQuiet {
			os.Exit(1)
		}
		return fmt.Errorf("not authenticated: %w", err)
	}

	user, err := client.GetCurrentUser(cmd.Context())
	if err != nil {
		if meQuiet {
			os.Exit(1)
		}
		return fmt.Errorf("failed to get user info: %w", err)
	}

	// quiet mode: just check authentication, no output
	if meQuiet {
		return nil
	}

	// JSON出力（profile.Output が json のとき）
	// /api/v2/users/myself のレスポンスをそのまま出力する
	if profile := cfg.CurrentProfile(); profile != nil && profile.Output == "json" {
		return cmdutil.OutputJSONFromProfile(user, profile.JSONFields, profile.JQ, profile.Template)
	}

	fmt.Printf("User ID:       %s\n", user.UserId.Value)
	fmt.Printf("Name:          %s\n", user.Name.Value)
	fmt.Printf("Email:         %s\n", user.MailAddress.Value)
	fmt.Printf("Role:          %s\n", roleTypeName(user.RoleType.Value))
	fmt.Printf("Language:      %s\n", user.Lang.Value)

	if user.NulabAccount.IsSet() {
		account := user.NulabAccount.Value
		fmt.Println()
		fmt.Println("Nulab Account:")
		fmt.Printf("  Nulab ID:    %s\n", account.NulabId.Value)
		fmt.Printf("  Name:        %s\n", account.Name.Value)
		fmt.Printf("  Unique ID:   %s\n", account.UniqueId.Value)
	}

	return nil
}

func roleTypeName(roleType int) string {
	switch roleType {
	case 1:
		return "Administrator"
	case 2:
		return "Normal User"
	case 3:
		return "Reporter"
	case 4:
		return "Viewer"
	case 5:
		return "Guest Reporter"
	case 6:
		return "Guest Viewer"
	default:
		return fmt.Sprintf("Unknown (%d)", roleType)
	}
}
