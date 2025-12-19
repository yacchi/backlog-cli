package project

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
	Short: "List projects",
	Long: `List all accessible projects.

Examples:
  backlog project list
  backlog project list -o json`,
	RunE: runList,
}

var listArchived bool

func init() {
	listCmd.Flags().BoolVar(&listArchived, "archived", false, "Include archived projects")
}

func runList(c *cobra.Command, args []string) error {
	client, cfg, err := cmdutil.GetAPIClient(c)
	if err != nil {
		return err
	}

	projects, err := client.GetProjects()
	if err != nil {
		return fmt.Errorf("failed to get projects: %w", err)
	}

	if len(projects) == 0 {
		fmt.Println("No projects found")
		return nil
	}

	// アーカイブフィルター
	if !listArchived {
		filtered := make([]api.Project, 0)
		for _, p := range projects {
			if !p.Archived {
				filtered = append(filtered, p)
			}
		}
		projects = filtered
	}

	// 出力
	profile := cfg.CurrentProfile()
	switch profile.Output {
	case "json":
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(projects)
	default:
		outputProjectTable(projects, profile.Project)
		return nil
	}
}

func outputProjectTable(projects []api.Project, currentProject string) {
	table := ui.NewTable("KEY", "NAME", "STATUS")

	for _, p := range projects {
		key := p.ProjectKey
		if key == currentProject {
			key = ui.Green(key + " ✓")
		}

		status := "active"
		if p.Archived {
			status = ui.Gray("archived")
		}

		table.AddRow(key, p.Name, status)
	}

	table.RenderWithColor(os.Stdout, ui.IsColorEnabled())
}
