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

var mergeCmd = &cobra.Command{
	Use:   "merge <number>",
	Short: "Merge a pull request",
	Long: `Merge an open pull request.

Examples:
  backlog pr merge 123 --repo myrepo
  backlog pr merge 123 --repo myrepo --yes  # Skip confirmation`,
	Args: cobra.ExactArgs(1),
	RunE: runMerge,
}

var (
	mergeRepo    string
	mergeYes     bool
	mergeComment string
)

func init() {
	mergeCmd.Flags().StringVarP(&mergeRepo, "repo", "R", "", "Repository name (required)")
	mergeCmd.Flags().BoolVar(&mergeYes, "yes", false, "Skip confirmation prompt")
	mergeCmd.Flags().StringVarP(&mergeComment, "comment", "c", "", "Add a comment when merging")
	_ = mergeCmd.MarkFlagRequired("repo")
}

func runMerge(c *cobra.Command, args []string) error {
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
	pr, err := client.GetPullRequest(ctx, projectKey, mergeRepo, number)
	if err != nil {
		return fmt.Errorf("failed to get pull request: %w", err)
	}

	// 既にクローズ/マージ済みの場合はエラー
	if pr.Status.ID != 1 { // 1 = Open
		return fmt.Errorf("pull request #%d is already %s", number, pr.Status.Name)
	}

	// 確認プロンプト
	if !mergeYes {
		var confirm bool
		prompt := &survey.Confirm{
			Message: fmt.Sprintf("Are you sure you want to merge PR #%d: %s?", pr.Number, pr.Summary),
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

	// マージ実行
	input := &api.MergePullRequestInput{
		Comment: mergeComment,
	}
	merged, err := client.MergePullRequest(ctx, projectKey, mergeRepo, number, input)
	if err != nil {
		return fmt.Errorf("failed to merge pull request: %w", err)
	}

	// 出力
	switch profile.Output {
	case "json":
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(merged)
	default:
		fmt.Printf("%s Pull request merged: #%d %s\n",
			ui.Green("✓"), merged.Number, merged.Summary)
		return nil
	}
}
