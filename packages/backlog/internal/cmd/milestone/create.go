package milestone

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/AlecAivazis/survey/v2"
	"github.com/spf13/cobra"
	"github.com/yacchi/backlog-cli/packages/backlog/internal/api"
	"github.com/yacchi/backlog-cli/packages/backlog/internal/cmdutil"
	"github.com/yacchi/backlog-cli/packages/backlog/internal/ui"
)

var createCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a milestone",
	Long: `Create a new milestone (version).

Examples:
  backlog milestone create --name "v2.0.0"
  backlog milestone create --name "Sprint 1" --start-date 2024-04-01 --due-date 2024-04-14
  backlog milestone create  # Interactive mode`,
	RunE: runCreate,
}

var (
	createName        string
	createDescription string
	createStartDate   string
	createDueDate     string
)

func init() {
	createCmd.Flags().StringVarP(&createName, "name", "n", "", "Milestone name (required)")
	createCmd.Flags().StringVarP(&createDescription, "description", "d", "", "Description")
	createCmd.Flags().StringVarP(&createStartDate, "start-date", "s", "", "Start date (YYYY-MM-DD)")
	createCmd.Flags().StringVarP(&createDueDate, "due-date", "D", "", "Release due date (YYYY-MM-DD)")
}

func runCreate(c *cobra.Command, args []string) error {
	client, cfg, err := cmdutil.GetAPIClient(c)
	if err != nil {
		return err
	}

	if err := cmdutil.RequireProject(cfg); err != nil {
		return err
	}

	projectKey := cmdutil.GetCurrentProject(cfg)
	profile := cfg.CurrentProfile()

	// 対話モード: 名前が未指定の場合
	if createName == "" {
		prompt := &survey.Input{
			Message: "Milestone name:",
		}
		if err := survey.AskOne(prompt, &createName, survey.WithValidator(survey.Required)); err != nil {
			return err
		}
	}

	// マイルストーン作成
	input := &api.CreateVersionInput{
		Name:           createName,
		Description:    createDescription,
		StartDate:      createStartDate,
		ReleaseDueDate: createDueDate,
	}

	version, err := client.CreateVersion(c.Context(), projectKey, input)
	if err != nil {
		return fmt.Errorf("failed to create milestone: %w", err)
	}

	// 出力
	switch profile.Output {
	case "json":
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(version)
	default:
		fmt.Printf("%s Milestone created: %s (ID: %d)\n",
			ui.Green("✓"), version.Name, version.ID)
		return nil
	}
}
