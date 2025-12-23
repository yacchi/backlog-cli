package wiki

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/pkg/browser"
	"github.com/spf13/cobra"
	"github.com/yacchi/backlog-cli/internal/api"
	"github.com/yacchi/backlog-cli/internal/cmdutil"
	"github.com/yacchi/backlog-cli/internal/config"
	"github.com/yacchi/backlog-cli/internal/ui"
)

var viewCmd = &cobra.Command{
	Use:   "view <wiki-id>",
	Short: "View a wiki page",
	Long: `View detailed information about a wiki page.

Examples:
  backlog wiki view 12345
  backlog wiki view 12345 --web`,
	Args: cobra.ExactArgs(1),
	RunE: runView,
}

var (
	viewWeb           bool
	viewMarkdown      bool
	viewRaw           bool
	viewMarkdownWarn  bool
	viewMarkdownCache bool
)

func init() {
	viewCmd.Flags().BoolVarP(&viewWeb, "web", "w", false, "Open in browser")
	viewCmd.Flags().BoolVar(&viewMarkdown, "markdown", false, "Render markdown by converting Backlog notation to GFM")
	viewCmd.Flags().BoolVar(&viewRaw, "raw", false, "Render raw content without markdown conversion")
	viewCmd.Flags().BoolVar(&viewMarkdownWarn, "markdown-warn", false, "Show markdown conversion warnings")
	viewCmd.Flags().BoolVar(&viewMarkdownCache, "markdown-cache", false, "Cache markdown conversion analysis data")
}

func runView(c *cobra.Command, args []string) error {
	wikiID, err := strconv.Atoi(args[0])
	if err != nil {
		return fmt.Errorf("invalid wiki ID: %s", args[0])
	}

	client, cfg, err := cmdutil.GetAPIClient(c)
	if err != nil {
		return err
	}

	profile := cfg.CurrentProfile()

	// ブラウザで開く
	if viewWeb {
		url := fmt.Sprintf("https://%s.%s/alias/wiki/%d",
			profile.Space, profile.Domain, wikiID)
		return browser.OpenURL(url)
	}

	// Wiki取得
	wiki, err := client.GetWiki(c.Context(), wikiID)
	if err != nil {
		return fmt.Errorf("failed to get wiki page: %w", err)
	}

	// 出力
	switch profile.Output {
	case "json":
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(wiki)
	default:
		display := cfg.Display()
		markdownOpts := cmdutil.ResolveMarkdownViewOptions(c, display)
		projectKey := cmdutil.GetCurrentProject(cfg)
		return renderWikiDetail(wiki, profile, projectKey, markdownOpts, c.OutOrStdout())
	}
}

func renderWikiDetail(wiki *api.Wiki, profile *config.ResolvedProfile, projectKey string, markdownOpts cmdutil.MarkdownViewOptions, out io.Writer) error {
	// ヘッダー
	fmt.Printf("%s\n", ui.Bold(wiki.Name))
	fmt.Println(strings.Repeat("─", 60))

	// タグ
	if len(wiki.Tags) > 0 {
		tags := ""
		for i, tag := range wiki.Tags {
			if i > 0 {
				tags += ", "
			}
			tags += tag.Name
		}
		fmt.Printf("Tags:    %s\n", ui.Cyan(tags))
	}

	// 作成者と日時
	fmt.Printf("Created: %s by %s\n", formatDate(wiki.Created), wiki.CreatedUser.Name)
	if wiki.UpdatedUser != nil {
		fmt.Printf("Updated: %s by %s\n", formatDate(wiki.Updated), wiki.UpdatedUser.Name)
	}

	// 添付ファイル
	if len(wiki.Attachments) > 0 {
		fmt.Printf("Attachments: %d file(s)\n", len(wiki.Attachments))
	}

	// 内容
	if wiki.Content != "" {
		fmt.Println()
		fmt.Println(ui.Bold("Content"))
		fmt.Println(strings.Repeat("─", 60))
		content := wiki.Content
		if markdownOpts.Enable {
			rendered, err := cmdutil.RenderMarkdownContent(content, markdownOpts, "wiki", wiki.ID, 0, projectKey, out)
			if err != nil {
				return err
			}
			content = rendered
		}
		fmt.Println(content)
	}

	// URL
	fmt.Println()
	url := fmt.Sprintf("https://%s.%s/alias/wiki/%d",
		profile.Space, profile.Domain, wiki.ID)
	fmt.Printf("URL: %s\n", ui.Cyan(url))

	return nil
}

func formatDate(dateStr string) string {
	if len(dateStr) >= 16 {
		return dateStr[:10] + " " + dateStr[11:16]
	}
	return dateStr
}
