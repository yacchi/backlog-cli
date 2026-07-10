package issue

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/yacchi/backlog-cli/packages/backlog/internal/api"
	"github.com/yacchi/backlog-cli/packages/backlog/internal/cmdutil"
	"github.com/yacchi/backlog-cli/packages/backlog/internal/config"
	"github.com/yacchi/backlog-cli/packages/backlog/internal/gen/backlog"
	"github.com/yacchi/backlog-cli/packages/backlog/internal/ui"
)

var editCmd = &cobra.Command{
	Use:   "edit <issue-key>",
	Short: "Edit an issue",
	Long: `Edit an existing issue.

Examples:
  # Update title and body
  backlog issue edit PROJ-123 --title "New title" --body "Updated description"

  # Read body from file
  backlog issue edit PROJ-123 --body-file description.md

  # Read body from stdin
  cat new-description.md | backlog issue edit PROJ-123 --body-file -

  # Assign to yourself
  backlog issue edit PROJ-123 --assignee @me

  # Change status
  backlog issue edit PROJ-123 --status 2

  # Patch description (search-and-replace via JSON)
  backlog issue edit PROJ-123 --patch '{"find":"old text","replace":"new text"}'
  backlog issue edit PROJ-123 --patch '[{"find":"A","replace":"A2"},{"find":"B","replace":"B2"}]'

  # Append/Prepend to description
  backlog issue edit PROJ-123 --append "Additional notes"
  backlog issue edit PROJ-123 --prepend "> Updated 2024-01-01"

  # Safe body replacement with conflict detection
  backlog issue edit PROJ-123 --body "New body" --safe`,
	Args: cobra.ExactArgs(1),
	RunE: runEdit,
}

var (
	editTitle            string
	editBody             string
	editBodyFile         string
	editStatusID         int
	editPriority         int
	editAssignee         string
	editDueDate          string
	editComment          string
	editMilestones       string
	editCategories       string
	editAddCategories    string
	editRemoveCategories string
	editRemoveMilestone  bool
	editAttachFiles      []string
	editSafe             bool
	editPatch            string
	editPatchFile        string
	editAppend           string
	editPrepend          string
)

func init() {
	editCmd.Flags().StringVarP(&editTitle, "title", "t", "", "Set the new title (summary)")
	editCmd.Flags().StringVarP(&editBody, "body", "b", "", "Set the new body (description)")
	editCmd.Flags().StringVarP(&editBodyFile, "body-file", "F", "", "Read body text from file (use \"-\" to read from standard input)")
	editCmd.Flags().IntVar(&editStatusID, "status", 0, "Status ID")
	editCmd.Flags().IntVar(&editPriority, "priority", 0, "Priority ID")
	editCmd.Flags().StringVarP(&editAssignee, "assignee", "a", "", "Assignee (user ID, userId, display name, or @me)")
	editCmd.Flags().StringVar(&editDueDate, "due", "", "Due date (YYYY-MM-DD)")
	editCmd.Flags().StringVarP(&editComment, "comment", "c", "", "Comment to add")
	editCmd.Flags().StringVarP(&editMilestones, "milestone", "m", "", "Milestone IDs or names (comma-separated)")
	editCmd.Flags().StringVar(&editCategories, "category", "", "Category IDs or names (comma-separated)")
	editCmd.Flags().StringVar(&editAddCategories, "add-category", "", "Add categories by name (comma-separated)")
	editCmd.Flags().StringVar(&editRemoveCategories, "remove-category", "", "Remove categories by name (comma-separated)")
	editCmd.Flags().BoolVar(&editRemoveMilestone, "remove-milestone", false, "Remove milestone from the issue")
	editCmd.Flags().StringArrayVar(&editAttachFiles, "attach", nil, "Attach local file(s) by path (can be specified multiple times)")
	editCmd.Flags().BoolVar(&editSafe, "safe", false, "Use conflict detection with three-way merge for body replacement")
	editCmd.Flags().StringVar(&editPatch, "patch", "", "Search-and-replace as JSON: {\"find\":\"...\",\"replace\":\"...\"} or array")
	editCmd.Flags().StringVar(&editPatchFile, "patch-file", "", "Read patch JSON from file (use \"-\" for stdin)")
	editCmd.Flags().StringVar(&editAppend, "append", "", "Text to append to description")
	editCmd.Flags().StringVar(&editPrepend, "prepend", "", "Text to prepend to description")
}

