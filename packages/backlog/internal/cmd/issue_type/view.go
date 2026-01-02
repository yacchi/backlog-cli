package issue_type

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/yacchi/backlog-cli/packages/backlog/internal/cmdutil"
	"github.com/yacchi/backlog-cli/packages/backlog/internal/ui"
)

var viewCmd = &cobra.Command{
	Use:   "view <id|name>",
	Short: "View issue type details",
	Long: `View details of an issue type.

Examples:
  backlog issue-type view 12345
  backlog issue-type view "バグ"
  backlog issue-type view Task -p PROJECT_KEY`,
	Args: cobra.ExactArgs(1),
	RunE: runIssueTypeView,
}

func runIssueTypeView(c *cobra.Command, args []string) error {
	idOrName := args[0]

	client, cfg, err := cmdutil.GetAPIClient(c)
	if err != nil {
		return err
	}

	if err := cmdutil.RequireProject(cfg); err != nil {
		return err
	}

	projectKey := cmdutil.GetCurrentProject(cfg)
	ctx := c.Context()

	issueType, err := resolveIssueType(ctx, client, projectKey, idOrName)
	if err != nil {
		return fmt.Errorf("failed to get issue type: %w", err)
	}

	if issueType == nil {
		return fmt.Errorf("issue type not found: %s", idOrName)
	}

	// 出力
	profile := cfg.CurrentProfile()
	switch profile.Output {
	case "json":
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(issueType)
	default:
		// 詳細表示
		fmt.Printf("%s %s\n", ui.Bold("種別:"), issueType.Name)
		fmt.Printf("%s %d\n", ui.Bold("ID:"), issueType.ID)
		fmt.Printf("%s %s %s\n", ui.Bold("色:"), ui.HexBgColor(issueType.Color, "  "), GetColorName(issueType.Color)+" ("+issueType.Color+")")
		fmt.Printf("%s %d\n", ui.Bold("表示順:"), issueType.DisplayOrder)
		fmt.Println()

		// テンプレート情報
		fmt.Println(ui.Bold("テンプレート:"))
		if issueType.TemplateSummary == "" && issueType.TemplateDescription == "" {
			fmt.Println(ui.Gray("  (テンプレート未設定)"))
		} else {
			if issueType.TemplateSummary != "" {
				fmt.Printf("  %s %s\n", ui.Bold("件名:"), issueType.TemplateSummary)
			}
			if issueType.TemplateDescription != "" {
				fmt.Printf("  %s\n", ui.Bold("詳細:"))
				fmt.Println("  " + issueType.TemplateDescription)
			}
		}

		return nil
	}
}
