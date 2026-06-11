package wiki

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

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
  backlog wiki create --name "Design Doc" --content "..." --attach diagram.png
  backlog wiki create  # Interactive mode`,
	RunE: runCreate,
}

var (
	createName        string
	createContent     string
	createContentFile string
	createMailNotify  bool
	createAttachFiles []string
)

func init() {
	createCmd.Flags().StringVarP(&createName, "name", "n", "", "Wiki page name")
	createCmd.Flags().StringVarP(&createContent, "content", "c", "", "Wiki page content")
	createCmd.Flags().StringVarP(&createContentFile, "content-file", "F", "", "Read content from file (use \"-\" to read from standard input)")
	createCmd.Flags().BoolVar(&createMailNotify, "notify", false, "Send mail notification")
	createCmd.Flags().StringArrayVar(&createAttachFiles, "attach", nil, "Attach local file(s) by path (can be specified multiple times)")
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
	interactive := ui.IsInteractiveInput()
	project, err := client.GetProject(ctx, projectKey)
	if err != nil {
		return fmt.Errorf("failed to get project: %w", err)
	}

	// 対話モード
	if createName == "" {
		if !interactive {
			return cmdutil.NonInteractiveFlagError(
				"--name is required when not running interactively",
				"backlog wiki create",
				"Use --name <text> to create a wiki page without prompts.",
			)
		}
		prompt := &survey.Input{
			Message: "Wiki page name:",
		}
		if err := survey.AskOne(prompt, &createName, survey.WithValidator(survey.Required)); err != nil {
			return err
		}
	}

	var interactiveContentInput func() (string, error)
	if interactive {
		interactiveContentInput = func() (string, error) {
			var content string
			prompt := &survey.Multiline{
				Message: "Content (Markdown supported):",
			}
			if err := survey.AskOne(prompt, &content); err != nil {
				return "", err
			}
			return content, nil
		}
	}
	createContent, err = cmdutil.ResolveBody(createContent, createContentFile, false, nil, interactiveContentInput)
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

	// 添付ファイルのアップロード（Wiki作成APIは添付に非対応のため、作成後に紐付ける）
	if len(createAttachFiles) > 0 {
		var attachmentIDs []int
		for _, filePath := range createAttachFiles {
			f, err := os.Open(filePath)
			if err != nil {
				return fmt.Errorf("failed to open %s: %w", filePath, err)
			}
			up, err := client.UploadSpaceAttachment(ctx, filepath.Base(filePath), f)
			_ = f.Close()
			if err != nil {
				return fmt.Errorf("failed to upload %s: %w", filePath, err)
			}
			attachmentIDs = append(attachmentIDs, up.ID)
		}
		if _, err := client.AttachFilesToWiki(ctx, wiki.ID, attachmentIDs); err != nil {
			return fmt.Errorf("failed to attach files to wiki %d: %w", wiki.ID, err)
		}
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
		url := fmt.Sprintf("https://%s/alias/wiki/%d",
			profile.Space, wiki.ID)
		fmt.Printf("URL: %s\n", ui.Cyan(url))
		return nil
	}
}
