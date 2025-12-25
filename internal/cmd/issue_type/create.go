package issue_type

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/AlecAivazis/survey/v2"
	"github.com/spf13/cobra"
	"github.com/yacchi/backlog-cli/internal/api"
	"github.com/yacchi/backlog-cli/internal/cmdutil"
	"github.com/yacchi/backlog-cli/internal/ui"
)

var createCmd = &cobra.Command{
	Use:   "create",
	Short: "Create an issue type",
	Long: `Create a new issue type.

Examples:
  backlog issue-type create --name "バグ" --color "#e30000"
  backlog issue-type create  # Interactive mode`,
	RunE: runIssueTypeCreate,
}

var (
	issueTypeCreateName                string
	issueTypeCreateColor               string
	issueTypeCreateTemplateSummary     string
	issueTypeCreateTemplateDescription string
)

func init() {
	createCmd.Flags().StringVarP(&issueTypeCreateName, "name", "n", "", "Issue type name")
	createCmd.Flags().StringVarP(&issueTypeCreateColor, "color", "c", "", "Background color (e.g. #e30000)")
	createCmd.Flags().StringVar(&issueTypeCreateTemplateSummary, "template-summary", "", "Template summary for issues")
	createCmd.Flags().StringVar(&issueTypeCreateTemplateDescription, "template-description", "", "Template description for issues")
}

// buildColorOptions は色選択肢を作成する
func buildColorOptions() []string {
	options := make([]string, len(IssueTypeColors))
	for i, c := range IssueTypeColors {
		// 色付きの■マークと名前を表示
		options[i] = ui.HexBgColor(c.Hex, "  ") + " " + c.Name + " (" + c.Hex + ")"
	}
	return options
}

// getColorFromOption は選択肢からHEXカラーコードを取得する
func getColorFromOption(option string) string {
	for _, c := range IssueTypeColors {
		expected := ui.HexBgColor(c.Hex, "  ") + " " + c.Name + " (" + c.Hex + ")"
		if option == expected {
			return c.Hex
		}
	}
	return ""
}

// findColorOptionIndex は色のHEX値から選択肢のインデックスを返す
func findColorOptionIndex(hex string) int {
	for i, c := range IssueTypeColors {
		if c.Hex == hex {
			return i
		}
	}
	return 0
}

func runIssueTypeCreate(c *cobra.Command, args []string) error {
	client, cfg, err := cmdutil.GetAPIClient(c)
	if err != nil {
		return err
	}

	if err := cmdutil.RequireProject(cfg); err != nil {
		return err
	}

	projectKey := cmdutil.GetCurrentProject(cfg)
	ctx := c.Context()

	name := issueTypeCreateName
	color := issueTypeCreateColor
	templateSummary := issueTypeCreateTemplateSummary
	templateDescription := issueTypeCreateTemplateDescription

	// 対話モード: 名前
	if name == "" {
		prompt := &survey.Input{
			Message: "種別名:",
		}
		if err := survey.AskOne(prompt, &name, survey.WithValidator(survey.Required)); err != nil {
			return err
		}
	}

	// 対話モード: 色
	if color == "" {
		colorOptions := buildColorOptions()
		prompt := &survey.Select{
			Message: "背景色を選択:",
			Options: colorOptions,
		}
		var selectedOption string
		if err := survey.AskOne(prompt, &selectedOption); err != nil {
			return err
		}
		color = getColorFromOption(selectedOption)
	}

	// 対話モード: テンプレート件名（オプション）
	if templateSummary == "" && !c.Flags().Changed("template-summary") {
		prompt := &survey.Input{
			Message: "テンプレート件名 (省略可):",
		}
		if err := survey.AskOne(prompt, &templateSummary); err != nil {
			return err
		}
	}

	// 対話モード: テンプレート詳細（オプション）
	if templateDescription == "" && !c.Flags().Changed("template-description") {
		prompt := &survey.Multiline{
			Message: "テンプレート詳細 (省略可):",
		}
		if err := survey.AskOne(prompt, &templateDescription); err != nil {
			return err
		}
	}

	// 種別作成
	input := &api.CreateIssueTypeInput{
		Name:                name,
		Color:               color,
		TemplateSummary:     templateSummary,
		TemplateDescription: templateDescription,
	}

	issueType, err := client.CreateIssueType(ctx, projectKey, input)
	if err != nil {
		return fmt.Errorf("failed to create issue type: %w", err)
	}

	// 出力
	profile := cfg.CurrentProfile()
	switch profile.Output {
	case "json":
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(issueType)
	default:
		ui.Success("種別を作成しました: %s (ID: %d)", issueType.Name, issueType.ID)
		return nil
	}
}
