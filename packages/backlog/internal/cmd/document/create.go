package document

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
	Short: "Create a document",
	Long: `Create a new document in the project.

Examples:
  backlog document create --title "Design Doc" --content "# Design"
  backlog document create --title "Notes" --content-file notes.md
  cat doc.md | backlog document create --title "Doc" --content-file -
  backlog document create --title "Sub" --parent <parent-id>`,
	RunE: runCreate,
}

var (
	createTitle       string
	createContent     string
	createContentFile string
	createEmoji       string
	createParentID    string
	createAddLast     bool
)

func init() {
	createCmd.Flags().StringVarP(&createTitle, "title", "t", "", "Document title")
	createCmd.Flags().StringVarP(&createContent, "content", "c", "", "Document content (Markdown)")
	createCmd.Flags().StringVarP(&createContentFile, "content-file", "F", "", "Read content from file (use \"-\" for stdin)")
	createCmd.Flags().StringVar(&createEmoji, "emoji", "", "Emoji for the document title")
	createCmd.Flags().StringVar(&createParentID, "parent", "", "Parent document ID")
	createCmd.Flags().BoolVar(&createAddLast, "add-last", false, "Add at the end (default: prepend)")
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
	ctx := c.Context()

	project, err := client.GetProject(ctx, projectKey)
	if err != nil {
		return fmt.Errorf("failed to get project: %w", err)
	}

	if createTitle == "" {
		prompt := &survey.Input{Message: "Document title:"}
		if err := survey.AskOne(prompt, &createTitle, survey.WithValidator(survey.Required)); err != nil {
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
			prompt := &survey.Multiline{Message: "Content (Markdown supported):"}
			if err := survey.AskOne(prompt, &content); err != nil {
				return "", err
			}
			return content, nil
		},
	)
	if err != nil {
		return fmt.Errorf("failed to get content: %w", err)
	}

	input := &api.CreateDocumentInput{
		ProjectID: project.ID,
		Title:     createTitle,
		Content:   createContent,
		Emoji:     createEmoji,
		ParentID:  createParentID,
		AddLast:   createAddLast,
	}

	doc, err := client.CreateDocument(ctx, input)
	if err != nil {
		return fmt.Errorf("failed to create document: %w", err)
	}

	profile := cfg.CurrentProfile()
	switch profile.Output {
	case "json":
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(doc)
	default:
		fmt.Printf("%s Document created: %s (ID: %s)\n", ui.Green("✓"), doc.Title, doc.ID)
		url := fmt.Sprintf("https://%s.%s/document/%s", profile.Space, profile.Domain, doc.ID)
		fmt.Printf("URL: %s\n", ui.Cyan(url))
		return nil
	}
}
