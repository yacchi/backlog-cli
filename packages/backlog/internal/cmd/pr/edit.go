package pr

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
	Use:   "edit <number>",
	Short: "Edit a pull request",
	Long: `Edit an existing pull request.

Examples:
  backlog pr edit 123 --repo myrepo --title "New title"
  backlog pr edit 123 --repo myrepo --body "Updated description"
  backlog pr edit 123 --repo myrepo --assignee 12345`,
	Args: cobra.ExactArgs(1),
	RunE: runEdit,
}

var (
	editRepo       string
	editTitle      string
	editBody       string
	editAssigneeID int
	editIssueID    int
)

func init() {
	editCmd.Flags().StringVarP(&editRepo, "repo", "R", "", "Repository name (required)")
	editCmd.Flags().StringVarP(&editTitle, "title", "t", "", "New title (summary)")
	editCmd.Flags().StringVarP(&editBody, "body", "b", "", "New description")
	editCmd.Flags().IntVar(&editAssigneeID, "assignee", 0, "Assignee user ID")
	editCmd.Flags().IntVar(&editIssueID, "issue", 0, "Related issue ID")
	_ = editCmd.MarkFlagRequired("repo")
}

func runEdit(c *cobra.Command, args []string) error {
	number, err := strconv.Atoi(args[0])
	if err != nil {
		return fmt.Errorf("invalid pull request number: %s", args[0])
	}

	client, cfg, err := cmdutil.GetAPIClient(c)
	if err != nil {
		return err
	}

	if err := cmdutil.RequireProject(cfg); err != nil {
		return err
	}

	profile := cfg.CurrentProfile()
	projectKey := cmdutil.GetCurrentProject(cfg)

	// 更新入力を構築
	input := &api.UpdatePullRequestInput{}
	hasChanges := false

	if c.Flags().Changed("title") {
		input.Summary = &editTitle
		hasChanges = true
	}
	if c.Flags().Changed("body") {
		input.Description = &editBody
		hasChanges = true
	}
	if c.Flags().Changed("assignee") {
		input.AssigneeID = &editAssigneeID
		hasChanges = true
	}
	if c.Flags().Changed("issue") {
		input.IssueID = &editIssueID
		hasChanges = true
	}

	if !hasChanges {
		return fmt.Errorf("no changes specified. Use --title, --body, --assignee, or --issue")
	}

	// 更新実行
	pr, err := client.UpdatePullRequest(c.Context(), projectKey, editRepo, number, input)
	if err != nil {
		return fmt.Errorf("failed to update pull request: %w", err)
	}

	// 出力
	switch profile.Output {
	case "json":
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(pr)
	default:
		fmt.Printf("%s Pull request updated: #%d %s\n",
			ui.Green("✓"), pr.Number, pr.Summary)
		return nil
	}
}
