package document

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/pkg/browser"
	"github.com/spf13/cobra"
	"github.com/yacchi/backlog-cli/packages/backlog/internal/api"
	"github.com/yacchi/backlog-cli/packages/backlog/internal/cmdutil"
	"github.com/yacchi/backlog-cli/packages/backlog/internal/ui"
)

var viewCmd = &cobra.Command{
	Use:   "view <document-id>",
	Short: "View a document",
	Long: `View detailed information about a document.

Examples:
  backlog document view 01HXXXXXXXX
  backlog document view 01HXXXXXXXX --web
  backlog document view 01HXXXXXXXX --markdown`,
	Args: cobra.ExactArgs(1),
	RunE: runView,
}

var (
	viewWeb      bool
	viewMarkdown bool
)

func init() {
	viewCmd.Flags().BoolVarP(&viewWeb, "web", "w", false, "Open in browser")
	viewCmd.Flags().BoolVar(&viewMarkdown, "markdown", false, "Display plain text content")
}

func runView(c *cobra.Command, args []string) error {
	documentID := args[0]

	client, cfg, err := cmdutil.GetAPIClient(c)
	if err != nil {
		return err
	}

	profile := cfg.CurrentProfile()

	if viewWeb {
		url := fmt.Sprintf("https://%s.%s/document/%s", profile.Space, profile.Domain, documentID)
		return browser.OpenURL(url)
	}

	doc, err := client.GetDocument(c.Context(), documentID)
	if err != nil {
		return fmt.Errorf("failed to get document: %w", err)
	}

	switch profile.Output {
	case "json":
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(doc)
	default:
		if viewMarkdown {
			fmt.Println(doc.Plain)
			return nil
		}
		return renderDocumentDetail(doc, profile.Space, profile.Domain)
	}
}

func renderDocumentDetail(doc *api.DocumentDetail, space, domain string) error {
	title := doc.Title
	if doc.Emoji != "" {
		title = doc.Emoji + " " + title
	}
	fmt.Printf("%s\n", ui.Bold(title))
	fmt.Println(strings.Repeat("─", 60))

	if len(doc.Tags) > 0 {
		fmt.Printf("Tags:    %s\n", ui.Cyan(joinTags(doc.Tags)))
	}

	fmt.Printf("Created: %s by %s\n", formatDate(doc.Created), doc.CreatedUser.Name)
	if doc.UpdatedUser != nil {
		fmt.Printf("Updated: %s by %s\n", formatDate(doc.Updated), doc.UpdatedUser.Name)
	}

	if len(doc.Attachments) > 0 {
		fmt.Printf("Attachments: %d file(s)\n", len(doc.Attachments))
		for _, a := range doc.Attachments {
			fmt.Printf("  - [%d] %s\n", a.ID, a.Name)
		}
	}

	docURL := fmt.Sprintf("https://%s.%s/document/%s", space, domain, doc.ID)

	if doc.Plain != "" {
		fmt.Println()
		fmt.Println(ui.Bold("Content"))
		fmt.Println(strings.Repeat("─", 60))
		fmt.Println(doc.Plain)
	}

	fmt.Println()
	fmt.Printf("URL: %s\n", ui.Cyan(docURL))
	return nil
}

func formatDate(dateStr string) string {
	if len(dateStr) >= 16 {
		return dateStr[:10] + " " + dateStr[11:16]
	}
	return dateStr
}
