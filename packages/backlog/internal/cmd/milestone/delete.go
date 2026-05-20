package milestone

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/AlecAivazis/survey/v2"
	"github.com/spf13/cobra"
	"github.com/yacchi/backlog-cli/packages/backlog/internal/cmdutil"
	"github.com/yacchi/backlog-cli/packages/backlog/internal/ui"
)

var deleteCmd = &cobra.Command{
	Use:   "delete <id-or-name>",
	Short: "Delete a milestone",
	Long: `Delete a milestone (version).

This action cannot be undone.

Examples:
  backlog milestone delete 123
  backlog milestone delete "v0.9.0" --yes  # Skip confirmation`,
	Args: cobra.ExactArgs(1),
	RunE: runDelete,
}

var (
	deleteYes bool
)

func init() {
	deleteCmd.Flags().BoolVar(&deleteYes, "yes", false, "Skip confirmation prompt")
}

func runDelete(c *cobra.Command, args []string) error {
	client, cfg, err := cmdutil.GetAPIClient(c)
	if err != nil {
		return err
	}

	if err := cmdutil.RequireProject(cfg); err != nil {
		return err
	}

	projectKey := cmdutil.GetCurrentProject(cfg)
	profile := cfg.CurrentProfile()
	idOrName := args[0]

	ctx := c.Context()
	version, err := cmdutil.ResolveMilestone(ctx, client, projectKey, idOrName)
	if err != nil {
		return fmt.Errorf("failed to resolve milestone: %w", err)
	}

	// 確認プロンプト
	if !deleteYes {
		if !ui.IsInteractiveInput() {
			return cmdutil.NonInteractiveFlagError(
				"--yes is required when not running interactively",
				"backlog milestone delete",
				"Use --yes to skip the confirmation prompt.",
			)
		}
		var confirm bool
		prompt := &survey.Confirm{
			Message: fmt.Sprintf("Are you sure you want to delete %s?", version.Name),
			Default: false,
		}
		if err := survey.AskOne(prompt, &confirm); err != nil {
			return err
		}
		if !confirm {
			fmt.Println("Aborted")
			return nil
		}
	}

	// 削除実行
	deleted, err := client.DeleteVersion(ctx, projectKey, version.ID)
	if err != nil {
		return fmt.Errorf("failed to delete milestone: %w", err)
	}

	// 出力
	switch profile.Output {
	case "json":
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(deleted)
	default:
		fmt.Printf("%s Milestone deleted: %s\n", ui.Green("✓"), deleted.Name)
		return nil
	}
}
