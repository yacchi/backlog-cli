package wiki

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"

	"github.com/AlecAivazis/survey/v2"
	"github.com/spf13/cobra"
	"github.com/yacchi/backlog-cli/packages/backlog/internal/cmdutil"
	"github.com/yacchi/backlog-cli/packages/backlog/internal/ui"
)

var wikiAttachmentCmd = &cobra.Command{
	Use:   "attachment",
	Short: "Manage wiki attachments",
}

// --- list ---

var wikiAttachmentListCmd = &cobra.Command{
	Use:   "list <wiki-id>",
	Short: "List attachments of a wiki page",
	Long: `List all attachments for the specified wiki page.

Examples:
  backlog wiki attachment list 100
  backlog wiki attachment list 100 --json id,name,size`,
	Args: cobra.ExactArgs(1),
	RunE: runWikiAttachmentList,
}

func runWikiAttachmentList(c *cobra.Command, args []string) error {
	wikiID, err := strconv.Atoi(args[0])
	if err != nil {
		return fmt.Errorf("invalid wiki ID: %s", args[0])
	}

	client, cfg, err := cmdutil.GetAPIClient(c)
	if err != nil {
		return err
	}

	atts, err := client.ListWikiAttachments(c.Context(), wikiID)
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

// --- upload ---

var wikiAttachmentUploadCmd = &cobra.Command{
	Use:   "upload <wiki-id> <file> [<file>...]",
	Short: "Upload and attach file(s) to a wiki page",
	Long: `Upload local file(s) and attach them to the wiki page.

Examples:
  backlog wiki attachment upload 100 report.pdf
  backlog wiki attachment upload 100 img1.png img2.png`,
	Args: cobra.MinimumNArgs(2),
	RunE: runWikiAttachmentUpload,
}

func runWikiAttachmentUpload(c *cobra.Command, args []string) error {
	wikiID, err := strconv.Atoi(args[0])
	if err != nil {
		return fmt.Errorf("invalid wiki ID: %s", args[0])
	}
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

	atts, err := client.AttachFilesToWiki(ctx, wikiID, attachmentIDs)
	if err != nil {
		return fmt.Errorf("failed to attach files to wiki: %w", err)
	}

	ui.Success("Attached %d file(s) to wiki %d", len(atts), wikiID)
	return nil
}

// --- download ---

var wikiAttachmentDownloadCmd = &cobra.Command{
	Use:   "download <wiki-id> <attachment-id>",
	Short: "Download a wiki attachment",
	Long: `Download an attachment from a wiki page.

Examples:
  backlog wiki attachment download 100 42
  backlog wiki attachment download 100 42 -o report.pdf
  backlog wiki attachment download 100 42 -o -`,
	Args: cobra.ExactArgs(2),
	RunE: runWikiAttachmentDownload,
}

var wikiAttachmentDownloadOutput string

func init() {
	wikiAttachmentDownloadCmd.Flags().StringVarP(&wikiAttachmentDownloadOutput, "output", "o", "", "Output file path (use \"-\" for stdout)")
	wikiAttachmentCmd.AddCommand(wikiAttachmentListCmd)
	wikiAttachmentCmd.AddCommand(wikiAttachmentUploadCmd)
	wikiAttachmentCmd.AddCommand(wikiAttachmentDownloadCmd)
	wikiAttachmentCmd.AddCommand(wikiAttachmentDeleteCmd)
}

func runWikiAttachmentDownload(c *cobra.Command, args []string) error {
	wikiID, err := strconv.Atoi(args[0])
	if err != nil {
		return fmt.Errorf("invalid wiki ID: %s", args[0])
	}
	attachmentID, err := strconv.Atoi(args[1])
	if err != nil {
		return fmt.Errorf("invalid attachment ID: %s", args[1])
	}

	client, _, err := cmdutil.GetAPIClient(c)
	if err != nil {
		return err
	}

	fallback := fmt.Sprintf("attachment-%d", attachmentID)
	return cmdutil.RunAttachmentDownload(c.Context(), wikiAttachmentDownloadOutput, fallback,
		func(ctx context.Context, w io.Writer) (string, int64, error) {
			return client.DownloadWikiAttachment(ctx, wikiID, attachmentID, w)
		})
}

// --- delete ---

var wikiAttachmentDeleteCmd = &cobra.Command{
	Use:   "delete <wiki-id> <attachment-id>",
	Short: "Delete a wiki attachment",
	Long: `Delete an attachment from a wiki page.

Examples:
  backlog wiki attachment delete 100 42
  backlog wiki attachment delete 100 42 --yes`,
	Args: cobra.ExactArgs(2),
	RunE: runWikiAttachmentDelete,
}

var wikiAttachmentDeleteYes bool

func init() {
	wikiAttachmentDeleteCmd.Flags().BoolVar(&wikiAttachmentDeleteYes, "yes", false, "Skip confirmation prompt")
}

func runWikiAttachmentDelete(c *cobra.Command, args []string) error {
	wikiID, err := strconv.Atoi(args[0])
	if err != nil {
		return fmt.Errorf("invalid wiki ID: %s", args[0])
	}
	attachmentID, err := strconv.Atoi(args[1])
	if err != nil {
		return fmt.Errorf("invalid attachment ID: %s", args[1])
	}

	client, _, err := cmdutil.GetAPIClient(c)
	if err != nil {
		return err
	}

	if !wikiAttachmentDeleteYes {
		var confirm bool
		prompt := &survey.Confirm{
			Message: fmt.Sprintf("Delete attachment %d from wiki %d?", attachmentID, wikiID),
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

	att, err := client.DeleteWikiAttachment(c.Context(), wikiID, attachmentID)
	if err != nil {
		return fmt.Errorf("failed to delete attachment: %w", err)
	}

	ui.Success("Deleted attachment: %s", att.Name)
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
