package milestone

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"

	"github.com/AlecAivazis/survey/v2"
	"github.com/spf13/cobra"
	"github.com/yacchi/backlog-cli/packages/backlog/internal/api"
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

	// まずバージョン一覧を取得してID/名前で検索
	ctx := c.Context()
	versions, err := client.GetVersions(ctx, projectKey)
	if err != nil {
		return fmt.Errorf("failed to get milestones: %w", err)
	}

	version := findVersionForDelete(versions, idOrName)
	if version == nil {
		return fmt.Errorf("milestone not found: %s", idOrName)
	}

	// 確認プロンプト
	if !deleteYes {
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

func findVersionForDelete(versions []api.Version, idOrName string) *api.Version {
	// まずIDとして解釈
	if id, err := strconv.Atoi(idOrName); err == nil {
		for i := range versions {
			if versions[i].ID == id {
				return &versions[i]
			}
		}
	}

	// 名前で検索
	for i := range versions {
		if versions[i].Name == idOrName {
			return &versions[i]
		}
	}

	return nil
}
