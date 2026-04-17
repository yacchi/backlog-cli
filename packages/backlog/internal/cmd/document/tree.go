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

var treeCmd = &cobra.Command{
	Use:   "tree",
	Short: "Show document tree",
	Long: `Show the document hierarchy as a tree.

Examples:
  backlog document tree
  backlog document tree --include-trash`,
	RunE: runTree,
}

var treeIncludeTrash bool

func init() {
	treeCmd.Flags().BoolVar(&treeIncludeTrash, "include-trash", false, "Also show trashed documents")
}

func runTree(c *cobra.Command, args []string) error {
	client, cfg, err := cmdutil.GetAPIClient(c)
	if err != nil {
		return err
	}

	if err := cmdutil.RequireProject(cfg); err != nil {
		return err
	}

	projectKey := cmdutil.GetCurrentProject(cfg)

	tree, err := client.GetDocumentTree(c.Context(), projectKey)
	if err != nil {
		return fmt.Errorf("failed to get document tree: %w", err)
	}

	profile := cfg.CurrentProfile()
	switch profile.Output {
	case "json":
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(tree)
	default:
		if tree.ActiveTree != nil {
			fmt.Println(ui.Bold("Documents"))
			printTree(tree.ActiveTree.Children, "")
		} else {
			fmt.Fprintln(os.Stderr, "No documents found")
		}
		if treeIncludeTrash && tree.TrashTree != nil {
			fmt.Println()
			fmt.Println(ui.Bold("Trash"))
			printTree(tree.TrashTree.Children, "")
		}
		return nil
	}
}

func printTree(nodes []api.DocumentTreeNode, prefix string) {
	for i, node := range nodes {
		isLast := i == len(nodes)-1
		connector := "├─"
		childPrefix := prefix + "│  "
		if isLast {
			connector = "└─"
			childPrefix = prefix + "   "
		}

		label := node.Name
		if node.Emoji != "" {
			label = node.Emoji + " " + label
		}
		fmt.Printf("%s%s %s\n", prefix, connector, label)

		if len(node.Children) > 0 {
			printTree(node.Children, childPrefix)
		}
	}
}
