package wiki

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strconv"

	"github.com/spf13/cobra"
	"github.com/yacchi/backlog-cli/packages/backlog/internal/api"
	"github.com/yacchi/backlog-cli/packages/backlog/internal/cmdutil"
	"github.com/yacchi/backlog-cli/packages/backlog/internal/config"
	"github.com/yacchi/backlog-cli/packages/backlog/internal/ui"
)

var editCmd = &cobra.Command{
	Use:   "edit <id-or-name>",
	Short: "Edit a wiki page",
	Long: `Edit an existing wiki page.

Modes:
  Direct replacement (default):
    backlog wiki edit 123 --content "New full content"
    backlog wiki edit 123 --content-file updated.md

  Safe replacement (conflict detection with three-way merge):
    backlog wiki edit 123 --content "New content" --safe

  Search-and-replace patch (JSON):
    backlog wiki edit 123 --patch '{"find":"old","replace":"new"}'
    backlog wiki edit 123 --patch '[{"find":"A","replace":"A2"},{"find":"B","replace":"B2"}]'
    echo '[...]' | backlog wiki edit 123 --patch-file -

  Append/Prepend:
    backlog wiki edit 123 --append "Text to add at end"
    backlog wiki edit 123 --prepend "Text to add at start"

Patch modes (--patch, --append, --prepend, --safe) use Read-Modify-Write
with conflict detection. If another user modified the page concurrently,
a three-way merge is attempted automatically.`,
	Args: cobra.ExactArgs(1),
	RunE: runEdit,
}

var (
	editName        string
	editContent     string
	editContentFile string
	editMailNotify  bool
	editSafe        bool
	editPatch       string
	editPatchFile   string
	editAppend      string
	editPrepend     string
)

func init() {
	editCmd.Flags().StringVarP(&editName, "name", "n", "", "New wiki page name")
	editCmd.Flags().StringVarP(&editContent, "content", "c", "", "New wiki page content")
	editCmd.Flags().StringVarP(&editContentFile, "content-file", "F", "", "Read content from file (use \"-\" to read from standard input)")
	editCmd.Flags().BoolVar(&editMailNotify, "notify", false, "Send mail notification")
	editCmd.Flags().BoolVar(&editSafe, "safe", false, "Use conflict detection with three-way merge for content replacement")
	editCmd.Flags().StringVar(&editPatch, "patch", "", "Search-and-replace as JSON: {\"find\":\"...\",\"replace\":\"...\"} or array")
	editCmd.Flags().StringVar(&editPatchFile, "patch-file", "", "Read patch JSON from file (use \"-\" for stdin)")
	editCmd.Flags().StringVar(&editAppend, "append", "", "Text to append to current content")
	editCmd.Flags().StringVar(&editPrepend, "prepend", "", "Text to prepend to current content")
}

func runEdit(c *cobra.Command, args []string) error {
	client, cfg, err := cmdutil.GetAPIClient(c)
	if err != nil {
		return err
	}

	profile := cfg.CurrentProfile()
	idOrName := args[0]
	ctx := c.Context()

	wikiID, err := resolveWikiID(client, ctx, cfg, idOrName)
	if err != nil {
		return err
	}

	hasPatchFlags := editPatch != "" || editPatchFile != "" || editAppend != "" || editPrepend != ""
	hasContentFlags := c.Flags().Changed("content") || editContentFile != ""

	if !hasPatchFlags && !hasContentFlags && !c.Flags().Changed("name") && !editSafe {
		return fmt.Errorf("no changes specified. Use --content, --patch, --append, or --prepend")
	}

	if hasPatchFlags && hasContentFlags && !editSafe {
		return fmt.Errorf("--patch/--append/--prepend cannot be combined with --content/--content-file (use --safe for safe full replacement)")
	}

	// Patch mode: use SafeUpdateWiki
	if hasPatchFlags || editSafe {
		return runEditPatch(ctx, client, profile, wikiID, c)
	}

	// Direct mode: existing behavior (last-write-wins)
	return runEditDirect(ctx, client, profile, wikiID, c)
}

