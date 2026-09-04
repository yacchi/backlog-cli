package star

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/yacchi/backlog-cli/packages/backlog/internal/cmdutil"
)

var countCmd = &cobra.Command{
	Use:   "count [<user>]",
	Short: "Count stars a user has received",
	Long: `Show the total number of stars a user has received on their issues,
comments, wiki pages, and pull requests.

<user> accepts a numeric user ID, a userId string, or "@me" (the default).`,
	Example: `  # Count your own received stars
  backlog star count

  # Count another user's received stars, by their userId string
  backlog star count alice

  # Count another user's received stars as a raw number, for scripting
  backlog star count bob --output json --jq '.count'`,
	Args: cobra.MaximumNArgs(1),
	RunE: runCount,
}

func runCount(c *cobra.Command, args []string) error {
	client, cfg, err := cmdutil.GetAPIClient(c)
	if err != nil {
		return err
	}
	ctx := c.Context()

	target := "@me"
	if len(args) == 1 {
		target = args[0]
	}
	userID, err := cmdutil.ResolveUserID(ctx, client, target)
	if err != nil {
		return fmt.Errorf("failed to resolve user: %w", err)
	}

	count, err := client.GetStarsCount(ctx, userID)
	if err != nil {
		return fmt.Errorf("failed to get star count: %w", err)
	}

	profile := cfg.CurrentProfile()
	switch profile.Output {
	case "json":
		return cmdutil.OutputJSONFromProfile(map[string]int{"count": count}, profile.JSONFields, profile.JQ, profile.Template)
	default:
		fmt.Println(count)
		return nil
	}
}
