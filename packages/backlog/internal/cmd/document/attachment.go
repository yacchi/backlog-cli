package document

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"github.com/spf13/cobra"
	"github.com/yacchi/backlog-cli/packages/backlog/internal/cmdutil"
	"github.com/yacchi/backlog-cli/packages/backlog/internal/ui"
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

	// 出力先を決定
	var outPath string
	useStdout := attachmentOutput == "-"

	var w *os.File
	if useStdout {
		w = os.Stdout
	} else {
		// ファイル名は DL 後に確定するので一時的に nil; 後で決定
		w = nil
	}

	if useStdout {
		_, _, err = client.DownloadDocumentAttachment(c.Context(), documentID, attachmentID, w)
		return err
	}

	// 一旦バッファなしで tmpFile に書き込み、ファイル名が確定したらリネーム
	tmpFile, err := os.CreateTemp("", "backlog-attachment-*")
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}
	tmpName := tmpFile.Name()
	defer func() {
		_ = tmpFile.Close()
		_ = os.Remove(tmpName)
	}()

	filename, _, err := client.DownloadDocumentAttachment(c.Context(), documentID, attachmentID, tmpFile)
	if err != nil {
		return fmt.Errorf("failed to download attachment: %w", err)
	}
	_ = tmpFile.Close()

	// 出力パス決定
	if attachmentOutput != "" {
		outPath = attachmentOutput
	} else if filename != "" {
		outPath = filename
	} else {
		outPath = fmt.Sprintf("attachment-%d", attachmentID)
	}

	if err := os.Rename(tmpName, outPath); err != nil {
		// Rename に失敗したら (cross-device) コピー
		if copyErr := copyFile(tmpName, outPath); copyErr != nil {
			return fmt.Errorf("failed to save attachment: %w", copyErr)
		}
	}

	abs, _ := filepath.Abs(outPath)
	ui.Success("Downloaded: %s", abs)
	return nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer func() { _ = out.Close() }()

	buf := make([]byte, 32*1024)
	for {
		n, readErr := in.Read(buf)
		if n > 0 {
			if _, writeErr := out.Write(buf[:n]); writeErr != nil {
				return writeErr
			}
		}
		if readErr != nil {
			if readErr.Error() == "EOF" {
				break
			}
			return readErr
		}
	}
	return nil
}
