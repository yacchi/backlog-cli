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

  # Interactive mode
  backlog pr comment 123 --repo myrepo`,
	Args: cobra.ExactArgs(1),
	RunE: runComment,
}

var (
	commentRepo string
	commentBody string
)

func init() {
	commentCmd.Flags().StringVarP(&commentRepo, "repo", "R", "", "Repository name (required)")
	commentCmd.Flags().StringVarP(&commentBody, "body", "b", "", "Comment body")
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

	// 対話モード: bodyが未指定の場合
	if commentBody == "" {
		prompt := &survey.Multiline{
			Message: "Comment body:",
		}
		if err := survey.AskOne(prompt, &commentBody, survey.WithValidator(survey.Required)); err != nil {
			return err
		}
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
