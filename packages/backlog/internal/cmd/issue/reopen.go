package issue

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

var reopenCmd = &cobra.Command{
	Use:   "reopen <issue-key>",
	Short: "Reopen a closed issue",
	Long: `Reopen a closed issue by changing its status back to open.

Examples:
  backlog issue reopen PROJ-123
  backlog issue reopen PROJ-123 --comment "Reopening for further investigation"`,
	Args: cobra.ExactArgs(1),
	RunE: runReopen,
}

var reopenComment string

func init() {
	reopenCmd.Flags().StringVarP(&reopenComment, "comment", "c", "", "Add a reopening comment")
}

func runReopen(c *cobra.Command, args []string) error {
	issueKey := args[0]

	client, cfg, err := cmdutil.GetAPIClient(c)
	if err != nil {
		return err
	}

	// 課題キーの解決
	issueKey, _ = cmdutil.ResolveIssueKey(issueKey, cmdutil.GetCurrentProject(cfg))

	// 現在の課題を取得
	ctx := c.Context()
	issue, err := client.GetIssue(ctx, issueKey)
	if err != nil {
		return fmt.Errorf("failed to get issue: %w", err)
	}

	// プロジェクトのステータスを取得してOpenステータスを探す
	statuses, err := client.GetStatuses(ctx, strconv.Itoa(issue.ProjectId.Value))
	if err != nil {
		return fmt.Errorf("failed to get statuses: %w", err)
	}

	var openStatusID int
	for _, s := range statuses {
		// "未対応" または "Open" を探す
		if s.Name == "未対応" || s.Name == "Open" || s.Name == "To Do" {
			openStatusID = s.ID
			break
		}
	}

	if openStatusID == 0 {
		// 見つからない場合は最初のステータスを使用
		if len(statuses) > 0 {
			openStatusID = statuses[0].ID
		} else {
			return fmt.Errorf("could not find open status")
		}
	}

	input := &api.UpdateIssueInput{
		StatusID: &openStatusID,
	}

	if reopenComment != "" {
		input.Comment = &reopenComment
	}

	issue, err = client.UpdateIssue(ctx, issueKey, input)
	if err != nil {
		return fmt.Errorf("failed to reopen issue: %w\n\nThis may be caused by the project's workflow settings.\nBacklog restricts status transitions based on the configured workflow.\nCheck the project settings at: Settings > General > Workflow", err)
	}

	profile := cfg.CurrentProfile()
	switch profile.Output {
	case "json":
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(issue)
	default:
		ui.Success("Reopened %s", issue.IssueKey.Value)
		url := fmt.Sprintf("https://%s.%s/view/%s", profile.Space, profile.Domain, issue.IssueKey.Value)
		fmt.Printf("URL: %s\n", ui.Cyan(url))
		return nil
	}
}
