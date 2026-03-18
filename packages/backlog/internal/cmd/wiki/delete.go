package wiki

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
	Short: "Delete a wiki page",
	Long: `Delete a wiki page permanently.

This action cannot be undone.

Examples:
  backlog wiki delete 123
  backlog wiki delete "Meeting Notes"
  backlog wiki delete 123 --yes  # Skip confirmation`,
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
	idOrName := args[0]
	ctx := c.Context()

	// Wiki IDを解決
	wikiID, err := resolveWikiID(client, ctx, projectKey, idOrName)
	if err != nil {
		return err
	}

	// Wikiページ情報を取得（確認プロンプト用）
	wiki, err := client.GetWiki(ctx, wikiID)
	if err != nil {
		return fmt.Errorf("failed to get wiki page: %w", err)
	}

	// 確認プロンプト
	if !deleteYes {
		var confirm bool
		prompt := &survey.Confirm{
			Message: fmt.Sprintf("Are you sure you want to delete wiki page: %s (ID: %d)?", wiki.Name, wiki.ID),
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

	// Wikiページを削除
	deletedWiki, err := client.DeleteWiki(ctx, wikiID)
	if err != nil {
		return fmt.Errorf("failed to delete wiki page: %w", err)
	}

	// 出力
	profile := cfg.CurrentProfile()
	switch profile.Output {
	case "json":
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(deletedWiki)
	default:
		ui.Success("Deleted wiki page: %s (ID: %d)", deletedWiki.Name, deletedWiki.ID)
		return nil
	}
}
