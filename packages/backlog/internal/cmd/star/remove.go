package star

import (
	"fmt"
	"strconv"

	"github.com/AlecAivazis/survey/v2"
	"github.com/spf13/cobra"
	"github.com/yacchi/backlog-cli/packages/backlog/internal/cmdutil"
	"github.com/yacchi/backlog-cli/packages/backlog/internal/ui"
)

var removeCmd = &cobra.Command{
	Use:     "remove <star-id>",
	Aliases: []string{"rm", "delete"},
	Short:   "Delete a star",
	Long: `Delete a star by its own star ID.

<star-id> is the ID of the star itself, from 'backlog star list' — it is
NOT an issue key, and NOT the ID of the issue/comment/wiki page/pull
request that was starred. Run 'backlog star list' (optionally with
'--output json' to get the "id" field directly) to find the star ID you
want to remove.

This action cannot be undone.`,
	Example: `  # Find the star ID first
  backlog star list --output json --jq '.[] | {id, title}'

  # Remove it, with confirmation
  backlog star remove 75

  # Remove it non-interactively (e.g. from a script)
  backlog star remove 75 --yes`,
	Args: cobra.ExactArgs(1),
	RunE: runRemove,
}

func runRemove(c *cobra.Command, args []string) error {
	starID, err := strconv.Atoi(args[0])
	if err != nil {
		return fmt.Errorf("invalid star ID: %q (must be numeric; get it from 'backlog star list')", args[0])
	}

	if !cmdutil.SkipConfirmation(c) {
		if !ui.IsInteractiveInput() {
			return cmdutil.NonInteractiveFlagError(
				"--yes is required when not running interactively",
				"backlog star remove",
				"Use --yes to skip the confirmation prompt.",
			)
		}
		var confirm bool
		prompt := &survey.Confirm{
			Message: fmt.Sprintf("Are you sure you want to delete star %d?", starID),
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

	client, cfg, err := cmdutil.GetAPIClient(c)
	if err != nil {
		return err
	}

	if err := client.DeleteStar(c.Context(), starID); err != nil {
		return fmt.Errorf("failed to remove star: %w", err)
	}

	profile := cfg.CurrentProfile()
	switch profile.Output {
	case "json":
		return cmdutil.OutputJSONFromProfile(map[string]any{
			"id":     starID,
			"status": "deleted",
		}, profile.JSONFields, profile.JQ, profile.Template)
	default:
		ui.Success("Removed star %d", starID)
		return nil
	}
}
