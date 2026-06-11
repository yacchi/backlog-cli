package pr

import (
	"encoding/json"
	"fmt"
	"os"
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

  # Read body from file
  backlog pr create --repo myrepo --base main --head feature/xxx --title "My PR" --body-file desc.md

  # Read body from stdin
  cat description.md | backlog pr create --repo myrepo --title "My PR" --body-file -

  # Interactive mode
  backlog pr create --repo myrepo

  # Minimal (will prompt for missing fields)
  backlog pr create --repo myrepo --base main --head feature/xxx`,
	RunE: runCreate,
}

var (
	createRepo      string
	createBase      string
	createHead      string
	createTitle     string
	createBody      string
	createBodyFile  string
	createIssueID   int
	createAssignee  string
	createReviewers string
)

func init() {
	createCmd.Flags().StringVarP(&createRepo, "repo", "R", "", "Repository name (required)")
	createCmd.Flags().StringVarP(&createBase, "base", "B", "", "Base branch (merge target)")
	createCmd.Flags().StringVarP(&createHead, "head", "H", "", "Head branch (branch to merge)")
	createCmd.Flags().StringVarP(&createTitle, "title", "t", "", "Pull request title")
	createCmd.Flags().StringVarP(&createBody, "body", "b", "", "Pull request description")
	createCmd.Flags().StringVarP(&createBodyFile, "body-file", "F", "", "Read body text from file (use \"-\" to read from standard input)")
	createCmd.Flags().IntVar(&createIssueID, "issue", 0, "Related issue ID")
	createCmd.Flags().StringVar(&createAssignee, "assignee", "", "Assignee (user ID, userId, display name, or @me)")
	createCmd.Flags().StringVar(&createReviewers, "reviewer", "", "Reviewer IDs, userIds, or display names (comma-separated)")
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
	interactive := ui.IsInteractiveInput()

	if !interactive {
		var missing []string
		if createBase == "" {
			missing = append(missing, "--base")
		}
		if createHead == "" {
			missing = append(missing, "--head")
		}
		if createTitle == "" {
			missing = append(missing, "--title")
		}
		if len(missing) > 0 {
			return cmdutil.NonInteractiveFlagError(
				fmt.Sprintf("%s required when not running interactively", strings.Join(missing, ", ")),
				"backlog pr create",
				"Use --base <branch>, --head <branch>, and --title <text>.",
			)
		}
	}

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

	var interactiveBodyInput func() (string, error)
	if interactive {
		interactiveBodyInput = func() (string, error) {
			var body string
			prompt := &survey.Multiline{
				Message: "Pull request description:",
			}
			if err := survey.AskOne(prompt, &body); err != nil {
				return "", err
			}
			return body, nil
		}
	}
	createBody, err = cmdutil.ResolveBody(
		createBody,
		createBodyFile,
		false,
		nil,
		interactiveBodyInput,
	)
	if err != nil {
		return fmt.Errorf("failed to get body: %w", err)
	}

	// レビュアーID解析
	var reviewerIDs []int
	if createReviewers != "" {
		reviewerIDs, err = cmdutil.ResolveProjectUserIDs(c.Context(), client, projectKey, createReviewers)
		if err != nil {
			return fmt.Errorf("failed to resolve reviewers: %w", err)
		}
	}

	assigneeID := 0
	if createAssignee != "" {
		assigneeID, err = cmdutil.ResolveProjectAssigneeID(c.Context(), client, projectKey, createAssignee)
		if err != nil {
			return fmt.Errorf("failed to resolve assignee: %w", err)
		}
	}

	// PR作成
	input := &api.CreatePullRequestInput{
		Summary:         createTitle,
		Description:     createBody,
		Base:            createBase,
		Branch:          createHead,
		IssueID:         createIssueID,
		AssigneeID:      assigneeID,
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
		url := fmt.Sprintf("https://%s/git/%s/%s/pullRequests/%d",
			profile.Space, projectKey, createRepo, pr.Number)
		fmt.Printf("URL: %s\n", ui.Cyan(url))
		return nil
	}
}