func runEdit(c *cobra.Command, args []string) error {
	issueKey := args[0]

	client, cfg, err := cmdutil.GetAPIClient(c)
	if err != nil {
		return err
	}

	hasPatchFlags := editPatch != "" || editPatchFile != "" || editAppend != "" || editPrepend != ""

	// Description patch mode
	if hasPatchFlags || editSafe {
		return runEditWithPatch(c, args)
	}

	input := &api.UpdateIssueInput{}
	hasUpdate := false
	resolvedKey, projectKey := cmdutil.ResolveIssueKey(issueKey, cmdutil.GetCurrentProject(cfg))

	if editTitle != "" {
		input.Summary = &editTitle
		hasUpdate = true
	}

	// body/body-file の処理
	if editBody != "" || editBodyFile != "" {
		body, err := cmdutil.ResolveBody(editBody, editBodyFile, false, nil, nil)
		if err != nil {
			return fmt.Errorf("failed to read body: %w", err)
		}
		if body != "" {
			input.Description = &body
			hasUpdate = true
		}
	}

	if editStatusID > 0 {
		input.StatusID = &editStatusID
		hasUpdate = true
	}
	if editPriority > 0 {
		input.PriorityID = &editPriority
		hasUpdate = true
	}
	if editDueDate != "" {
		input.DueDate = &editDueDate
		hasUpdate = true
	}
	if editComment != "" {
		input.Comment = &editComment
		hasUpdate = true
	}

	// 担当者
	ctx := c.Context()
	if editAssignee != "" {
		assigneeID, err := cmdutil.ResolveProjectAssigneeID(ctx, client, projectKey, editAssignee)
		if err != nil {
			return fmt.Errorf("failed to resolve assignee: %w", err)
		}
		input.AssigneeID = &assigneeID
		hasUpdate = true
	}

	// 差分操作が必要な場合は現在の課題情報を取得
	needsFetch := editAddCategories != "" || editRemoveCategories != "" || editRemoveMilestone
	var currentIssue *backlog.Issue
	if needsFetch {
		var err error
		currentIssue, err = client.GetIssue(ctx, resolvedKey)
		if err != nil {
			return fmt.Errorf("failed to get current issue: %w", err)
		}
	}

	// マイルストーン
	if editRemoveMilestone {
		// 空配列を設定してマイルストーンを解除
		input.MilestoneIDs = []int{}
		hasUpdate = true
	} else if editMilestones != "" {
		milestoneIDs, err := cmdutil.ResolveMilestoneIDs(ctx, client, projectKey, editMilestones)
		if err != nil {
			return fmt.Errorf("failed to resolve milestones: %w", err)
		}
		input.MilestoneIDs = milestoneIDs
		hasUpdate = true
	}

	// カテゴリ（完全置換）
	if editCategories != "" {
		categoryIDs, err := cmdutil.ResolveCategoryIDs(ctx, client, projectKey, editCategories)
		if err != nil {
			return fmt.Errorf("failed to resolve categories: %w", err)
		}
		input.CategoryIDs = categoryIDs
		hasUpdate = true
	}

	// カテゴリ追加（差分操作）
	if editAddCategories != "" {
		addIDs, err := cmdutil.ResolveCategoryIDs(ctx, client, projectKey, editAddCategories)
		if err != nil {
			return fmt.Errorf("failed to resolve categories to add: %w", err)
		}
		// 現在のカテゴリIDを取得
		currentIDs := make(map[int]bool)
		for _, cat := range currentIssue.Category {
			if cat.ID.IsSet() {
				currentIDs[cat.ID.Value] = true
			}
		}
		// 追加するIDをマージ
		for _, id := range addIDs {
			currentIDs[id] = true
		}
		// mapをsliceに変換
		result := make([]int, 0, len(currentIDs))
		for id := range currentIDs {
			result = append(result, id)
		}
		input.CategoryIDs = result
		hasUpdate = true
	}

	// カテゴリ削除（差分操作）
	if editRemoveCategories != "" {
		removeIDs, err := cmdutil.ResolveCategoryIDs(ctx, client, projectKey, editRemoveCategories)
		if err != nil {
			return fmt.Errorf("failed to resolve categories to remove: %w", err)
		}
		// 削除するIDをset化
		removeSet := make(map[int]bool)
		for _, id := range removeIDs {
			removeSet[id] = true
		}
		// 現在のカテゴリから削除対象を除外
		var result []int
		for _, cat := range currentIssue.Category {
			if cat.ID.IsSet() && !removeSet[cat.ID.Value] {
				result = append(result, cat.ID.Value)
			}
		}
		input.CategoryIDs = result
		hasUpdate = true
	}

	// 添付ファイルのアップロード
	if len(editAttachFiles) > 0 {
		attachmentIDs, err := cmdutil.UploadFiles(ctx, client, editAttachFiles)
		if err != nil {
			return err
		}
		input.AttachmentIDs = attachmentIDs
		hasUpdate = true
	}

	if !hasUpdate {
		return fmt.Errorf("no updates specified")
	}

	issue, err := client.UpdateIssue(ctx, issueKey, input)
	if err != nil {
		return fmt.Errorf("failed to update issue: %w", err)
	}

	return printIssueEditResult(cfg, issue, false)
}

