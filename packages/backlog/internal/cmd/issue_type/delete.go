package issue_type

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
	Use:   "delete <id|name>",
	Short: "Delete an issue type",
	Long: `Delete an issue type.

Issues associated with this type will be reassigned to another type.
You must specify the substitute issue type ID.

Examples:
  backlog issue-type delete 12345 --substitute 67890
  backlog issue-type delete "バグ"  # Interactive mode`,
	Args: cobra.ExactArgs(1),
	RunE: runIssueTypeDelete,
}

var (
	issueTypeDeleteSubstitute string
	issueTypeDeleteYes        bool
)

func init() {
	deleteCmd.Flags().StringVar(&issueTypeDeleteSubstitute, "substitute", "", "Substitute issue type ID or name for reassigning issues")
	deleteCmd.Flags().BoolVar(&issueTypeDeleteYes, "yes", false, "Skip confirmation prompt")
}

func runIssueTypeDelete(c *cobra.Command, args []string) error {
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

	// 削除対象の種別を取得
	issueType, err := cmdutil.ResolveIssueType(ctx, client, projectKey, idOrName)
	if err != nil {
		return fmt.Errorf("failed to resolve issue type: %w", err)
	}

	// すべての種別を取得（付け替え先の選択肢用）
	allIssueTypes, err := client.GetIssueTypes(ctx, projectKey)
	if err != nil {
		return fmt.Errorf("failed to get issue types: %w", err)
	}

	// 削除対象以外の種別を選択肢として作成
	var otherTypes []struct {
		ID   int
		Name string
	}
	for _, it := range allIssueTypes {
		if it.ID != issueType.ID {
			otherTypes = append(otherTypes, struct {
				ID   int
				Name string
			}{it.ID, it.Name})
		}
	}

	if len(otherTypes) == 0 {
		return fmt.Errorf("cannot delete: no other issue types to reassign issues to")
	}

	substituteID := 0

	// 対話モード: 付け替え先
	if issueTypeDeleteSubstitute == "" {
		if !ui.IsInteractiveInput() {
			return cmdutil.NonInteractiveFlagError(
				"--substitute is required when not running interactively",
				"backlog issue-type delete",
				"Use --substitute <issue-type-id|issue-type-name> to choose the replacement type.",
			)
		}
		options := make([]string, len(otherTypes))
		for i, t := range otherTypes {
			options[i] = fmt.Sprintf("%s (ID: %d)", t.Name, t.ID)
		}

		prompt := &survey.Select{
			Message: "課題の付け替え先を選択:",
			Options: options,
		}
		var selected string
		if err := survey.AskOne(prompt, &selected); err != nil {
			return err
		}

		// 選択された付け替え先のIDを取得
		for i, opt := range options {
			if opt == selected {
				substituteID = otherTypes[i].ID
				break
			}
		}
	} else {
		resolvedSubstitute, err := cmdutil.ResolveIssueType(ctx, client, projectKey, issueTypeDeleteSubstitute)
		if err != nil {
			return fmt.Errorf("failed to resolve substitute issue type: %w", err)
		}
		substituteID = resolvedSubstitute.ID
	}
	if substituteID == issueType.ID {
		return fmt.Errorf("substitute issue type must be different from the issue type being deleted")
	}

	// 確認プロンプト
	if !issueTypeDeleteYes && !ui.AssumeYes() {
		if !ui.IsInteractiveInput() {
			return cmdutil.NonInteractiveFlagError(
				"--yes is required when not running interactively",
				"backlog issue-type delete",
				"Use --yes to skip the confirmation prompt.",
			)
		}
		var substituteName string
		for _, t := range otherTypes {
			if t.ID == substituteID {
				substituteName = t.Name
				break
			}
		}

		fmt.Printf("種別 \"%s\" を削除します。\n", ui.Bold(issueType.Name))
		fmt.Printf("紐づいている課題は \"%s\" に付け替えられます。\n", ui.Bold(substituteName))

		var confirm bool
		confirmPrompt := &survey.Confirm{
			Message: "本当に削除しますか？",
			Default: false,
		}
		if err := survey.AskOne(confirmPrompt, &confirm); err != nil {
			return err
		}

		if !confirm {
			ui.Warning("キャンセルしました")
			return nil
		}
	}

	// 種別削除
	deletedIssueType, err := client.DeleteIssueType(ctx, projectKey, issueType.ID, substituteID)
	if err != nil {
		return fmt.Errorf("failed to delete issue type: %w", err)
	}

	// 出力
	profile := cfg.CurrentProfile()
	switch profile.Output {
	case "json":
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(deletedIssueType)
	default:
		ui.Success("種別を削除しました: %s (ID: %d)", deletedIssueType.Name, deletedIssueType.ID)
		return nil
	}
}
