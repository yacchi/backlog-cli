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

var closeCmd = &cobra.Command{
	Use:   "close <number>",
	Short: "Close a pull request",
	Long: `Close an open pull request without merging.

Examples:
  backlog pr close 123 --repo myrepo
  backlog pr close 123 --repo myrepo --yes  # Skip confirmation`,
	Args: cobra.ExactArgs(1),
	RunE: runClose,
}

var (
	closeRepo         string
	closeYes          bool
	closeComment      string
	closeDeleteBranch bool
)

func init() {
	closeCmd.Flags().StringVarP(&closeRepo, "repo", "R", "", "Repository name (required)")
	closeCmd.Flags().BoolVar(&closeYes, "yes", false, "Skip confirmation prompt")
	closeCmd.Flags().StringVarP(&closeComment, "comment", "c", "", "Add a comment when closing")
	closeCmd.Flags().BoolVarP(&closeDeleteBranch, "delete-branch", "d", false, "Delete the head branch (not supported by Backlog API)")
	_ = closeCmd.MarkFlagRequired("repo")
}

func runClose(c *cobra.Command, args []string) error {
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
	ctx := c.Context()

	// まずPRの現在の状態を取得
	pr, err := client.GetPullRequest(ctx, projectKey, closeRepo, number)
	if err != nil {
		return fmt.Errorf("failed to get pull request: %w", err)
	}

	// 既にクローズ/マージ済みの場合はエラー
	if pr.Status.ID != 1 { // 1 = Open
		return fmt.Errorf("pull request #%d is already %s", number, pr.Status.Name)
	}

	// 確認プロンプト
	if !closeYes {
		var confirm bool
		prompt := &survey.Confirm{
			Message: fmt.Sprintf("Are you sure you want to close PR #%d: %s?", pr.Number, pr.Summary),
			Default: false,
		}
		if err := survey.AskOne(prompt, &confirm); err != nil {
			return err
		}
		if !confirm {
			fmt.Println("Aborted")
			return nil
		}
	}

	// --delete-branchはBacklog APIでサポートされていないため警告
	if closeDeleteBranch {
		fmt.Fprintf(os.Stderr, "%s --delete-branch is not supported by Backlog API. The branch will not be deleted.\n", ui.Yellow("Warning:"))
	}

	// クローズ実行
	input := &api.ClosePullRequestInput{
		Comment: closeComment,
	}
	closed, err := client.ClosePullRequest(ctx, projectKey, closeRepo, number, input)
	if err != nil {
		return fmt.Errorf("failed to close pull request: %w", err)
	}

	// 出力
	switch profile.Output {
	case "json":
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(closed)
	default:
		fmt.Printf("%s Pull request closed: #%d %s\n",
			ui.Green("✓"), closed.Number, closed.Summary)
		return nil
	}
}
