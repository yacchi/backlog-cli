package star

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/yacchi/backlog-cli/packages/backlog/internal/api"
	"github.com/yacchi/backlog-cli/packages/backlog/internal/cmdutil"
	"github.com/yacchi/backlog-cli/packages/backlog/internal/ui"
)

var listCmd = &cobra.Command{
	Use:   "list [<user>]",
	Short: "List stars a user has received",
	Long: `List the stars a user has received on their issues, comments, wiki pages,
and pull requests.

<user> accepts a numeric user ID, a userId string, or "@me" (the default).
This lists stars the user has *received*, not stars they have given — the
Backlog API has no "stars I gave" endpoint.`,
	Example: `  # List the stars you have received
  backlog star list

  # List stars received by another user, as JSON for scripting
  backlog star list alice --output json

  # Page backwards from a known star ID, 50 at a time
  backlog star list --max-id 500 --count 50`,
	Args: cobra.MaximumNArgs(1),
	RunE: runList,
}

var (
	listMinID int
	listMaxID int
	listCount int
	listOrder string
)

func init() {
	listCmd.Flags().IntVar(&listMinID, "min-id", 0, "Return stars with ID greater than this value")
	listCmd.Flags().IntVar(&listMaxID, "max-id", 0, "Return stars with ID less than this value")
	listCmd.Flags().IntVar(&listCount, "count", 0, "Maximum number of stars to fetch (1-100; API default 20)")
	listCmd.Flags().StringVar(&listOrder, "order", "", "Sort order: asc or desc (API default desc)")
}

func runList(c *cobra.Command, args []string) error {
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

	stars, err := client.GetStars(ctx, userID, &api.StarListOptions{
		MinID: listMinID,
		MaxID: listMaxID,
		Count: listCount,
		Order: listOrder,
	})
	if err != nil {
		return fmt.Errorf("failed to get stars: %w", err)
	}

	profile := cfg.CurrentProfile()
	switch profile.Output {
	case "json":
		return cmdutil.OutputJSONFromProfile(stars, profile.JSONFields, profile.JQ, profile.Template)
	default:
		renderStarTable(stars)
		return nil
	}
}

func renderStarTable(stars []api.Star) {
	if len(stars) == 0 {
		fmt.Println("No stars found")
		return
	}
	table := ui.NewTable("ID", "TITLE", "URL", "CREATED")
	for _, s := range stars {
		table.AddRow(fmt.Sprintf("%d", s.ID), s.Title, s.URL, s.Created)
	}
	table.RenderWithColor(os.Stdout, ui.IsColorEnabled())
}
