package pr

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/AlecAivazis/survey/v2"
	"github.com/spf13/cobra"
	"github.com/yacchi/backlog-cli/packages/backlog/internal/api"
	"github.com/yacchi/backlog-cli/packages/backlog/internal/cmdutil"
	"github.com/yacchi/backlog-cli/packages/backlog/internal/ui"
)

var createCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a pull request",
	Long: `Create a new pull request.

Examples:
  # Create a PR with all options
  backlog pr create --repo myrepo --base main --head feature/xxx --title "My PR" --body "Description"

  # Interactive mode
  backlog pr create --repo myrepo

  # Minimal (will prompt for missing fields)
  backlog pr create --repo myrepo --base main --head feature/xxx`,
	RunE: runCreate,
}

var (
	createRepo       string
	createBase       string
	createHead       string
	createTitle      string
	createBody       string
	createIssueID    int
	createAssigneeID int
	createReviewers  string
)

func init() {
	createCmd.Flags().StringVarP(&createRepo, "repo", "R", "", "Repository name (required)")
	createCmd.Flags().StringVarP(&createBase, "base", "B", "", "Base branch (merge target)")
	createCmd.Flags().StringVarP(&createHead, "head", "H", "", "Head branch (branch to merge)")
	createCmd.Flags().StringVarP(&createTitle, "title", "t", "", "Pull request title")
	createCmd.Flags().StringVarP(&createBody, "body", "b", "", "Pull request description")
	createCmd.Flags().IntVar(&createIssueID, "issue", 0, "Related issue ID")
	createCmd.Flags().IntVar(&createAssigneeID, "assignee", 0, "Assignee user ID")
	createCmd.Flags().StringVar(&createReviewers, "reviewer", "", "Reviewer user IDs (comma-separated)")
	_ = createCmd.MarkFlagRequired("repo")
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

	// 対話モード: 必須フィールドが未指定の場合
	if createBase == "" {
		prompt := &survey.Input{
			Message: "Base branch (merge target):",
			Default: "main",
		}
		if err := survey.AskOne(prompt, &createBase, survey.WithValidator(survey.Required)); err != nil {
			return err
		}
	}

	if createHead == "" {
		prompt := &survey.Input{
			Message: "Head branch (branch to merge):",
		}
		if err := survey.AskOne(prompt, &createHead, survey.WithValidator(survey.Required)); err != nil {
			return err
		}
	}

	if createTitle == "" {
		prompt := &survey.Input{
			Message: "Pull request title:",
		}
		if err := survey.AskOne(prompt, &createTitle, survey.WithValidator(survey.Required)); err != nil {
			return err
		}
	}

	if createBody == "" {
		prompt := &survey.Multiline{
			Message: "Pull request description:",
		}
		if err := survey.AskOne(prompt, &createBody); err != nil {
			return err
		}
	}

	// レビュアーID解析
	var reviewerIDs []int
	if createReviewers != "" {
		parts := strings.Split(createReviewers, ",")
		for _, part := range parts {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			id, err := strconv.Atoi(part)
			if err != nil {
				return fmt.Errorf("invalid reviewer ID: %s", part)
			}
			reviewerIDs = append(reviewerIDs, id)
		}
	}

	// PR作成
	input := &api.CreatePullRequestInput{
		Summary:         createTitle,
		Description:     createBody,
		Base:            createBase,
		Branch:          createHead,
		IssueID:         createIssueID,
		AssigneeID:      createAssigneeID,
		NotifiedUserIDs: reviewerIDs,
	}

	pr, err := client.CreatePullRequest(c.Context(), projectKey, createRepo, input)
	if err != nil {
		return fmt.Errorf("failed to create pull request: %w", err)
	}

	// 出力
	switch profile.Output {
	case "json":
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(pr)
	default:
		fmt.Printf("%s Pull request created: #%d %s\n",
			ui.Green("✓"), pr.Number, pr.Summary)
		url := fmt.Sprintf("https://%s.%s/git/%s/%s/pullRequests/%d",
			profile.Space, profile.Domain, projectKey, createRepo, pr.Number)
		fmt.Printf("URL: %s\n", ui.Cyan(url))
		return nil
	}
}
