package wiki

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/yacchi/backlog-cli/packages/backlog/internal/api"
	"github.com/yacchi/backlog-cli/packages/backlog/internal/cmdutil"
	"github.com/yacchi/backlog-cli/packages/backlog/internal/ui"
)

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List wiki pages",
	Long: `List wiki pages in the project.

Examples:
  backlog wiki list
  backlog wiki list --project MYPROJECT
  backlog wiki list --search "release notes"
  backlog wiki list --count`,
	RunE: runList,
}

var (
	wikiListCount  bool
	wikiListSearch string
)

func init() {
	listCmd.Flags().BoolVar(&wikiListCount, "count", false, "Show only the count of wiki pages")
	listCmd.Flags().StringVarP(&wikiListSearch, "search", "S", "", "Search wiki pages by keyword (name and content)")
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

	// 件数のみ表示
	if wikiListCount {
		// Backlog の /wikis/count は keyword を無視するため、
		// --search 併用時は一覧を取得してクライアント側で数える。
		if wikiListSearch != "" {
			wikis, err := client.GetWikis(c.Context(), projectKey, wikiListSearch)
			if err != nil {
				return fmt.Errorf("failed to get wiki pages: %w", err)
			}
			fmt.Println(len(wikis))
			return nil
		}
		count, err := client.GetWikisCount(c.Context(), projectKey, wikiListSearch)
		if err != nil {
			return fmt.Errorf("failed to get wiki count: %w", err)
		}
		fmt.Println(count)
		return nil
	}

	wikis, err := client.GetWikis(c.Context(), projectKey, wikiListSearch)
	if err != nil {
		return fmt.Errorf("failed to get wiki pages: %w", err)
	}

	// 出力
	profile := cfg.CurrentProfile()
	switch profile.Output {
	case "json":
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(wikis)
	default:
		if len(wikis) == 0 {
			fmt.Println("No wiki pages found")
			return nil
		}
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
