package team

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/yacchi/backlog-cli/packages/backlog/internal/api"
	"github.com/yacchi/backlog-cli/packages/backlog/internal/cmdutil"
	"github.com/yacchi/backlog-cli/packages/backlog/internal/ui"
)

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List teams",
	Long: `List teams.

With no '-p/--project', lists every team defined at the space level. With
'-p/--project <projectKey>', lists only the teams assigned to that project
instead — see 'backlog team --help' for the distinction between the two.
'--order', '--offset', and '--count' apply only to the space-wide listing;
the project-scoped listing does not support paging or sorting.`,
	Example: `  # List every team in the space
  backlog team list

  # List only the teams assigned to project PROJ
  backlog team list --project PROJ

  # Get team names as JSON, for scripting
  backlog team list --output json --jq '.[].name'`,
	RunE: runList,
}

var (
	listOrder  string
	listOffset int
	listCount  int
)

func init() {
	listCmd.Flags().StringVar(&listOrder, "order", "", "Sort order: asc or desc (API default desc; space-wide listing only)")
	listCmd.Flags().IntVar(&listOffset, "offset", 0, "Number of teams to skip (space-wide listing only)")
	listCmd.Flags().IntVar(&listCount, "count", 0, "Maximum number of teams to fetch (1-100; API default 20; space-wide listing only)")
}

func runList(c *cobra.Command, args []string) error {
	client, cfg, err := cmdutil.GetAPIClient(c)
	if err != nil {
		return err
	}

	projectKey, _ := c.Flags().GetString("project")

	var teams []api.Team
	if projectKey != "" {
		teams, err = client.GetProjectTeams(c.Context(), projectKey)
	} else {
		teams, err = client.GetTeams(c.Context(), &api.TeamListOptions{
			Order:  listOrder,
			Offset: listOffset,
			Count:  listCount,
		})
	}
	if err != nil {
		return fmt.Errorf("failed to get teams: %w", err)
	}

	profile := cfg.CurrentProfile()
	switch profile.Output {
	case "json":
		return cmdutil.OutputJSONFromProfile(teams, profile.JSONFields, profile.JQ, profile.Template)
	default:
		renderTeamTable(teams)
		return nil
	}
}

func renderTeamTable(teams []api.Team) {
	if len(teams) == 0 {
		fmt.Println("No teams found")
		return
	}
	table := ui.NewTable("ID", "NAME", "MEMBERS", "CREATED")
	for _, t := range teams {
		table.AddRow(fmt.Sprintf("%d", t.ID), t.Name, fmt.Sprintf("%d", len(t.Members)), t.Created)
	}
	table.RenderWithColor(os.Stdout, ui.IsColorEnabled())
}