func runEditWithPatch(c *cobra.Command, args []string) error {
	issueKey := args[0]

	client, cfg, err := cmdutil.GetAPIClient(c)
	if err != nil {
		return err
	}

	resolvedKey, _ := cmdutil.ResolveIssueKey(issueKey, cmdutil.GetCurrentProject(cfg))
	ctx := c.Context()

	// Parse patch ops
	var patchOps []api.PatchOp
	if editPatch != "" || editPatchFile != "" {
		patchJSON, err := cmdutil.ResolveBody(editPatch, editPatchFile, false, nil, nil)
		if err != nil {
			return fmt.Errorf("failed to read patch: %w", err)
		}
		patchOps, err = cmdutil.ParsePatchOps(patchJSON)
		if err != nil {
			return err
		}
	}

	// Safe full replacement content
	var fullReplace string
	if editSafe && (editBody != "" || editBodyFile != "") {
		body, err := cmdutil.ResolveBody(editBody, editBodyFile, false, nil, nil)
		if err != nil {
			return fmt.Errorf("failed to read body: %w", err)
		}
		fullReplace = body
	}

	patchFn, err := cmdutil.BuildPatchFn(patchOps, editPrepend, editAppend, fullReplace)
	if err != nil {
		return err
	}

	issue, merged, err := client.SafeUpdateIssueDescription(ctx, resolvedKey, patchFn)
	if err != nil {
		var conflictErr *api.ConflictError
		if errors.As(err, &conflictErr) {
			fmt.Fprintf(os.Stderr, "%s %s\n", ui.Red("✗"), conflictErr.Error())
			fmt.Fprintf(os.Stderr, "  Hint: resolve the conflict manually, or use --body without --safe to force overwrite.\n")
			return err
		}
		return fmt.Errorf("failed to update issue: %w", err)
	}

	// Apply non-description updates if any
	hasOtherUpdates := editTitle != "" || editStatusID > 0 || editPriority > 0 ||
		editDueDate != "" || editComment != "" || editAssignee != "" ||
		editMilestones != "" || editCategories != "" || editAddCategories != "" ||
		editRemoveCategories != "" || editRemoveMilestone || len(editAttachFiles) > 0
	if hasOtherUpdates {
		input := &api.UpdateIssueInput{}
		_, projectKey := cmdutil.ResolveIssueKey(issueKey, cmdutil.GetCurrentProject(cfg))

		if editTitle != "" {
			input.Summary = &editTitle
		}
		if editStatusID > 0 {
			input.StatusID = &editStatusID
		}
		if editPriority > 0 {
			input.PriorityID = &editPriority
		}
		if editDueDate != "" {
			input.DueDate = &editDueDate
		}
		if editComment != "" {
			input.Comment = &editComment
		}
		if editAssignee != "" {
			assigneeID, err := cmdutil.ResolveProjectAssigneeID(ctx, client, projectKey, editAssignee)
			if err != nil {
				return fmt.Errorf("description patched but failed to resolve assignee: %w", err)
			}
			input.AssigneeID = &assigneeID
		}
		if editRemoveMilestone {
			input.MilestoneIDs = []int{}
		} else if editMilestones != "" {
			milestoneIDs, err := cmdutil.ResolveMilestoneIDs(ctx, client, projectKey, editMilestones)
			if err != nil {
				return fmt.Errorf("failed to resolve milestones: %w", err)
			}
			input.MilestoneIDs = milestoneIDs
		}
		if editCategories != "" {
			categoryIDs, err := cmdutil.ResolveCategoryIDs(ctx, client, projectKey, editCategories)
			if err != nil {
				return fmt.Errorf("failed to resolve categories: %w", err)
			}
			input.CategoryIDs = categoryIDs
		}
		if len(editAttachFiles) > 0 {
			attachmentIDs, err := cmdutil.UploadFiles(ctx, client, editAttachFiles)
			if err != nil {
				return err
			}
			input.AttachmentIDs = attachmentIDs
		}

		issue, err = client.UpdateIssue(ctx, resolvedKey, input)
		if err != nil {
			return fmt.Errorf("description patched but failed to update other fields: %w", err)
		}
	}

	return printIssueEditResult(cfg, issue, merged)
}

func printIssueEditResult(cfg *config.Store, issue *backlog.Issue, merged bool) error {
	profile := cfg.CurrentProfile()
	switch profile.Output {
	case "json":
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(issue)
	default:
		if merged {
			ui.Success("Updated %s (auto-merged)", issue.IssueKey.Value)
		} else {
			ui.Success("Updated %s", issue.IssueKey.Value)
		}
		url := fmt.Sprintf("https://%s/view/%s", profile.Space, issue.IssueKey.Value)
		fmt.Printf("URL: %s\n", ui.Cyan(url))
		return nil
	}
}
