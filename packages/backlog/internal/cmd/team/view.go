package team

import (
	"fmt"
	"strconv"

	"github.com/spf13/cobra"
	"github.com/yacchi/backlog-cli/packages/backlog/internal/api"
	"github.com/yacchi/backlog-cli/packages/backlog/internal/cmdutil"
)

var viewCmd = &cobra.Command{
	Use:   "view <team-id>",
	Short: "View a team",
	Long: `Show detailed information about one team, including its member list.

<team-id> is the team's numeric ID (see 'backlog team list').`,
	Example: `  # View a team by ID
  backlog team view 5

  # Find the team ID first, then view it
  backlog team list --output json --jq '.[] | select(.name=="Backend") | .id'

  # Get just the member display names as JSON, for scripting
  backlog team view 5 --output json --jq '.members[].name'`,
	Args: cobra.ExactArgs(1),
	RunE: runView,
}

func runView(c *cobra.Command, args []string) error {
	teamID, err := strconv.Atoi(args[0])
	if err != nil {
		return fmt.Errorf("invalid team ID: %q (must be numeric; get it from 'backlog team list')", args[0])
	}

	client, cfg, err := cmdutil.GetAPIClient(c)
	if err != nil {
		return err
	}

	team, err := client.GetTeam(c.Context(), teamID)
	if err != nil {
		return fmt.Errorf("failed to get team: %w", err)
	}

	profile := cfg.CurrentProfile()
	switch profile.Output {
	case "json":
		return cmdutil.OutputJSONFromProfile(team, profile.JSONFields, profile.JQ, profile.Template)
	default:
		renderTeamDetail(team)
		return nil
	}
}

func renderTeamDetail(t *api.Team) {
	fmt.Printf("Team: %s (ID: %d)\n", t.Name, t.ID)
	fmt.Printf("Members: %d\n", len(t.Members))
	for _, m := range t.Members {
		if m.UserID != "" {
			fmt.Printf("  - %s (%s)\n", m.Name, m.UserID)
		} else {
			fmt.Printf("  - %s\n", m.Name)
		}
	}
}
