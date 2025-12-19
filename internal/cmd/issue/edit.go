package issue

import (
	"fmt"
	"strconv"

	"github.com/spf13/cobra"
	"github.com/yacchi/backlog-cli/internal/api"
	"github.com/yacchi/backlog-cli/internal/cmdutil"
	"github.com/yacchi/backlog-cli/internal/ui"
)

var editCmd = &cobra.Command{
	Use:   "edit <issue-key>",
	Short: "Edit an issue",
	Long: `Edit an existing issue.

Examples:
  backlog issue edit PROJ-123 --summary "New title"
  backlog issue edit PROJ-123 --assignee @me
  backlog issue edit PROJ-123 --status 2`,
	Args: cobra.ExactArgs(1),
	RunE: runEdit,
}

var (
	editSummary     string
	editDescription string
	editStatusID    int
	editPriorityID  int
	editAssignee    string
	editDueDate     string
	editComment     string
)

func init() {
	editCmd.Flags().StringVarP(&editSummary, "summary", "s", "", "New summary")
	editCmd.Flags().StringVarP(&editDescription, "description", "d", "", "New description")
	editCmd.Flags().IntVar(&editStatusID, "status", 0, "Status ID")
	editCmd.Flags().IntVar(&editPriorityID, "priority", 0, "Priority ID")
	editCmd.Flags().StringVarP(&editAssignee, "assignee", "a", "", "Assignee (user ID or @me)")
	editCmd.Flags().StringVar(&editDueDate, "due", "", "Due date (YYYY-MM-DD)")
	editCmd.Flags().StringVarP(&editComment, "comment", "c", "", "Comment to add")
}

func runEdit(c *cobra.Command, args []string) error {
	issueKey := args[0]

	client, _, err := cmdutil.GetAPIClient(c)
	if err != nil {
		return err
	}

	input := &api.UpdateIssueInput{}
	hasUpdate := false

	if editSummary != "" {
		input.Summary = &editSummary
		hasUpdate = true
	}
	if editDescription != "" {
		input.Description = &editDescription
		hasUpdate = true
	}
	if editStatusID > 0 {
		input.StatusID = &editStatusID
		hasUpdate = true
	}
	if editPriorityID > 0 {
		input.PriorityID = &editPriorityID
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
	if editAssignee == "@me" {
		me, err := client.GetCurrentUser()
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

	if !hasUpdate {
		return fmt.Errorf("no updates specified")
	}

	issue, err := client.UpdateIssue(issueKey, input)
	if err != nil {
		return fmt.Errorf("failed to update issue: %w", err)
	}

	ui.Success("Updated %s", issue.IssueKey)
	return nil
}
