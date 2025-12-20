package wiki

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/yacchi/backlog-cli/internal/api"
	"github.com/yacchi/backlog-cli/internal/cmdutil"
	"github.com/yacchi/backlog-cli/internal/ui"
)

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List wiki pages",
	Long: `List wiki pages in the project.

Examples:
  backlog wiki list
  backlog wiki list --project MYPROJECT`,
	RunE: runList,
}

func init() {
}

func runList(c *cobra.Command, args []string) error {
	client, cfg, err := cmdutil.GetAPIClient(c)
	if err != nil {
		return err
	}

	if err := cmdutil.RequireProject(cfg); err != nil {
		return err
	}

	projectKey := cmdutil.GetCurrentProject(cfg)

	wikis, err := client.GetWikis(c.Context(), projectKey)
	if err != nil {
		return fmt.Errorf("failed to get wiki pages: %w", err)
	}

	if len(wikis) == 0 {
		fmt.Println("No wiki pages found")
		return nil
	}

	// 出力
	profile := cfg.CurrentProfile()
	switch profile.Output {
	case "json":
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(wikis)
	default:
		outputWikiTable(wikis)
		return nil
	}
}

func outputWikiTable(wikis []api.Wiki) {
	table := ui.NewTable("ID", "NAME", "TAGS", "UPDATED")

	for _, w := range wikis {
		tags := ""
		for i, tag := range w.Tags {
			if i > 0 {
				tags += ", "
			}
			tags += tag.Name
		}

		updated := ""
		if len(w.Updated) >= 10 {
			updated = w.Updated[:10]
		}

		table.AddRow(
			fmt.Sprintf("%d", w.ID),
			truncate(w.Name, 40),
			truncate(tags, 20),
			updated,
		)
	}

	table.RenderWithColor(os.Stdout, ui.IsColorEnabled())
}

func truncate(s string, max int) string {
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	return string(runes[:max-3]) + "..."
}
