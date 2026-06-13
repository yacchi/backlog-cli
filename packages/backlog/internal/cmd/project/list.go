package project

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
	Short: "List projects",
	Long: `List all accessible projects.

Examples:
  backlog project list
  backlog project list --archived
  backlog project list --all
  backlog project list -o json`,
	RunE: runList,
}

var (
	listArchived bool
	listAll      bool
)

func init() {
	listCmd.Flags().BoolVar(&listArchived, "archived", false, "Include archived projects")
	listCmd.Flags().BoolVar(&listAll, "all", false, "List all projects on the space (administrators only)")
}

func runList(c *cobra.Command, args []string) error {
	client, cfg, err := cmdutil.GetAPIClient(c)
	if err != nil {
		return err
	}

	opts := &api.ProjectListOptions{All: listAll}
	// --archived 未指定時は未アーカイブのみ。指定時はアーカイブ済みも含める。
	if !listArchived {
		notArchived := false
		opts.Archived = &notArchived
	}

	projects, err := client.GetProjects(c.Context(), opts)
	if err != nil {
		return fmt.Errorf("failed to get projects: %w", err)
	}

	// 出力
	profile := cfg.CurrentProfile()
	switch profile.Output {
	case "json":
		return cmdutil.OutputJSONFromProfile(projects, profile.JSONFields, profile.JQ, profile.Template)
	default:
		if len(projects) == 0 {
			fmt.Println("No projects found")
			return nil
		}
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
