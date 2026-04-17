package document

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/yacchi/backlog-cli/packages/backlog/internal/api"
	"github.com/yacchi/backlog-cli/packages/backlog/internal/cmdutil"
	"github.com/yacchi/backlog-cli/packages/backlog/internal/ui"
)

var tagCmd = &cobra.Command{
	Use:   "tag",
	Short: "Manage document tags",
}

var tagAddCmd = &cobra.Command{
	Use:   "add <document-id>",
	Short: "Add tags to a document",
	Long: `Add one or more tags to a document.

Examples:
  backlog document tag add 01HXXXXXXXX -t foo -t bar`,
	Args: cobra.ExactArgs(1),
	RunE: runTagAdd,
}

var tagRemoveCmd = &cobra.Command{
	Use:   "remove <document-id>",
	Short: "Remove tags from a document",
	Long: `Remove one or more tags from a document.

Examples:
  backlog document tag remove 01HXXXXXXXX -t foo`,
	Args: cobra.ExactArgs(1),
	RunE: runTagRemove,
}

var tagNames []string

func init() {
	tagAddCmd.Flags().StringArrayVarP(&tagNames, "tag", "t", nil, "Tag name (can be specified multiple times)")
	_ = tagAddCmd.MarkFlagRequired("tag")
	tagCmd.AddCommand(tagAddCmd)

	// tagRemove は別の変数を使う必要があるが package-level var を共有しないよう独立フラグを定義
	tagRemoveCmd.Flags().StringArray("tag", nil, "Tag name (can be specified multiple times)")
	_ = tagRemoveCmd.MarkFlagRequired("tag")
	tagCmd.AddCommand(tagRemoveCmd)
}

func runTagAdd(c *cobra.Command, args []string) error {
	documentID := args[0]
	names, _ := c.Flags().GetStringArray("tag")

	client, cfg, err := cmdutil.GetAPIClient(c)
	if err != nil {
		return err
	}

	tags, err := client.AddDocumentTags(c.Context(), documentID, names)
	if err != nil {
		return fmt.Errorf("failed to add tags: %w", err)
	}

	return outputTags(tags, cfg.CurrentProfile().Output)
}

func runTagRemove(c *cobra.Command, args []string) error {
	documentID := args[0]
	names, _ := c.Flags().GetStringArray("tag")

	client, cfg, err := cmdutil.GetAPIClient(c)
	if err != nil {
		return err
	}

	tags, err := client.RemoveDocumentTags(c.Context(), documentID, names)
	if err != nil {
		return fmt.Errorf("failed to remove tags: %w", err)
	}

	return outputTags(tags, cfg.CurrentProfile().Output)
}

func outputTags(tags []api.DocumentTag, outputFmt string) error {
	switch outputFmt {
	case "json":
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(tags)
	default:
		table := ui.NewTable("ID", "NAME")
		for _, t := range tags {
			table.AddRow(fmt.Sprintf("%d", t.ID), t.Name)
		}
		table.RenderWithColor(os.Stdout, ui.IsColorEnabled())
		return nil
	}
}
