package document

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/yacchi/backlog-cli/packages/backlog/internal/api"
	"github.com/yacchi/backlog-cli/packages/backlog/internal/cmdutil"
	"github.com/yacchi/backlog-cli/packages/backlog/internal/ui"
)

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List documents",
	Long: `List documents in the project.

Examples:
  backlog document list
  backlog document list --keyword "design"
  backlog document list --sort updated --order asc
  backlog document list --limit 50`,
	RunE: runList,
}

var (
	listKeyword string
	listSort    string
	listOrder   string
	listLimit   int
	listCount   bool
)

func init() {
	listCmd.Flags().StringVarP(&listKeyword, "keyword", "S", "", "Filter by keyword")
	listCmd.Flags().StringVar(&listSort, "sort", "", "Sort field (created, updated)")
	listCmd.Flags().StringVar(&listOrder, "order", "desc", "Sort order (asc, desc)")
	listCmd.Flags().IntVarP(&listLimit, "limit", "L", 20, "Maximum number of documents to fetch (1-100)")
	listCmd.Flags().BoolVar(&listCount, "count", false, "Show only the count of documents")
}

func runList(c *cobra.Command, args []string) error {
	client, cfg, err := cmdutil.GetAPIClient(c)
	if err != nil {
		return err
	}

	if err := cmdutil.RequireProject(cfg); err != nil {
		return err
	}

	projectKey := cmdutil.GetCurrentProject(cfg)
	ctx := c.Context()

	if listCount {
		count, err := client.GetDocumentCount(ctx, projectKey)
		if err != nil {
			return fmt.Errorf("failed to get document count: %w", err)
		}
		fmt.Println(count)
		return nil
	}

	// projectId を解決
	project, err := client.GetProject(ctx, projectKey)
	if err != nil {
		return fmt.Errorf("failed to get project: %w", err)
	}

	count := listLimit
	if count < 1 {
		count = 20
	} else if count > 100 {
		count = 100
	}

	opts := &api.DocumentListOptions{
		ProjectIDs: []int{project.ID},
		Keyword:    listKeyword,
		Sort:       listSort,
		Order:      listOrder,
		Offset:     0,
		Count:      count,
	}

	docs, err := client.GetDocuments(ctx, opts)
	if err != nil {
		return fmt.Errorf("failed to get documents: %w", err)
	}

	if len(docs) == 0 {
		fmt.Fprintln(os.Stderr, "No documents found")
		return nil
	}

	profile := cfg.CurrentProfile()
	switch profile.Output {
	case "json":
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(docs)
	default:
		outputDocumentTable(docs)
		return nil
	}
}

func outputDocumentTable(docs []api.Document) {
	table := ui.NewTable("ID", "TITLE", "TAGS", "UPDATED_BY", "UPDATED")

	for _, d := range docs {
		tags := joinTags(d.Tags)
		updatedBy := ""
		if d.UpdatedUser != nil {
			updatedBy = d.UpdatedUser.Name
		} else {
			updatedBy = d.CreatedUser.Name
		}
		updated := ""
		if d.Updated != "" && len(d.Updated) >= 10 {
			updated = d.Updated[:10]
		} else if len(d.Created) >= 10 {
			updated = d.Created[:10]
		}

		table.AddRow(
			d.ID,
			truncate(d.Title, 40),
			truncate(tags, 20),
			updatedBy,
			updated,
		)
	}

	table.RenderWithColor(os.Stdout, ui.IsColorEnabled())
}

func joinTags(tags []api.DocumentTag) string {
	names := make([]string, 0, len(tags))
	for _, t := range tags {
		names = append(names, t.Name)
	}
	return strings.Join(names, ", ")
}

func truncate(s string, max int) string {
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	return string(runes[:max-3]) + "..."
}
