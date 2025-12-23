package issue

import (
	"fmt"
	"strconv"

	"github.com/spf13/cobra"
	"github.com/yacchi/backlog-cli/internal/api"
	"github.com/yacchi/backlog-cli/internal/cmdutil"
	"github.com/yacchi/backlog-cli/internal/ui"
)

var closeCmd = &cobra.Command{
	Use:   "close <issue-key>",
	Short: "Close an issue",
	Long: `Close an issue by changing its status to "Closed".

Examples:
  backlog issue close PROJ-123
  backlog issue close PROJ-123 --resolution 0
  backlog issue close PROJ-123 --comment "Fixed in v1.2"`,
	Args: cobra.ExactArgs(1),
	RunE: runClose,
}

var (
	closeResolutionID int
	closeComment      string
)

func init() {
	closeCmd.Flags().IntVar(&closeResolutionID, "resolution", 0, "Resolution ID")
	closeCmd.Flags().StringVarP(&closeComment, "comment", "c", "", "Comment to add")
}

func runClose(c *cobra.Command, args []string) error {
	issueKey := args[0]

	client, cfg, err := cmdutil.GetAPIClient(c)
	if err != nil {
		return err
	}

	// 現在の課題を取得
	ctx := c.Context()
	issue, err := client.GetIssue(ctx, issueKey)
	if err != nil {
		return fmt.Errorf("failed to get issue: %w", err)
	}

	// プロジェクトのステータスを取得してCloseステータスを探す
	statuses, err := client.GetStatuses(ctx, strconv.Itoa(issue.ProjectId.Value))
	if err != nil {
		return fmt.Errorf("failed to get statuses: %w", err)
	}

	var closedStatusID int
	for _, s := range statuses {
		// "完了" または "Closed" を探す
		if s.Name == "完了" || s.Name == "Closed" || s.Name == "Done" {
			closedStatusID = s.ID
			break
		}
	}

	if closedStatusID == 0 {
		// 見つからない場合は最後のステータスを使用
		if len(statuses) > 0 {
			closedStatusID = statuses[len(statuses)-1].ID
		} else {
			return fmt.Errorf("could not find closed status")
		}
	}

	input := &api.UpdateIssueInput{
		StatusID: &closedStatusID,
	}

	if closeResolutionID > 0 {
		input.ResolutionID = &closeResolutionID
	}
	if closeComment != "" {
		input.Comment = &closeComment
	}

	issue, err = client.UpdateIssue(ctx, issueKey, input)
	if err != nil {
		return fmt.Errorf("failed to close issue: %w", err)
	}

	ui.Success("Closed %s", issue.IssueKey.Value)

	profile := cfg.CurrentProfile()
	url := fmt.Sprintf("https://%s.%s/view/%s", profile.Space, profile.Domain, issue.IssueKey.Value)
	fmt.Printf("URL: %s\n", ui.Cyan(url))

	return nil
}
