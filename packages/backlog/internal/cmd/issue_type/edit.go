package issue_type

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

var editCmd = &cobra.Command{
	Use:   "edit <id|name>",
	Short: "Edit an issue type",
	Long: `Edit an existing issue type.

Examples:
  backlog issue-type edit 12345 --name "新しい名前"
  backlog issue-type edit "バグ" --color "#990000"
  backlog issue-type edit Task  # Interactive mode`,
	Args: cobra.ExactArgs(1),
	RunE: runIssueTypeEdit,
}

var (
	issueTypeEditName                string
	issueTypeEditColor               string
	issueTypeEditTemplateSummary     string
	issueTypeEditTemplateDescription string
)

func init() {
	editCmd.Flags().StringVarP(&issueTypeEditName, "name", "n", "", "Issue type name")
	editCmd.Flags().StringVarP(&issueTypeEditColor, "color", "c", "", "Background color (e.g. #e30000)")
	editCmd.Flags().StringVar(&issueTypeEditTemplateSummary, "template-summary", "", "Template summary for issues")
	editCmd.Flags().StringVar(&issueTypeEditTemplateDescription, "template-description", "", "Template description for issues")
}

func runIssueTypeEdit(c *cobra.Command, args []string) error {
	idOrName := args[0]

	client, cfg, err := cmdutil.GetAPIClient(c)
	if err != nil {
		return err
	}

	if err := cmdutil.RequireProject(cfg); err != nil {
		return err
	}

	projectKey := cmdutil.GetCurrentProject(cfg)
	ctx := c.Context()

	// 既存の種別を取得
	issueType, err := resolveIssueType(ctx, client, projectKey, idOrName)
	if err != nil {
		return fmt.Errorf("failed to get issue type: %w", err)
	}

	if issueType == nil {
		return fmt.Errorf("issue type not found: %s", idOrName)
	}

	// フラグが一つも指定されていない場合は対話モード
	interactive := !c.Flags().Changed("name") &&
		!c.Flags().Changed("color") &&
		!c.Flags().Changed("template-summary") &&
		!c.Flags().Changed("template-description")

	input := &api.UpdateIssueTypeInput{}

	if interactive {
		// 対話モード: 名前
		var name string
		prompt := &survey.Input{
			Message: "種別名:",
			Default: issueType.Name,
		}
		if err := survey.AskOne(prompt, &name, survey.WithValidator(survey.Required)); err != nil {
			return err
		}
		if name != issueType.Name {
			input.Name = &name
		}

		// 対話モード: 色
		colorOptions := buildColorOptions()
		defaultIndex := findColorOptionIndex(issueType.Color)
		selectPrompt := &survey.Select{
			Message: "背景色を選択:",
			Options: colorOptions,
			Default: colorOptions[defaultIndex],
		}
		var selectedOption string
		if err := survey.AskOne(selectPrompt, &selectedOption); err != nil {
			return err
		}
		newColor := getColorFromOption(selectedOption)
		if newColor != issueType.Color {
			input.Color = &newColor
		}

		// 対話モード: テンプレート件名
		var templateSummary string
		templateSummaryPrompt := &survey.Input{
			Message: "テンプレート件名:",
			Default: issueType.TemplateSummary,
		}
		if err := survey.AskOne(templateSummaryPrompt, &templateSummary); err != nil {
			return err
		}
		if templateSummary != issueType.TemplateSummary {
			input.TemplateSummary = &templateSummary
		}

		// 対話モード: テンプレート詳細
		var templateDescription string
		templateDescPrompt := &survey.Multiline{
			Message: "テンプレート詳細:",
			Default: issueType.TemplateDescription,
		}
		if err := survey.AskOne(templateDescPrompt, &templateDescription); err != nil {
			return err
		}
		if templateDescription != issueType.TemplateDescription {
			input.TemplateDescription = &templateDescription
		}
	} else {
		// フラグモード
		if c.Flags().Changed("name") {
			input.Name = &issueTypeEditName
		}
		if c.Flags().Changed("color") {
			input.Color = &issueTypeEditColor
		}
		if c.Flags().Changed("template-summary") {
			input.TemplateSummary = &issueTypeEditTemplateSummary
		}
		if c.Flags().Changed("template-description") {
			input.TemplateDescription = &issueTypeEditTemplateDescription
		}
	}

	// 変更がない場合はスキップ
	if input.Name == nil && input.Color == nil && input.TemplateSummary == nil && input.TemplateDescription == nil {
		ui.Warning("変更はありません")
		return nil
	}

	// 種別更新
	updatedIssueType, err := client.UpdateIssueType(ctx, projectKey, issueType.ID, input)
	if err != nil {
		return fmt.Errorf("failed to update issue type: %w", err)
	}

	// 出力
	profile := cfg.CurrentProfile()
	switch profile.Output {
	case "json":
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(updatedIssueType)
	default:
		ui.Success("種別を更新しました: %s (ID: %d)", updatedIssueType.Name, updatedIssueType.ID)
		return nil
	}
}
