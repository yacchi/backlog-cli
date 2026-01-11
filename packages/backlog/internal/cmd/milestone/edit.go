package milestone

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"

	"github.com/spf13/cobra"
	"github.com/yacchi/backlog-cli/packages/backlog/internal/api"
	"github.com/yacchi/backlog-cli/packages/backlog/internal/cmdutil"
	"github.com/yacchi/backlog-cli/packages/backlog/internal/ui"
)

var editCmd = &cobra.Command{
	Use:   "edit <id-or-name>",
	Short: "Edit a milestone",
	Long: `Edit an existing milestone (version).

Examples:
  backlog milestone edit 123 --name "v2.0.0-rc1"
  backlog milestone edit "v1.0.0" --due-date 2024-04-15
  backlog milestone edit 123 --archive
  backlog milestone edit 123 --unarchive`,
	Args: cobra.ExactArgs(1),
	RunE: runEdit,
}

var (
	editName        string
	editDescription string
	editStartDate   string
	editDueDate     string
	editArchive     bool
	editUnarchive   bool
)

func init() {
	editCmd.Flags().StringVarP(&editName, "name", "n", "", "New name")
	editCmd.Flags().StringVarP(&editDescription, "description", "d", "", "New description")
	editCmd.Flags().StringVarP(&editStartDate, "start-date", "s", "", "New start date (YYYY-MM-DD)")
	editCmd.Flags().StringVarP(&editDueDate, "due-date", "D", "", "New release due date (YYYY-MM-DD)")
	editCmd.Flags().BoolVar(&editArchive, "archive", false, "Archive the milestone")
	editCmd.Flags().BoolVar(&editUnarchive, "unarchive", false, "Unarchive the milestone")
}

func runEdit(c *cobra.Command, args []string) error {
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

	version := findVersionForEdit(versions, idOrName)
	if version == nil {
		return fmt.Errorf("milestone not found: %s", idOrName)
	}

	// 更新入力を構築（nameは必須なので現在の値をデフォルトに）
	name := version.Name
	if editName != "" {
		name = editName
	}

	input := &api.UpdateVersionInput{
		Name: name,
	}

	// 変更があるフィールドのみ設定
	if c.Flags().Changed("description") {
		input.Description = &editDescription
	}
	if c.Flags().Changed("start-date") {
		input.StartDate = &editStartDate
	}
	if c.Flags().Changed("due-date") {
		input.ReleaseDueDate = &editDueDate
	}

	// archive/unarchiveの処理
	if editArchive && editUnarchive {
		return fmt.Errorf("cannot use both --archive and --unarchive")
	}
	if editArchive {
		archived := true
		input.Archived = &archived
	}
	if editUnarchive {
		archived := false
		input.Archived = &archived
	}

	// 更新実行
	updated, err := client.UpdateVersion(ctx, projectKey, version.ID, input)
	if err != nil {
		return fmt.Errorf("failed to update milestone: %w", err)
	}

	// 出力
	switch profile.Output {
	case "json":
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(updated)
	default:
		fmt.Printf("%s Milestone updated: %s (ID: %d)\n",
			ui.Green("✓"), updated.Name, updated.ID)
		return nil
	}
}

func findVersionForEdit(versions []api.Version, idOrName string) *api.Version {
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
