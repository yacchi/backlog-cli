package pr

import (
	"context"
	"fmt"
	"io"
	"os"
	"strconv"

	"github.com/AlecAivazis/survey/v2"
	"github.com/spf13/cobra"
	"github.com/yacchi/backlog-cli/packages/backlog/internal/cmdutil"
	"github.com/yacchi/backlog-cli/packages/backlog/internal/ui"
)

var prAttachmentCmd = &cobra.Command{
	Use:   "attachment",
	Short: "Manage pull request attachments",
}

// --- list ---

var prAttachmentListCmd = &cobra.Command{
	Use:   "list <pr-number>",
	Short: "List attachments of a pull request",
	Long: `List all attachments for the specified pull request.

Examples:
  backlog pr attachment list 3 --repo myrepo
  backlog pr attachment list 3 -R myrepo --json id,name,size`,
	Args: cobra.ExactArgs(1),
	RunE: runPRAttachmentList,
}

var prAttachmentListRepo string

func init() {
	prAttachmentListCmd.Flags().StringVarP(&prAttachmentListRepo, "repo", "R", "", "Repository name (required)")
	_ = prAttachmentListCmd.MarkFlagRequired("repo")
}

func runPRAttachmentList(c *cobra.Command, args []string) error {
	number, err := strconv.Atoi(args[0])
	if err != nil {
		return fmt.Errorf("invalid pull request number: %s", args[0])
	}

	client, cfg, err := cmdutil.GetAPIClient(c)
	if err != nil {
		return err
	}
	if err := cmdutil.RequireProject(cfg); err != nil {
		return err
	}
	projectKey := cmdutil.GetCurrentProject(cfg)

	atts, err := client.ListPRAttachments(c.Context(), projectKey, prAttachmentListRepo, number)
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

var prAttachmentDownloadCmd = &cobra.Command{
	Use:   "download <pr-number> <attachment-id>",
	Short: "Download a pull request attachment",
	Long: `Download an attachment from a pull request.

Examples:
  backlog pr attachment download 3 42 --repo myrepo
  backlog pr attachment download 3 42 -R myrepo -o patch.diff
  backlog pr attachment download 3 42 -R myrepo -o -`,
	Args: cobra.ExactArgs(2),
	RunE: runPRAttachmentDownload,
}

var (
	prAttachmentDownloadRepo   string
	prAttachmentDownloadOutput string
)

func init() {
	prAttachmentDownloadCmd.Flags().StringVarP(&prAttachmentDownloadRepo, "repo", "R", "", "Repository name (required)")
	prAttachmentDownloadCmd.Flags().StringVarP(&prAttachmentDownloadOutput, "output", "o", "", "Output file path (use \"-\" for stdout)")
	_ = prAttachmentDownloadCmd.MarkFlagRequired("repo")
}

func runPRAttachmentDownload(c *cobra.Command, args []string) error {
	number, err := strconv.Atoi(args[0])
	if err != nil {
		return fmt.Errorf("invalid pull request number: %s", args[0])
	}
	attachmentID, err := strconv.Atoi(args[1])
	if err != nil {
		return fmt.Errorf("invalid attachment ID: %s", args[1])
	}

	client, cfg, err := cmdutil.GetAPIClient(c)
	if err != nil {
		return err
	}
	if err := cmdutil.RequireProject(cfg); err != nil {
		return err
	}
	projectKey := cmdutil.GetCurrentProject(cfg)

	fallback := fmt.Sprintf("attachment-%d", attachmentID)
	return cmdutil.RunAttachmentDownload(c.Context(), prAttachmentDownloadOutput, fallback,
		func(ctx context.Context, w io.Writer) (string, int64, error) {
			return client.DownloadPRAttachment(ctx, projectKey, prAttachmentDownloadRepo, number, attachmentID, w)
		})
}

// --- delete ---

var prAttachmentDeleteCmd = &cobra.Command{
	Use:   "delete <pr-number> <attachment-id>",
	Short: "Delete a pull request attachment",
	Long: `Delete an attachment from a pull request.

Examples:
  backlog pr attachment delete 3 42 --repo myrepo
  backlog pr attachment delete 3 42 -R myrepo --yes`,
	Args: cobra.ExactArgs(2),
	RunE: runPRAttachmentDelete,
}

var (
	prAttachmentDeleteRepo string
	prAttachmentDeleteYes  bool
)

func init() {
	prAttachmentDeleteCmd.Flags().StringVarP(&prAttachmentDeleteRepo, "repo", "R", "", "Repository name (required)")
	prAttachmentDeleteCmd.Flags().BoolVar(&prAttachmentDeleteYes, "yes", false, "Skip confirmation prompt")
	_ = prAttachmentDeleteCmd.MarkFlagRequired("repo")
	prAttachmentCmd.AddCommand(prAttachmentListCmd)
	prAttachmentCmd.AddCommand(prAttachmentDownloadCmd)
	prAttachmentCmd.AddCommand(prAttachmentDeleteCmd)
}

func runPRAttachmentDelete(c *cobra.Command, args []string) error {
	number, err := strconv.Atoi(args[0])
	if err != nil {
		return fmt.Errorf("invalid pull request number: %s", args[0])
	}
	attachmentID, err := strconv.Atoi(args[1])
	if err != nil {
		return fmt.Errorf("invalid attachment ID: %s", args[1])
	}

	client, cfg, err := cmdutil.GetAPIClient(c)
	if err != nil {
		return err
	}
	if err := cmdutil.RequireProject(cfg); err != nil {
		return err
	}
	projectKey := cmdutil.GetCurrentProject(cfg)

	if !prAttachmentDeleteYes {
		var confirm bool
		prompt := &survey.Confirm{
			Message: fmt.Sprintf("Delete attachment %d from PR #%d?", attachmentID, number),
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

	att, err := client.DeletePRAttachment(c.Context(), projectKey, prAttachmentDeleteRepo, number, attachmentID)
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
