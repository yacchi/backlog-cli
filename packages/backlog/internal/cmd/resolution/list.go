package resolution

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
	Short:   "List resolutions",
	Long: `List available resolutions.

Resolutions are used to specify how an issue was resolved when closing it.

Examples:
  backlog resolution list
  backlog resolution list --output json`,
	RunE: runList,
}

func runList(cmd *cobra.Command, args []string) error {
	client, cfg, err := cmdutil.GetAPIClient(cmd)
	if err != nil {
		return err
	}

	resolutions, err := client.GetResolutions(cmd.Context())
	if err != nil {
		return fmt.Errorf("failed to get resolutions: %w", err)
	}

	if len(resolutions) == 0 {
		fmt.Println("No resolutions found")
		return nil
	}

	profile := cfg.CurrentProfile()
	switch profile.Output {
	case "json":
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(resolutions)
	default:
		outputResolutionTable(resolutions)
		return nil
	}
}

func outputResolutionTable(resolutions []backlog.Resolution) {
	table := ui.NewTable("ID", "NAME")

	for _, r := range resolutions {
		table.AddRow(
			fmt.Sprintf("%d", r.ID.Value),
			r.Name.Value,
		)
	}

	table.RenderWithColor(os.Stdout, ui.IsColorEnabled())
}
