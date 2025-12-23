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
	editTitle    string
	editBody     string
	editBodyFile string
	editStatusID int
	editPriority int
	editAssignee string
	editDueDate  string
	editComment  string
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