func runEditDirect(ctx context.Context, client *api.Client, profile *config.ResolvedProfile, wikiID int, c *cobra.Command) error {
	input := &api.UpdateWikiInput{
		MailNotify: editMailNotify,
	}
	hasChanges := false

	if c.Flags().Changed("name") {
		input.Name = &editName
		hasChanges = true
	}
	if c.Flags().Changed("content") || editContentFile != "" {
		content, err := cmdutil.ResolveBody(editContent, editContentFile, false, nil, nil)
		if err != nil {
			return fmt.Errorf("failed to read content: %w", err)
		}
		if content != "" {
			input.Content = &content
			hasChanges = true
		}
	}

	if !hasChanges {
		return fmt.Errorf("no changes specified. Use --name, --content, or --content-file")
	}

	wiki, err := client.UpdateWiki(ctx, wikiID, input)
	if err != nil {
		return fmt.Errorf("failed to update wiki page: %w", err)
	}

	return printEditResult(profile, wiki, false)
}

func runEditPatch(ctx context.Context, client *api.Client, profile *config.ResolvedProfile, wikiID int, c *cobra.Command) error {
	// Parse patch ops
	var patchOps []api.PatchOp
	if editPatch != "" || editPatchFile != "" {
		patchJSON, err := cmdutil.ResolveBody(editPatch, editPatchFile, false, nil, nil)
		if err != nil {
			return fmt.Errorf("failed to read patch: %w", err)
		}
		patchOps, err = cmdutil.ParsePatchOps(patchJSON)
		if err != nil {
			return err
		}
	}

	// Safe full replacement content
	var fullReplace string
	if editSafe && (c.Flags().Changed("content") || editContentFile != "") {
		content, err := cmdutil.ResolveBody(editContent, editContentFile, false, nil, nil)
		if err != nil {
			return fmt.Errorf("failed to read content: %w", err)
		}
		fullReplace = content
	}

	patchFn, err := cmdutil.BuildPatchFn(patchOps, editPrepend, editAppend, fullReplace)
	if err != nil {
		return err
	}

	result, err := client.SafeUpdateWiki(ctx, wikiID, patchFn)
	if err != nil {
		var conflictErr *api.ConflictError
		if errors.As(err, &conflictErr) {
			fmt.Fprintf(os.Stderr, "%s %s\n", ui.Red("✗"), conflictErr.Error())
			fmt.Fprintf(os.Stderr, "  Hint: resolve the conflict manually, or use --content without --safe to force overwrite.\n")
			return err
		}
		return fmt.Errorf("failed to update wiki page: %w", err)
	}

	// Handle --name update separately (not part of content patching)
	if c.Flags().Changed("name") {
		updated, err := client.UpdateWiki(ctx, wikiID, &api.UpdateWikiInput{
			Name:       &editName,
			MailNotify: editMailNotify,
		})
		if err != nil {
			return fmt.Errorf("content updated but failed to rename: %w", err)
		}
		result.Wiki = updated
	}

	return printEditResult(profile, result.Wiki, result.Merged)
}

func printEditResult(profile *config.ResolvedProfile, wiki *api.Wiki, merged bool) error {
	switch profile.Output {
	case "json":
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(wiki)
	default:
		if merged {
			fmt.Printf("%s Wiki page updated (auto-merged): %s (ID: %d)\n",
				ui.Green("✓"), wiki.Name, wiki.ID)
		} else {
			fmt.Printf("%s Wiki page updated: %s (ID: %d)\n",
				ui.Green("✓"), wiki.Name, wiki.ID)
		}
		return nil
	}
}

func resolveWikiID(client *api.Client, ctx context.Context, cfg *config.Store, idOrName string) (int, error) {
	if id, err := strconv.Atoi(idOrName); err == nil {
		return id, nil
	}

	if err := cmdutil.RequireProject(cfg); err != nil {
		return 0, err
	}
	projectKey := cmdutil.GetCurrentProject(cfg)

	wikis, err := client.GetWikis(ctx, projectKey, "")
	if err != nil {
		return 0, fmt.Errorf("failed to get wiki list: %w", err)
	}

	for _, wiki := range wikis {
		if wiki.Name == idOrName {
			return wiki.ID, nil
		}
	}

	return 0, fmt.Errorf("wiki page not found: %s", idOrName)
}
