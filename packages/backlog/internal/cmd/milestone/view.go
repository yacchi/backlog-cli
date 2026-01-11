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

var viewCmd = &cobra.Command{
	Use:   "view <id-or-name>",
	Short: "View a milestone",
	Long: `View details of a milestone (version).

Examples:
  backlog milestone view 123
  backlog milestone view "v1.0.0"`,
	Args: cobra.ExactArgs(1),
	RunE: runView,
}

func init() {
}

func runView(c *cobra.Command, args []string) error {
	client, cfg, err := cmdutil.GetAPIClient(c)
	if err != nil {
		return err
	}

	if err := cmdutil.RequireProject(cfg); err != nil {
		return err
	}

	projectKey := cmdutil.GetCurrentProject(cfg)
	idOrName := args[0]

	// まずバージョン一覧を取得してID/名前で検索
	versions, err := client.GetVersions(c.Context(), projectKey)
	if err != nil {
		return fmt.Errorf("failed to get milestones: %w", err)
	}

	version := findVersion(versions, idOrName)
	if version == nil {
		return fmt.Errorf("milestone not found: %s", idOrName)
	}

	// 出力
	profile := cfg.CurrentProfile()
	switch profile.Output {
	case "json":
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(version)
	default:
		outputMilestoneDetail(version)
		return nil
	}
}

func findVersion(versions []api.Version, idOrName string) *api.Version {
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

func outputMilestoneDetail(v *api.Version) {
	// タイトル
	fmt.Printf("%s\n", ui.Bold(v.Name))
	fmt.Println()

	// 詳細
	fmt.Printf("ID:          %d\n", v.ID)

	if v.Description != "" {
		fmt.Printf("Description: %s\n", v.Description)
	}

	fmt.Println()

	if v.StartDate != "" {
		fmt.Printf("Start Date:  %s\n", formatDate(v.StartDate))
	} else {
		fmt.Printf("Start Date:  -\n")
	}

	if v.ReleaseDueDate != "" {
		fmt.Printf("Due Date:    %s\n", formatDate(v.ReleaseDueDate))
	} else {
		fmt.Printf("Due Date:    -\n")
	}

	if v.Archived {
		fmt.Printf("Archived:    %s\n", ui.Yellow("true"))
	} else {
		fmt.Printf("Archived:    false\n")
	}
}
