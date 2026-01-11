package wiki

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"

	"github.com/spf13/cobra"
	"github.com/yacchi/backlog-cli/packages/backlog/internal/api"
	"github.com/yacchi/backlog-cli/packages/backlog/internal/cmdutil"
	"github.com/yacchi/backlog-cli/packages/backlog/internal/ui"
)

var editCmd = &cobra.Command{
	Use:   "edit <id-or-name>",
	Short: "Edit a wiki page",
	Long: `Edit an existing wiki page.

Examples:
  backlog wiki edit 123 --content "Updated content"
  backlog wiki edit "Meeting Notes" --name "Weekly Meeting Notes"
  backlog wiki edit 123 --notify`,
	Args: cobra.ExactArgs(1),
	RunE: runEdit,
}

var (
	editName       string
	editContent    string
	editMailNotify bool
)

func init() {
	editCmd.Flags().StringVarP(&editName, "name", "n", "", "New wiki page name")
	editCmd.Flags().StringVarP(&editContent, "content", "c", "", "New wiki page content")
	editCmd.Flags().BoolVar(&editMailNotify, "notify", false, "Send mail notification")
}

func runEdit(c *cobra.Command, args []string) error {
	client, cfg, err := cmdutil.GetAPIClient(c)
	if err != nil {
		return err
	}

	if err := cmdutil.RequireProject(cfg); err != nil {
		return err
	}

	projectKey := cmdutil.GetCurrentProject(cfg)
	profile := cfg.CurrentProfile()
	idOrName := args[0]
	ctx := c.Context()

	// Wiki IDを解決
	wikiID, err := resolveWikiID(client, ctx, projectKey, idOrName)
	if err != nil {
		return err
	}

	// 更新入力を構築
	input := &api.UpdateWikiInput{
		MailNotify: editMailNotify,
	}
	hasChanges := false

	if c.Flags().Changed("name") {
		input.Name = &editName
		hasChanges = true
	}
	if c.Flags().Changed("content") {
		input.Content = &editContent
		hasChanges = true
	}

	if !hasChanges {
		return fmt.Errorf("no changes specified. Use --name or --content")
	}

	// 更新実行
	wiki, err := client.UpdateWiki(ctx, wikiID, input)
	if err != nil {
		return fmt.Errorf("failed to update wiki page: %w", err)
	}

	// 出力
	switch profile.Output {
	case "json":
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(wiki)
	default:
		fmt.Printf("%s Wiki page updated: %s (ID: %d)\n",
			ui.Green("✓"), wiki.Name, wiki.ID)
		return nil
	}
}

func resolveWikiID(client *api.Client, ctx context.Context, projectKey, idOrName string) (int, error) {
	// まずIDとして解釈
	if id, err := strconv.Atoi(idOrName); err == nil {
		return id, nil
	}

	// 名前で検索
	wikis, err := client.GetWikis(ctx, projectKey)
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
