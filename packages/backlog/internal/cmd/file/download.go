package file

import (
	"context"
	"fmt"
	"io"
	"strconv"

	"github.com/spf13/cobra"
	"github.com/yacchi/backlog-cli/packages/backlog/internal/cmdutil"
)

var downloadCmd = &cobra.Command{
	Use:   "download <shared-file-id>",
	Short: "Download a project shared file",
	Long: `Download a shared file from the project by its file ID.
Use "backlog file list" to find the file ID.

Examples:
  backlog file download 999 -p MYPROJ
  backlog file download 999 -p MYPROJ -o /tmp/spec.pdf
  backlog file download 999 -p MYPROJ -o -`,
	Args: cobra.ExactArgs(1),
	RunE: runDownload,
}

var (
	downloadProject string
	downloadOutput  string
)

func init() {
	downloadCmd.Flags().StringVarP(&downloadProject, "project", "p", "", "Project key (required if not in project context)")
	downloadCmd.Flags().StringVarP(&downloadOutput, "output", "o", "", "Output file path (use \"-\" for stdout)")
}

func runDownload(c *cobra.Command, args []string) error {
	sharedFileID, err := strconv.Atoi(args[0])
	if err != nil {
		return fmt.Errorf("invalid shared file ID: %s", args[0])
	}

	client, cfg, err := cmdutil.GetAPIClient(c)
	if err != nil {
		return err
	}

	projectKey := downloadProject
	if projectKey == "" {
		if err := cmdutil.RequireProject(cfg); err != nil {
			return fmt.Errorf("project key required: use -p flag or run inside a project context")
		}
		projectKey = cmdutil.GetCurrentProject(cfg)
	}

	fallback := fmt.Sprintf("file-%d", sharedFileID)
	return cmdutil.RunAttachmentDownload(c.Context(), downloadOutput, fallback,
		func(ctx context.Context, w io.Writer) (string, int64, error) {
			return client.DownloadProjectFile(ctx, projectKey, sharedFileID, w)
		})
}
