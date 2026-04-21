package issue

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"

	"github.com/AlecAivazis/survey/v2"
	"github.com/spf13/cobra"
	"github.com/yacchi/backlog-cli/packages/backlog/internal/api"
	"github.com/yacchi/backlog-cli/packages/backlog/internal/cmdutil"
	"github.com/yacchi/backlog-cli/packages/backlog/internal/ui"
)

var attachmentCmd = &cobra.Command{
	Use:   "attachment",
	Short: "Manage issue attachments",
}

// --- list ---

var issueAttachmentListCmd = &cobra.Command{
	Use:   "list <issue-key>",
	Short: "List attachments of an issue",
	Long: `List all attachments for the specified issue.

Examples:
  backlog issue attachment list PROJ-123
  backlog issue attachment list PROJ-123 --json id,name,size`,
	Args: cobra.ExactArgs(1),
	RunE: runIssueAttachmentList,
}

func runIssueAttachmentList(c *cobra.Command, args []string) error {
	issueKey := args[0]

	client, cfg, err := cmdutil.GetAPIClient(c)
	if err != nil {
		return err
	}

	atts, err := client.ListIssueAttachments(c.Context(), issueKey)
	if err != nil {
		return fmt.Errorf("failed to list attachments: %w", err)
	}

	profile := cfg.CurrentProfile()
	if err := cmdutil.OutputJSONFromProfile(atts, profile.JSONFields, profile.JQ, profile.Template); err == nil {
		return nil
	}

	t := ui.NewTable()
	t.AddRow("ID", "NAME", "SIZE", "CREATED BY", "CREATED")
	for _, a := range atts {
		t.AddRow(
			strconv.Itoa(a.ID),
			a.Name,
			formatBytes(a.Size),
			a.CreatedUser.Name,
			shortDate(a.Created),
		)
	}
	t.RenderWithColor(os.Stdout, ui.IsColorEnabled())
	return nil
}

// --- download ---

var issueAttachmentDownloadCmd = &cobra.Command{
	Use:   "download <issue-key> <attachment-id>",
	Short: "Download an issue attachment",
	Long: `Download an attachment from an issue.

Examples:
  backlog issue attachment download PROJ-123 42
  backlog issue attachment download PROJ-123 42 -o report.pdf
  backlog issue attachment download PROJ-123 42 -o -`,
	Args: cobra.ExactArgs(2),
	RunE: runIssueAttachmentDownload,
}

var issueAttachmentDownloadOutput string

func init() {
	issueAttachmentDownloadCmd.Flags().StringVarP(&issueAttachmentDownloadOutput, "output", "o", "", "Output file path (use \"-\" for stdout)")
	attachmentCmd.AddCommand(issueAttachmentListCmd)
	attachmentCmd.AddCommand(issueAttachmentDownloadCmd)
	attachmentCmd.AddCommand(issueAttachmentDeleteCmd)
	attachmentCmd.AddCommand(issueAttachmentUploadCmd)
}

func runIssueAttachmentDownload(c *cobra.Command, args []string) error {
	issueKey := args[0]
	attachmentID, err := strconv.Atoi(args[1])
	if err != nil {
		return fmt.Errorf("invalid attachment ID: %s", args[1])
	}

	client, _, err := cmdutil.GetAPIClient(c)
	if err != nil {
		return err
	}

	fallback := fmt.Sprintf("attachment-%d", attachmentID)
	return cmdutil.RunAttachmentDownload(c.Context(), issueAttachmentDownloadOutput, fallback,
		func(ctx context.Context, w io.Writer) (string, int64, error) {
			return client.DownloadIssueAttachment(ctx, issueKey, attachmentID, w)
		})
}

// --- delete ---

var issueAttachmentDeleteCmd = &cobra.Command{
	Use:   "delete <issue-key> <attachment-id>",
	Short: "Delete an issue attachment",
	Long: `Delete an attachment from an issue.

Examples:
  backlog issue attachment delete PROJ-123 42
  backlog issue attachment delete PROJ-123 42 --yes`,
	Args: cobra.ExactArgs(2),
	RunE: runIssueAttachmentDelete,
}

var issueAttachmentDeleteYes bool

func init() {
	issueAttachmentDeleteCmd.Flags().BoolVar(&issueAttachmentDeleteYes, "yes", false, "Skip confirmation prompt")
}

func runIssueAttachmentDelete(c *cobra.Command, args []string) error {
	issueKey := args[0]
	attachmentID, err := strconv.Atoi(args[1])
	if err != nil {
		return fmt.Errorf("invalid attachment ID: %s", args[1])
	}

	client, _, err := cmdutil.GetAPIClient(c)
	if err != nil {
		return err
	}

	if !issueAttachmentDeleteYes {
		var confirm bool
		prompt := &survey.Confirm{
			Message: fmt.Sprintf("Delete attachment %d from %s?", attachmentID, issueKey),
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

	att, err := client.DeleteIssueAttachment(c.Context(), issueKey, attachmentID)
	if err != nil {
		return fmt.Errorf("failed to delete attachment: %w", err)
	}

	ui.Success("Deleted attachment: %s", att.Name)
	return nil
}

// --- upload ---

var issueAttachmentUploadCmd = &cobra.Command{
	Use:   "upload <issue-key> <file> [<file>...]",
	Short: "Upload and attach file(s) to an issue",
	Long: `Upload local file(s) and attach them to the issue.

Examples:
  backlog issue attachment upload PROJ-123 report.pdf
  backlog issue attachment upload PROJ-123 img1.png img2.png`,
	Args: cobra.MinimumNArgs(2),
	RunE: runIssueAttachmentUpload,
}

func runIssueAttachmentUpload(c *cobra.Command, args []string) error {
	issueKey := args[0]
	files := args[1:]

	client, _, err := cmdutil.GetAPIClient(c)
	if err != nil {
		return err
	}

	ctx := c.Context()
	var attachmentIDs []int
	for _, filePath := range files {
		f, err := os.Open(filePath)
		if err != nil {
			return fmt.Errorf("failed to open %s: %w", filePath, err)
		}
		up, err := client.UploadSpaceAttachment(ctx, filepath.Base(filePath), f)
		_ = f.Close()
		if err != nil {
			return fmt.Errorf("failed to upload %s: %w", filePath, err)
		}
		attachmentIDs = append(attachmentIDs, up.ID)
		fmt.Printf("Uploaded: %s (id: %d)\n", up.Name, up.ID)
	}

	_, err = client.UpdateIssue(ctx, issueKey, &api.UpdateIssueInput{
		AttachmentIDs: attachmentIDs,
	})
	if err != nil {
		return fmt.Errorf("failed to attach files to issue: %w", err)
	}

	ui.Success("Attached %d file(s) to %s", len(attachmentIDs), issueKey)
	return nil
}

// --- helpers ---

func formatBytes(size int64) string {
	const (
		KB = 1024
		MB = KB * 1024
		GB = MB * 1024
	)
	switch {
	case size >= GB:
		return fmt.Sprintf("%.1fGB", float64(size)/GB)
	case size >= MB:
		return fmt.Sprintf("%.1fMB", float64(size)/MB)
	case size >= KB:
		return fmt.Sprintf("%.1fKB", float64(size)/KB)
	default:
		return fmt.Sprintf("%dB", size)
	}
}

func shortDate(s string) string {
	if len(s) >= 10 {
		return s[:10]
	}
	return s
}
