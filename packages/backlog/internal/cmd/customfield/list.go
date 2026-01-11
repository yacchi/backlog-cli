package customfield

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/yacchi/backlog-cli/packages/backlog/internal/cmdutil"
	"github.com/yacchi/backlog-cli/packages/backlog/internal/gen/backlog"
	"github.com/yacchi/backlog-cli/packages/backlog/internal/ui"
)

var listCmd = &cobra.Command{
	Use:     "list",
	Aliases: []string{"ls"},
	Short:   "List custom fields",
	Long: `List custom fields in the project.

Custom field types:
  1: Text
  2: Text area
  3: Numeric
  4: Date
  5: Single-select list
  6: Multi-select list
  7: Checkbox
  8: Radio button

Examples:
  backlog custom-field list
  backlog custom-field list -p PROJECT_KEY
  backlog custom-field list --output json`,
	RunE: runList,
}

func runList(cmd *cobra.Command, args []string) error {
	client, cfg, err := cmdutil.GetAPIClient(cmd)
	if err != nil {
		return err
	}

	if err := cmdutil.RequireProject(cfg); err != nil {
		return err
	}

	projectKey := cmdutil.GetCurrentProject(cfg)

	customFields, err := client.GetCustomFields(cmd.Context(), projectKey)
	if err != nil {
		return fmt.Errorf("failed to get custom fields: %w", err)
	}

	if len(customFields) == 0 {
		fmt.Println("No custom fields found")
		return nil
	}

	profile := cfg.CurrentProfile()
	switch profile.Output {
	case "json":
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(customFields)
	default:
		outputCustomFieldTable(customFields)
		return nil
	}
}

func outputCustomFieldTable(customFields []backlog.CustomField) {
	table := ui.NewTable("ID", "NAME", "TYPE", "REQUIRED", "DESCRIPTION")

	for _, cf := range customFields {
		required := "No"
		if cf.Required.Value {
			required = ui.Yellow("Yes")
		}

		table.AddRow(
			fmt.Sprintf("%d", cf.ID.Value),
			truncate(cf.Name.Value, 25),
			typeName(cf.TypeId.Value),
			required,
			truncate(cf.Description.Value, 30),
		)
	}

	table.RenderWithColor(os.Stdout, ui.IsColorEnabled())
}

func typeName(typeID int) string {
	switch typeID {
	case 1:
		return "Text"
	case 2:
		return "TextArea"
	case 3:
		return "Numeric"
	case 4:
		return "Date"
	case 5:
		return "SingleList"
	case 6:
		return "MultiList"
	case 7:
		return "Checkbox"
	case 8:
		return "Radio"
	default:
		return fmt.Sprintf("Unknown(%d)", typeID)
	}
}

func truncate(s string, max int) string {
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	return string(runes[:max-3]) + "..."
}
