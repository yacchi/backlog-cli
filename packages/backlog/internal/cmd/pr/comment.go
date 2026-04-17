package pr

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"

	"github.com/AlecAivazis/survey/v2"
	"github.com/spf13/cobra"
	"github.com/yacchi/backlog-cli/packages/backlog/internal/api"
	"github.com/yacchi/backlog-cli/packages/backlog/internal/cmdutil"
	"github.com/yacchi/backlog-cli/packages/backlog/internal/ui"
)

var commentCmd = &cobra.Command{
	Use:   "comment <pr-number>",
	Short: "Add a comment to a pull request",
	Long: `Add a comment to a pull request.

Examples:
  # Add a comment with body
  backlog pr comment 123 --repo myrepo --body "LGTM!"

  # Read comment from file
  backlog pr comment 123 --repo myrepo --body-file review.md

  # Read comment from stdin
  cat comment.md | backlog pr comment 123 --repo myrepo --body-file -

  # Interactive mode
  backlog pr comment 123 --repo myrepo`,
	Args: cobra.ExactArgs(1),
	RunE: runComment,
}

var (
	commentRepo     string
	commentBody     string
	commentBodyFile string
)

func init() {
	commentCmd.Flags().StringVarP(&commentRepo, "repo", "R", "", "Repository name (required)")
	commentCmd.Flags().StringVarP(&commentBody, "body", "b", "", "Comment body")
	commentCmd.Flags().StringVarP(&commentBodyFile, "body-file", "F", "", "Read body text from file (use \"-\" to read from standard input)")
	_ = commentCmd.MarkFlagRequired("repo")
}

func runComment(c *cobra.Command, args []string) error {
	client, cfg, err := cmdutil.GetAPIClient(c)
	if err != nil {
		return err
	}

	if err := cmdutil.RequireProject(cfg); err != nil {
		return err
	}

	prNumber, err := strconv.Atoi(args[0])
	if err != nil {
		return fmt.Errorf("invalid pull request number: %s", args[0])
	}

	projectKey := cmdutil.GetCurrentProject(cfg)
	profile := cfg.CurrentProfile()

	// body解決: body > body-file > interactive
	commentBody, err = cmdutil.ResolveBody(
		commentBody,
		commentBodyFile,
		false,
		nil,
		func() (string, error) {
			var body string
			prompt := &survey.Multiline{
				Message: "Comment body:",
			}
			if err := survey.AskOne(prompt, &body, survey.WithValidator(survey.Required)); err != nil {
				return "", err
			}
			return body, nil
		},
	)
	if err != nil {
		return fmt.Errorf("failed to get comment body: %w", err)
	}

	// コメント追加
	input := &api.AddPRCommentInput{
		Content: commentBody,
	}

	comment, err := client.AddPullRequestComment(c.Context(), projectKey, commentRepo, prNumber, input)
	if err != nil {
		return fmt.Errorf("failed to add comment: %w", err)
	}

	// 出力
	switch profile.Output {
	case "json":
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(comment)
	default:
		fmt.Printf("%s Comment added to PR #%d\n", ui.Green("✓"), prNumber)
		url := fmt.Sprintf("https://%s.%s/git/%s/%s/pullRequests/%d#comment-%d",
			profile.Space, profile.Domain, projectKey, commentRepo, prNumber, comment.ID)
		fmt.Printf("URL: %s\n", ui.Cyan(url))
		return nil
	}
}
