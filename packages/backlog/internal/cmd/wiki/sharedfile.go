package wiki

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

var wikiSharedFileCmd = &cobra.Command{
	Use:   "sharedfile",
	Short: "Manage shared file links on a wiki page",
}

// --- list ---

var wikiSharedFileListCmd = &cobra.Command{
	Use:   "list <wiki-id>",
	Short: "List shared files linked to a wiki page",
	Long: `List all shared files linked to the specified wiki page.

Examples:
  backlog wiki sharedfile list 100`,
	Args: cobra.ExactArgs(1),
	RunE: runWikiSharedFileList,
}

func runWikiSharedFileList(c *cobra.Command, args []string) error {
	wikiID, err := strconv.Atoi(args[0])
	if err != nil {
		return fmt.Errorf("invalid wiki ID: %s", args[0])
	}

	client, cfg, err := cmdutil.GetAPIClient(c)
	if err != nil {
		return err
	}

	files, err := client.ListWikiSharedFiles(c.Context(), wikiID)
	if err != nil {
		return fmt.Errorf("failed to list shared files: %w", err)
	}

	profile := cfg.CurrentProfile()
	if err := cmdutil.OutputJSONFromProfile(files, profile.JSONFields, profile.JQ); err == nil {
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

var wikiSharedFileLinkCmd = &cobra.Command{
	Use:   "link <wiki-id> <shared-file-id> [<shared-file-id>...]",
	Short: "Link shared file(s) to a wiki page",
	Long: `Link one or more project shared files to a wiki page.

Examples:
  backlog wiki sharedfile link 100 456
  backlog wiki sharedfile link 100 456 789`,
	Args: cobra.MinimumNArgs(2),
	RunE: runWikiSharedFileLink,
}

func runWikiSharedFileLink(c *cobra.Command, args []string) error {
	wikiID, err := strconv.Atoi(args[0])
	if err != nil {
		return fmt.Errorf("invalid wiki ID: %s", args[0])
	}
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

	files, err := client.LinkWikiSharedFiles(c.Context(), wikiID, fileIDs)
	if err != nil {
		return fmt.Errorf("failed to link shared files: %w", err)
	}

	ui.Success("Linked %d shared file(s) to wiki %d", len(files), wikiID)
	return nil
}

// --- unlink ---

var wikiSharedFileUnlinkCmd = &cobra.Command{
	Use:   "unlink <wiki-id> <shared-file-id>",
	Short: "Unlink a shared file from a wiki page",
	Long: `Remove a shared file link from a wiki page.

Examples:
  backlog wiki sharedfile unlink 100 456
  backlog wiki sharedfile unlink 100 456 --yes`,
	Args: cobra.ExactArgs(2),
	RunE: runWikiSharedFileUnlink,
}

var wikiSharedFileUnlinkYes bool

func init() {
	wikiSharedFileUnlinkCmd.Flags().BoolVar(&wikiSharedFileUnlinkYes, "yes", false, "Skip confirmation prompt")
	wikiSharedFileCmd.AddCommand(wikiSharedFileListCmd)
	wikiSharedFileCmd.AddCommand(wikiSharedFileLinkCmd)
	wikiSharedFileCmd.AddCommand(wikiSharedFileUnlinkCmd)
}

func runWikiSharedFileUnlink(c *cobra.Command, args []string) error {
	wikiID, err := strconv.Atoi(args[0])
	if err != nil {
		return fmt.Errorf("invalid wiki ID: %s", args[0])
	}
	fileID, err := strconv.Atoi(args[1])
	if err != nil {
		return fmt.Errorf("invalid shared file ID: %s", args[1])
	}

	client, _, err := cmdutil.GetAPIClient(c)
	if err != nil {
		return err
	}

	if !wikiSharedFileUnlinkYes {
		var confirm bool
		prompt := &survey.Confirm{
			Message: fmt.Sprintf("Unlink shared file %d from wiki %d?", fileID, wikiID),
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

	f, err := client.UnlinkWikiSharedFile(c.Context(), wikiID, fileID)
	if err != nil {
		return fmt.Errorf("failed to unlink shared file: %w", err)
	}

	ui.Success("Unlinked shared file: %s", f.Name)
	return nil
}
