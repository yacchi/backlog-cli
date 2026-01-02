package issue

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/yacchi/backlog-cli/packages/backlog/internal/api"
	"github.com/yacchi/backlog-cli/packages/backlog/internal/cmdutil"
	"github.com/yacchi/backlog-cli/packages/backlog/internal/ui"
)

var commentCmd = &cobra.Command{
	Use:   "comment <issue-key>",
	Short: "Add or edit a comment on an issue",
	Long: `Add or edit a comment on an issue.

Without the body text supplied through flags, the command will interactively
prompt for the comment text.

Examples:
  # Add a comment using --body flag
  backlog issue comment PROJ-123 --body "This is fixed"
  backlog issue comment PROJ-123 -b "Quick fix applied"

  # Read comment from a file
  backlog issue comment PROJ-123 --body-file comment.txt

  # Read comment from standard input
  echo "Comment from stdin" | backlog issue comment PROJ-123 --body-file -

  # Open editor to write comment
  backlog issue comment PROJ-123 --editor

  # Edit an existing comment by ID
  backlog issue comment PROJ-123 --edit 12345 --body "Updated comment"
  backlog issue comment PROJ-123 --edit 12345 --editor

  # Edit your last comment on the issue
  backlog issue comment PROJ-123 --edit-last --body "Updated comment"
  backlog issue comment PROJ-123 --edit-last --editor`,
	Args: cobra.ExactArgs(1),
	RunE: runComment,
}

var (
	commentBody     string
	commentBodyFile string
	commentEditor   bool
	editCommentID   int
	editLast        bool
)

func init() {
	commentCmd.Flags().StringVarP(&commentBody, "body", "b", "", "The comment body text")
	commentCmd.Flags().StringVarP(&commentBodyFile, "body-file", "F", "", "Read body text from file (use \"-\" to read from standard input)")
	commentCmd.Flags().BoolVarP(&commentEditor, "editor", "e", false, "Open editor to write the comment")
	commentCmd.Flags().IntVar(&editCommentID, "edit", 0, "Edit an existing comment by ID")
	commentCmd.Flags().BoolVar(&editLast, "edit-last", false, "Edit your last comment on the issue")
	commentCmd.MarkFlagsMutuallyExclusive("edit", "edit-last")
}

func runComment(c *cobra.Command, args []string) error {
	issueKey := args[0]

	// 編集モードの判定
	isEditMode := editCommentID > 0 || editLast

	if isEditMode {
		return runEditComment(c, issueKey)
	}

	return runAddComment(c, issueKey)
}

// runAddComment は新しいコメントを追加する
func runAddComment(c *cobra.Command, issueKey string) error {
	client, cfg, err := cmdutil.GetAPIClient(c)
	if err != nil {
		return err
	}

	// メッセージ取得
	message, err := cmdutil.ResolveBody(
		commentBody,
		commentBodyFile,
		commentEditor,
		openEditor,
		func() (string, error) {
			return ui.Input("Comment:", "")
		},
	)
	if err != nil {
		return fmt.Errorf("failed to get comment: %w", err)
	}

	if message == "" {
		return fmt.Errorf("comment cannot be empty")
	}

	comment, err := client.AddComment(c.Context(), issueKey, message, nil)
	if err != nil {
		return fmt.Errorf("failed to add comment: %w", err)
	}

	ui.Success("Added comment #%d to %s", comment.ID, issueKey)

	profile := cfg.CurrentProfile()
	url := fmt.Sprintf("https://%s.%s/view/%s#comment-%d", profile.Space, profile.Domain, issueKey, comment.ID)
	fmt.Printf("URL: %s\n", ui.Cyan(url))

	return nil
}

// runEditComment は既存のコメントを編集する
func runEditComment(c *cobra.Command, issueKey string) error {
	client, cfg, err := cmdutil.GetAPIClient(c)
	if err != nil {
		return err
	}

	ctx := c.Context()
	var targetCommentID int

	if editLast {
		// 自分の最後のコメントを探す
		currentUser, err := client.GetCurrentUser(ctx)
		if err != nil {
			return fmt.Errorf("failed to get current user: %w", err)
		}

		comment, err := findMyLastComment(ctx, client, issueKey, currentUser.ID.Value)
		if err != nil {
			return err
		}
		targetCommentID = comment.ID
	} else {
		targetCommentID = editCommentID
	}

	// 編集対象のコメントを取得
	existingComment, err := client.GetComment(ctx, issueKey, targetCommentID)
	if err != nil {
		return fmt.Errorf("failed to get comment #%d: %w", targetCommentID, err)
	}

	// 新しいコンテンツを取得（エディタの場合は既存のコンテンツを初期値に）
	var message string
	if commentEditor {
		message, err = openEditor(existingComment.Content)
		if err != nil {
			return fmt.Errorf("failed to open editor: %w", err)
		}
	} else {
		message, err = cmdutil.ResolveBody(
			commentBody,
			commentBodyFile,
			false,
			nil,
			func() (string, error) {
				return ui.Input("New comment:", existingComment.Content)
			},
		)
		if err != nil {
			return fmt.Errorf("failed to get comment: %w", err)
		}
	}

	if message == "" {
		return fmt.Errorf("comment cannot be empty")
	}

	// 変更がない場合はスキップ
	if message == existingComment.Content {
		ui.Warning("No changes made to comment #%d", targetCommentID)
		return nil
	}

	// コメントを更新
	comment, err := client.UpdateComment(ctx, issueKey, targetCommentID, message)
	if err != nil {
		return fmt.Errorf("failed to update comment #%d: %w", targetCommentID, err)
	}

	ui.Success("Updated comment #%d on %s", comment.ID, issueKey)

	profile := cfg.CurrentProfile()
	url := fmt.Sprintf("https://%s.%s/view/%s#comment-%d", profile.Space, profile.Domain, issueKey, comment.ID)
	fmt.Printf("URL: %s\n", ui.Cyan(url))

	return nil
}

// findMyLastComment は指定されたユーザーの最後のコメントを探す
func findMyLastComment(ctx context.Context, client *api.Client, issueKey string, myUserID int) (*api.Comment, error) {
	comments, err := client.GetComments(ctx, issueKey, &api.CommentListOptions{
		Count: 100,
		Order: "desc",
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get comments: %w", err)
	}

	for i := range comments {
		if comments[i].CreatedUser.ID == myUserID {
			return &comments[i], nil
		}
	}

	return nil, fmt.Errorf("no comments found by you on issue %s", issueKey)
}
