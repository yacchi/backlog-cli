package space

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/yacchi/backlog-cli/packages/backlog/internal/cmdutil"
)

var spaceAttachmentCmd = &cobra.Command{
	Use:   "attachment",
	Short: "Manage space attachments",
}

var spaceAttachmentUploadCmd = &cobra.Command{
	Use:   "upload <file>",
	Short: "Upload a file to the space attachment store",
	Long: `Upload a local file to the Backlog space attachment store.
The returned attachment ID can be used when creating or updating issues and wiki pages.

Examples:
  backlog space attachment upload report.pdf
  backlog space attachment upload screenshot.png`,
	Args: cobra.ExactArgs(1),
	RunE: runSpaceAttachmentUpload,
}

func init() {
	spaceAttachmentCmd.AddCommand(spaceAttachmentUploadCmd)
	SpaceCmd.AddCommand(spaceAttachmentCmd)
}

func runSpaceAttachmentUpload(c *cobra.Command, args []string) error {
	filePath := args[0]

	client, _, err := cmdutil.GetAPIClient(c)
	if err != nil {
		return err
	}

	f, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("failed to open %s: %w", filePath, err)
	}
	defer func() { _ = f.Close() }()

	up, err := client.UploadSpaceAttachment(c.Context(), filepath.Base(filePath), f)
	if err != nil {
		return fmt.Errorf("failed to upload attachment: %w", err)
	}

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(up)
}
