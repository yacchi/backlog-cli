package issue_type

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/yacchi/backlog-cli/internal/cmdutil"
	"github.com/yacchi/backlog-cli/internal/ui"
)

var listCmd = &cobra.Command{
	Use:     "list",
	Aliases: []string{"ls"},
	Short:   "List issue types",
	Long: `List all issue types in the project.

Examples:
  backlog issue-type list
  backlog issue-type list -p PROJECT_KEY`,
	RunE: runIssueTypeList,
}

func runIssueTypeList(c *cobra.Command, args []string) error {
	client, cfg, err := cmdutil.GetAPIClient(c)
	if err != nil {
		return err
	}

	if err := cmdutil.RequireProject(cfg); err != nil {
		return err
	}

	projectKey := cmdutil.GetCurrentProject(cfg)
	ctx := c.Context()

	issueTypes, err := client.GetIssueTypes(ctx, projectKey)
	if err != nil {
		return fmt.Errorf("failed to get issue types: %w", err)
	}

	// 出力
	profile := cfg.CurrentProfile()
	switch profile.Output {
	case "json":
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(issueTypes)
	default:
		if len(issueTypes) == 0 {
			fmt.Println("No issue types found.")
			return nil
		}

		// テーブル形式で出力
		table := ui.NewTable("ID", "名前", "色", "テンプレート件名")
		for _, it := range issueTypes {
			colorDisplay := ui.HexBgColor(it.Color, "  ") + " " + GetColorName(it.Color)
			templateSummary := it.TemplateSummary
			if templateSummary == "" {
				templateSummary = ui.Gray("(なし)")
			}
			table.AddRow(
				fmt.Sprintf("%d", it.ID),
				it.Name,
				colorDisplay,
				templateSummary,
			)
		}
		table.RenderWithColor(os.Stdout, ui.IsColorEnabled())
		return nil
	}
}
