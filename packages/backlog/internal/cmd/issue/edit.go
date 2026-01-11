package issue

import (
	"fmt"
	"strconv"

	"github.com/spf13/cobra"
	"github.com/yacchi/backlog-cli/packages/backlog/internal/api"
	"github.com/yacchi/backlog-cli/packages/backlog/internal/cmdutil"
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
  backlog issue edit PROJ-123 --status 2`,
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
)

func init() {
	editCmd.Flags().StringVarP(&editTitle, "title", "t", "", "Set the new title (summary)")
	editCmd.Flags().StringVarP(&editBody, "body", "b", "", "Set the new body (description)")
	editCmd.Flags().StringVarP(&editBodyFile, "body-file", "F", "", "Read body text from file (use \"-\" to read from standard input)")
	editCmd.Flags().IntVar(&editStatusID, "status", 0, "Status ID")
	editCmd.Flags().IntVar(&editPriority, "priority", 0, "Priority ID")
	editCmd.Flags().StringVarP(&editAssignee, "assignee", "a", "", "Assignee (user ID or @me)")
	editCmd.Flags().StringVar(&editDueDate, "due", "", "Due date (YYYY-MM-DD)")
	editCmd.Flags().StringVarP(&editComment, "comment", "c", "", "Comment to add")
	editCmd.Flags().StringVarP(&editMilestones, "milestone", "m", "", "Milestone IDs or names (comma-separated)")
	editCmd.Flags().StringVar(&editCategories, "category", "", "Category IDs or names (comma-separated)")
	editCmd.Flags().StringVar(&editAddCategories, "add-category", "", "Add categories by name (comma-separated)")
	editCmd.Flags().StringVar(&editRemoveCategories, "remove-category", "", "Remove categories by name (comma-separated)")
	editCmd.Flags().BoolVar(&editRemoveMilestone, "remove-milestone", false, "Remove milestone from the issue")
}

func runEdit(c *cobra.Command, args []string) error {
	issueKey := args[0]

	client, cfg, err := cmdutil.GetAPIClient(c)
	if err != nil {
		return err
	}

	input := &api.UpdateIssueInput{}
	hasUpdate := false

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
	if editAssignee == "@me" {
		me, err := client.GetCurrentUser(ctx)
		if err != nil {
			return fmt.Errorf("failed to get current user: %w", err)
		}
		id := me.ID.Value
		input.AssigneeID = &id
		hasUpdate = true
	} else if editAssignee != "" {
		assigneeID, err := strconv.Atoi(editAssignee)
		if err != nil {
			return fmt.Errorf("invalid assignee ID: %s", editAssignee)
		}
		input.AssigneeID = &assigneeID
		hasUpdate = true
	}

	// マイルストーン・カテゴリの処理にはプロジェクトキーが必要
	resolvedKey, projectKey := cmdutil.ResolveIssueKey(issueKey, cmdutil.GetCurrentProject(cfg))

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
		milestoneIDs, err := resolveMilestoneIDs(ctx, client, projectKey, editMilestones)
		if err != nil {
			return fmt.Errorf("failed to resolve milestones: %w", err)
		}
		input.MilestoneIDs = milestoneIDs
		hasUpdate = true
	}

	// カテゴリ（完全置換）
	if editCategories != "" {
		categoryIDs, err := resolveCategoryIDs(ctx, client, projectKey, editCategories)
		if err != nil {
			return fmt.Errorf("failed to resolve categories: %w", err)
		}
		input.CategoryIDs = categoryIDs
		hasUpdate = true
	}

	// カテゴリ追加（差分操作）
	if editAddCategories != "" {
		addIDs, err := resolveCategoryIDs(ctx, client, projectKey, editAddCategories)
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
		removeIDs, err := resolveCategoryIDs(ctx, client, projectKey, editRemoveCategories)
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

	if !hasUpdate {
		return fmt.Errorf("no updates specified")
	}

	issue, err := client.UpdateIssue(ctx, issueKey, input)
	if err != nil {
		return fmt.Errorf("failed to update issue: %w", err)
	}

	ui.Success("Updated %s", issue.IssueKey)

	profile := cfg.CurrentProfile()
	url := fmt.Sprintf("https://%s.%s/view/%s", profile.Space, profile.Domain, issue.IssueKey.Value)
	fmt.Printf("URL: %s\n", ui.Cyan(url))

	return nil
}
