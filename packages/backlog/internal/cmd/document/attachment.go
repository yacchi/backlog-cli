package document

import (
	"context"
	"fmt"
	"io"
	"strconv"

	"github.com/spf13/cobra"
	"github.com/yacchi/backlog-cli/packages/backlog/internal/cmdutil"
)

var attachmentCmd = &cobra.Command{
	Use:   "attachment",
	Short: "Manage document attachments",
}

var attachmentDownloadCmd = &cobra.Command{
	Use:   "download <document-id> <attachment-id>",
	Short: "Download a document attachment",
	Long: `Download an attachment from a document.

Examples:
  backlog document attachment download 01HXXXXXXXX 123
  backlog document attachment download 01HXXXXXXXX 123 -o report.pdf
  backlog document attachment download 01HXXXXXXXX 123 -o -`,
	Args: cobra.ExactArgs(2),
	RunE: runAttachmentDownload,
}

var attachmentOutput string

func init() {
	attachmentDownloadCmd.Flags().StringVarP(&attachmentOutput, "output", "o", "", "Output file path (use \"-\" for stdout)")
	attachmentCmd.AddCommand(attachmentDownloadCmd)
}

func runAttachmentDownload(c *cobra.Command, args []string) error {
	documentID := args[0]
	attachmentID, err := strconv.Atoi(args[1])
	if err != nil {
		return fmt.Errorf("invalid attachment ID: %s", args[1])
	}

	client, _, err := cmdutil.GetAPIClient(c)
	if err != nil {
		return err
	}

	fallback := fmt.Sprintf("attachment-%d", attachmentID)
	return cmdutil.RunAttachmentDownload(c.Context(), attachmentOutput, fallback,
		func(ctx context.Context, w io.Writer) (string, int64, error) {
			return client.DownloadDocumentAttachment(ctx, documentID, attachmentID, w)
		})
}
