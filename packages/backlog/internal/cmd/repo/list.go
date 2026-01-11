package repo

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
	Short: "List Git repositories",
	Long: `List Git repositories in the project.

Examples:
  backlog repo list
  backlog repo list --project MYPROJECT`,
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

	repos, err := client.GetGitRepositories(c.Context(), projectKey)
	if err != nil {
		return fmt.Errorf("failed to get Git repositories: %w", err)
	}

	if len(repos) == 0 {
		fmt.Println("No Git repositories found")
		return nil
	}

	// 出力
	profile := cfg.CurrentProfile()
	switch profile.Output {
	case "json":
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(repos)
	default:
		outputRepoTable(repos)
		return nil
	}
}

func outputRepoTable(repos []api.GitRepository) {
	table := ui.NewTable("ID", "NAME", "DESCRIPTION", "PUSHED AT")

	for _, r := range repos {
		pushedAt := "-"
		if r.PushedAt != "" && len(r.PushedAt) >= 10 {
			pushedAt = r.PushedAt[:10]
		}

		table.AddRow(
			fmt.Sprintf("%d", r.ID),
			r.Name,
			truncate(r.Description, 30),
			pushedAt,
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
