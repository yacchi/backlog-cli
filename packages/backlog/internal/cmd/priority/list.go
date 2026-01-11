package priority

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
	Short:   "List priorities",
	Long: `List available priorities.

Examples:
  backlog priority list
  backlog priority list --output json`,
	RunE: runList,
}

func runList(cmd *cobra.Command, args []string) error {
	client, cfg, err := cmdutil.GetAPIClient(cmd)
	if err != nil {
		return err
	}

	priorities, err := client.GetPriorities(cmd.Context())
	if err != nil {
		return fmt.Errorf("failed to get priorities: %w", err)
	}

	if len(priorities) == 0 {
		fmt.Println("No priorities found")
		return nil
	}

	profile := cfg.CurrentProfile()
	switch profile.Output {
	case "json":
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(priorities)
	default:
		outputPriorityTable(priorities)
		return nil
	}
}

func outputPriorityTable(priorities []backlog.Priority) {
	table := ui.NewTable("ID", "NAME")

	for _, p := range priorities {
		table.AddRow(
			fmt.Sprintf("%d", p.ID.Value),
			p.Name.Value,
		)
	}

	table.RenderWithColor(os.Stdout, ui.IsColorEnabled())
}
