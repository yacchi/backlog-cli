package star

import (
	"fmt"
	"strconv"

	"github.com/spf13/cobra"
	"github.com/yacchi/backlog-cli/packages/backlog/internal/api"
	"github.com/yacchi/backlog-cli/packages/backlog/internal/cmdutil"
	"github.com/yacchi/backlog-cli/packages/backlog/internal/ui"
)

var addCmd = &cobra.Command{
	Use:   "add [<issue-key-or-id>]",
	Short: "Star an issue, comment, wiki page, or pull request",
	Long: `Add a star to exactly one item: an issue, a comment, a wiki page, or a
pull request.

The item is identified in one of two mutually exclusive ways:
  - the positional argument, for an issue: an issue key (e.g. PROJ-123) or
    a bare number. A bare number is treated as Backlog's own internal issue
    ID (not the issue key's numeric suffix); an issue key is resolved
    through the API to look up that internal ID.
  - one of --comment, --wiki, or --pull-request, each taking the target's
    own numeric ID, for the other three item types.

Specifying the positional argument together with any of --comment/--wiki/
--pull-request, or specifying more than one of those flags, is a usage
error and no request is sent to the API.`,
	Example: `  # Star an issue by key
  backlog star add PROJ-123

  # Star an issue by its internal numeric ID
  backlog star add 40001

  # Star a comment by its numeric comment ID, and confirm as JSON
  backlog star add --comment 5001 --output json`,
	Args: cobra.MaximumNArgs(1),
	RunE: runAdd,
}

var (
	addComment     int
	addWiki        int
	addPullRequest int
)

func init() {
	addCmd.Flags().IntVar(&addComment, "comment", 0, "Star a comment by its numeric comment ID (mutually exclusive with the issue argument, --wiki, and --pull-request)")
	addCmd.Flags().IntVar(&addWiki, "wiki", 0, "Star a wiki page by its numeric wiki ID (mutually exclusive with the issue argument, --comment, and --pull-request)")
	addCmd.Flags().IntVar(&addPullRequest, "pull-request", 0, "Star a pull request by its numeric pull request ID (mutually exclusive with the issue argument, --comment, and --wiki)")
}

func runAdd(c *cobra.Command, args []string) error {
	kindsGiven := 0
	if len(args) == 1 {
		kindsGiven++
	}
	if addComment != 0 {
		kindsGiven++
	}
	if addWiki != 0 {
		kindsGiven++
	}
	if addPullRequest != 0 {
		kindsGiven++
	}
	if kindsGiven == 0 {
		return fmt.Errorf("specify a target: an issue key/ID argument, or one of --comment, --wiki, --pull-request")
	}
	if kindsGiven > 1 {
		return fmt.Errorf("specify exactly one star target: an issue key/ID argument, or one of --comment, --wiki, --pull-request (not more than one)")
	}

	client, cfg, err := cmdutil.GetAPIClient(c)
	if err != nil {
		return err
	}
	ctx := c.Context()

	input := &api.AddStarInput{
		CommentID:     addComment,
		WikiID:        addWiki,
		PullRequestID: addPullRequest,
	}

	var targetDesc string
	switch {
	case len(args) == 1:
		target := args[0]
		if id, err := strconv.Atoi(target); err == nil {
			// 数値のみの場合は Backlog 内部の課題IDとして扱う
			input.IssueID = id
			targetDesc = fmt.Sprintf("issue #%d", id)
		} else {
			resolvedKey, _ := cmdutil.ResolveIssueKey(target, cmdutil.GetCurrentProject(cfg))
			issue, err := client.GetIssue(ctx, resolvedKey)
			if err != nil {
				return fmt.Errorf("failed to resolve issue: %w", err)
			}
			input.IssueID = issue.ID.Value
			targetDesc = fmt.Sprintf("issue %s", issue.IssueKey.Value)
		}
	case addComment != 0:
		targetDesc = fmt.Sprintf("comment #%d", addComment)
	case addWiki != 0:
		targetDesc = fmt.Sprintf("wiki page #%d", addWiki)
	default:
		targetDesc = fmt.Sprintf("pull request #%d", addPullRequest)
	}

	if err := client.AddStar(ctx, input); err != nil {
		return fmt.Errorf("failed to add star: %w", err)
	}

	profile := cfg.CurrentProfile()
	switch profile.Output {
	case "json":
		return cmdutil.OutputJSONFromProfile(map[string]string{
			"status": "starred",
			"target": targetDesc,
		}, profile.JSONFields, profile.JQ, profile.Template)
	default:
		ui.Success("Starred %s", targetDesc)
		return nil
	}
}
