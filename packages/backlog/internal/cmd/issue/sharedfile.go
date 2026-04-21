package issue

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/AlecAivazis/survey/v2"
	"github.com/spf13/cobra"
	"github.com/yacchi/backlog-cli/packages/backlog/internal/cmdutil"
	"github.com/yacchi/backlog-cli/packages/backlog/internal/ui"
)

var sharedFileCmd = &cobra.Command{
	Use:   "sharedfile",
	Short: "Manage shared file links on an issue",
}

// --- list ---

var issueSharedFileListCmd = &cobra.Command{
	Use:   "list <issue-key>",
	Short: "List shared files linked to an issue",
	Long: `List all shared files linked to the specified issue.

Examples:
  backlog issue sharedfile list PROJ-123`,
	Args: cobra.ExactArgs(1),
	RunE: runIssueSharedFileList,
}

func runIssueSharedFileList(c *cobra.Command, args []string) error {
	issueKey := args[0]

	client, cfg, err := cmdutil.GetAPIClient(c)
	if err != nil {
		return err
	}

	files, err := client.ListIssueSharedFiles(c.Context(), issueKey)
	if err != nil {
		return fmt.Errorf("failed to list shared files: %w", err)
	}

	profile := cfg.CurrentProfile()
	if err := cmdutil.OutputJSONFromProfile(files, profile.JSONFields, profile.JQ, profile.Template); err == nil {
		return nil
	}

	t := ui.NewTable()
	t.AddRow("ID", "TYPE", "DIR", "NAME", "SIZE")
	for _, f := range files {
		t.AddRow(
			strconv.Itoa(f.ID),
			f.Type,
			f.Dir,
			f.Name,
			formatBytes(f.Size),
		)
	}
	t.RenderWithColor(os.Stdout, ui.IsColorEnabled())
	return nil
}

// --- link ---

var issueSharedFileLinkCmd = &cobra.Command{
	Use:   "link <issue-key> <shared-file-id> [<shared-file-id>...]",
	Short: "Link shared file(s) to an issue",
	Long: `Link one or more project shared files to an issue.

Examples:
  backlog issue sharedfile link PROJ-123 456
  backlog issue sharedfile link PROJ-123 456 789`,
	Args: cobra.MinimumNArgs(2),
	RunE: runIssueSharedFileLink,
}

func runIssueSharedFileLink(c *cobra.Command, args []string) error {
	issueKey := args[0]
	rawIDs := args[1:]

	var fileIDs []int
	for _, s := range rawIDs {
		id, err := strconv.Atoi(strings.TrimSpace(s))
		if err != nil {
			return fmt.Errorf("invalid shared file ID: %s", s)
		}
		fileIDs = append(fileIDs, id)
	}

	client, _, err := cmdutil.GetAPIClient(c)
	if err != nil {
		return err
	}

	files, err := client.LinkIssueSharedFiles(c.Context(), issueKey, fileIDs)
	if err != nil {
		return fmt.Errorf("failed to link shared files: %w", err)
	}

	ui.Success("Linked %d shared file(s) to %s", len(files), issueKey)
	return nil
}

// --- unlink ---

var issueSharedFileUnlinkCmd = &cobra.Command{
	Use:   "unlink <issue-key> <shared-file-id>",
	Short: "Unlink a shared file from an issue",
	Long: `Remove a shared file link from an issue.

Examples:
  backlog issue sharedfile unlink PROJ-123 456
  backlog issue sharedfile unlink PROJ-123 456 --yes`,
	Args: cobra.ExactArgs(2),
	RunE: runIssueSharedFileUnlink,
}

var issueSharedFileUnlinkYes bool

func init() {
	issueSharedFileUnlinkCmd.Flags().BoolVar(&issueSharedFileUnlinkYes, "yes", false, "Skip confirmation prompt")
	sharedFileCmd.AddCommand(issueSharedFileListCmd)
	sharedFileCmd.AddCommand(issueSharedFileLinkCmd)
	sharedFileCmd.AddCommand(issueSharedFileUnlinkCmd)
}

func runIssueSharedFileUnlink(c *cobra.Command, args []string) error {
	issueKey := args[0]
	fileID, err := strconv.Atoi(args[1])
	if err != nil {
		return fmt.Errorf("invalid shared file ID: %s", args[1])
	}

	client, _, err := cmdutil.GetAPIClient(c)
	if err != nil {
		return err
	}

	if !issueSharedFileUnlinkYes {
		var confirm bool
		prompt := &survey.Confirm{
			Message: fmt.Sprintf("Unlink shared file %d from %s?", fileID, issueKey),
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

	f, err := client.UnlinkIssueSharedFile(c.Context(), issueKey, fileID)
	if err != nil {
		return fmt.Errorf("failed to unlink shared file: %w", err)
	}

	ui.Success("Unlinked shared file: %s", f.Name)
	return nil
}
