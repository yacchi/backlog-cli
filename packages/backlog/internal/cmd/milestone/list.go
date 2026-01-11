package milestone

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
	Use:     "list",
	Aliases: []string{"ls"},
	Short:   "List milestones",
	Long: `List milestones (versions) in the project.

Examples:
  backlog milestone list
  backlog milestone list --project MYPROJECT
  backlog milestone list --all  # Include archived`,
	RunE: runList,
}

var (
	listAll bool
)

func init() {
	listCmd.Flags().BoolVarP(&listAll, "all", "a", false, "Show archived milestones too")
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

	versions, err := client.GetVersions(c.Context(), projectKey)
	if err != nil {
		return fmt.Errorf("failed to get milestones: %w", err)
	}

	// アーカイブフィルター
	if !listAll {
		filtered := make([]api.Version, 0, len(versions))
		for _, v := range versions {
			if !v.Archived {
				filtered = append(filtered, v)
			}
		}
		versions = filtered
	}

	if len(versions) == 0 {
		fmt.Println("No milestones found")
		return nil
	}

	// 出力
	profile := cfg.CurrentProfile()
	switch profile.Output {
	case "json":
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(versions)
	default:
		outputMilestoneTable(versions)
		return nil
	}
}

func outputMilestoneTable(versions []api.Version) {
	table := ui.NewTable("ID", "NAME", "START", "DUE", "ARCHIVED")

	for _, v := range versions {
		startDate := "-"
		if v.StartDate != "" {
			startDate = formatDate(v.StartDate)
		}

		dueDate := "-"
		if v.ReleaseDueDate != "" {
			dueDate = formatDate(v.ReleaseDueDate)
		}

		archived := "false"
		if v.Archived {
			archived = ui.Yellow("true")
		}

		table.AddRow(
			fmt.Sprintf("%d", v.ID),
			truncate(v.Name, 30),
			startDate,
			dueDate,
			archived,
		)
	}

	table.RenderWithColor(os.Stdout, ui.IsColorEnabled())
}

func formatDate(s string) string {
	if len(s) >= 10 {
		return s[:10]
	}
	return s
}

func truncate(s string, max int) string {
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	return string(runes[:max-3]) + "..."
}
