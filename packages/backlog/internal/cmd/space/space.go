package space

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/yacchi/backlog-cli/packages/backlog/internal/cmdutil"
	"github.com/yacchi/backlog-cli/packages/backlog/internal/gen/backlog"
	"github.com/yacchi/backlog-cli/packages/backlog/internal/ui"
)

// SpaceCmd is the root command for space operations
var SpaceCmd = &cobra.Command{
	Use:   "space",
	Short: "Display space information",
	Long: `Display information about the Backlog space.

Examples:
  backlog space
  backlog space --json spaceKey,name,textFormattingRule
  backlog space --output json`,
	RunE: runSpace,
}

func runSpace(cmd *cobra.Command, args []string) error {
	client, cfg, err := cmdutil.GetAPIClient(cmd)
	if err != nil {
		return err
	}

	space, err := client.GetSpace(cmd.Context())
	if err != nil {
		return fmt.Errorf("failed to get space: %w", err)
	}

	profile := cfg.CurrentProfile()
	switch profile.Output {
	case "json":
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(space)
	default:
		outputSpaceTable(space)
		return nil
	}
}

func outputSpaceTable(space *backlog.Space) {
	fmt.Printf("%s %s\n", ui.Bold("Space Key:"), space.SpaceKey.Value)
	fmt.Printf("%s %s\n", ui.Bold("Name:"), space.Name.Value)
	fmt.Printf("%s %s\n", ui.Bold("Language:"), space.Lang.Value)
	fmt.Printf("%s %s\n", ui.Bold("Timezone:"), space.Timezone.Value)
	fmt.Printf("%s %s\n", ui.Bold("Text Formatting:"), space.TextFormattingRule.Value)
	fmt.Printf("%s %s\n", ui.Bold("Created:"), formatDate(space.Created.Value))
	fmt.Printf("%s %s\n", ui.Bold("Updated:"), formatDate(space.Updated.Value))
}

func formatDate(s string) string {
	if len(s) >= 10 {
		return s[:10]
	}
	return s
}
