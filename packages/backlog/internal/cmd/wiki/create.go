package wiki

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/AlecAivazis/survey/v2"
	"github.com/spf13/cobra"
	"github.com/yacchi/backlog-cli/packages/backlog/internal/api"
	"github.com/yacchi/backlog-cli/packages/backlog/internal/cmdutil"
	"github.com/yacchi/backlog-cli/packages/backlog/internal/ui"
)

var createCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a wiki page",
	Long: `Create a new wiki page.

Examples:
  backlog wiki create --name "Meeting Notes" --content "# Meeting Notes"
  backlog wiki create --name "Spec" --content-file spec.md
  cat content.md | backlog wiki create --name "Page" --content-file -
  backlog wiki create  # Interactive mode`,
	RunE: runCreate,
}

var (
	createName        string
	createContent     string
	createContentFile string
	createMailNotify  bool
)

func init() {
	createCmd.Flags().StringVarP(&createName, "name", "n", "", "Wiki page name")
	createCmd.Flags().StringVarP(&createContent, "content", "c", "", "Wiki page content")
	createCmd.Flags().StringVarP(&createContentFile, "content-file", "F", "", "Read content from file (use \"-\" to read from standard input)")
	createCmd.Flags().BoolVar(&createMailNotify, "notify", false, "Send mail notification")
}

func runCreate(c *cobra.Command, args []string) error {
	client, cfg, err := cmdutil.GetAPIClient(c)
	if err != nil {
		return err
	}

	if err := cmdutil.RequireProject(cfg); err != nil {
		return err
	}

	projectKey := cmdutil.GetCurrentProject(cfg)

	// プロジェクトID取得
	ctx := c.Context()
	project, err := client.GetProject(ctx, projectKey)
	if err != nil {
		return fmt.Errorf("failed to get project: %w", err)
	}

	// 対話モード
	if createName == "" {
		prompt := &survey.Input{
			Message: "Wiki page name:",
		}
		if err := survey.AskOne(prompt, &createName, survey.WithValidator(survey.Required)); err != nil {
			return err
		}
	}

	createContent, err = cmdutil.ResolveBody(
		createContent,
		createContentFile,
		false,
		nil,
		func() (string, error) {
			var content string
			prompt := &survey.Multiline{
				Message: "Content (Markdown supported):",
			}
			if err := survey.AskOne(prompt, &content); err != nil {
				return "", err
			}
			return content, nil
		},
	)
	if err != nil {
		return fmt.Errorf("failed to get content: %w", err)
	}

	// Wiki作成
	input := &api.CreateWikiInput{
		ProjectID:  project.ID,
		Name:       createName,
		Content:    createContent,
		MailNotify: createMailNotify,
	}

	wiki, err := client.CreateWiki(ctx, input)
	if err != nil {
		return fmt.Errorf("failed to create wiki page: %w", err)
	}

	// 出力
	profile := cfg.CurrentProfile()
	switch profile.Output {
	case "json":
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(wiki)
	default:
		fmt.Printf("%s Wiki page created: %s (ID: %d)\n",
			ui.Green("✓"), wiki.Name, wiki.ID)
		url := fmt.Sprintf("https://%s.%s/alias/wiki/%d",
			profile.Space, profile.Domain, wiki.ID)
		fmt.Printf("URL: %s\n", ui.Cyan(url))
		return nil
	}
}
