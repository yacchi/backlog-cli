package issue

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	"github.com/yacchi/backlog-cli/internal/api"
	"github.com/yacchi/backlog-cli/internal/cmdutil"
	"github.com/yacchi/backlog-cli/internal/ui"
)

var createCmd = &cobra.Command{
	Use:     "create",
	Aliases: []string{"new"},
	Short:   "Create a new issue",
	Long: `Create a new issue in the project.

Without the title or body text supplied through flags, the command will
interactively prompt for the required information.

Examples:
  # Interactive mode
  backlog issue create

  # Non-interactive mode
  backlog issue create --title "Bug fix" --body "Details here" --type 1

  # Read body from file
  backlog issue create --title "Feature" --body-file spec.md

  # Read body from stdin
  cat description.md | backlog issue create --title "New feature" --body-file -

  # Open editor for body
  backlog issue create --title "Bug" --editor

  # Assign to yourself
  backlog issue create -t "Task" -a @me`,
	RunE: runCreate,
}

var (
	createTitle    string
	createBody     string
	createBodyFile string
	createTypeID   int
	createPriority int
	createAssignee string
	createDueDate  string
	createEditor   bool
)

func init() {
	createCmd.Flags().StringVarP(&createTitle, "title", "t", "", "Issue title (summary)")
	createCmd.Flags().StringVarP(&createBody, "body", "b", "", "Issue body (description)")
	createCmd.Flags().StringVarP(&createBodyFile, "body-file", "F", "", "Read body text from file (use \"-\" to read from standard input)")
	createCmd.Flags().IntVar(&createTypeID, "type", 0, "Issue type ID")
	createCmd.Flags().IntVar(&createPriority, "priority", 0, "Priority ID")
	createCmd.Flags().StringVarP(&createAssignee, "assignee", "a", "", "Assignee (user ID or @me)")
	createCmd.Flags().StringVar(&createDueDate, "due", "", "Due date (YYYY-MM-DD)")
	createCmd.Flags().BoolVarP(&createEditor, "editor", "e", false, "Open editor to write the body")
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

	// プロジェクト情報取得
	project, err := client.GetProject(ctx, projectKey)
	if err != nil {
		return fmt.Errorf("failed to get project: %w", err)
	}

	// 課題種別を取得
	issueTypes, err := client.GetIssueTypes(ctx, projectKey)
	if err != nil {
		return fmt.Errorf("failed to get issue types: %w", err)
	}

	// 入力
	input := &api.CreateIssueInput{
		ProjectID: project.ID,
	}

	// 件名（タイトル）
	if createTitle != "" {
		input.Summary = createTitle
	} else {
		input.Summary, err = ui.Input("Title:", "")
		if err != nil {
			return err
		}
		if input.Summary == "" {
			return fmt.Errorf("title is required")
		}
	}

	// 課題種別
	if createTypeID > 0 {
		input.IssueTypeID = createTypeID
	} else {
		typeOpts := make([]ui.SelectOption, len(issueTypes))
		for i, t := range issueTypes {
			typeOpts[i] = ui.SelectOption{
				Value:       fmt.Sprintf("%d", t.ID),
				Description: t.Name,
			}
		}

		selected, err := ui.SelectWithDesc("Issue type:", typeOpts)
		if err != nil {
			return err
		}
		input.IssueTypeID, _ = strconv.Atoi(selected)
	}

	// 優先度
	if createPriority > 0 {
		input.PriorityID = createPriority
	} else {
		// デフォルトは「中」(ID=3)
		priorityOpts := []ui.SelectOption{
			{Value: "2", Description: "高"},
			{Value: "3", Description: "中"},
			{Value: "4", Description: "低"},
		}

		selected, err := ui.SelectWithDesc("Priority:", priorityOpts)
		if err != nil {
			return err
		}
		input.PriorityID, _ = strconv.Atoi(selected)
	}

	// 担当者
	if createAssignee == "@me" {
		me, err := client.GetCurrentUser(ctx)
		if err != nil {
			return fmt.Errorf("failed to get current user: %w", err)
		}
		input.AssigneeID = me.ID.Value
	} else if createAssignee != "" {
		assigneeID, err := strconv.Atoi(createAssignee)
		if err != nil {
			return fmt.Errorf("invalid assignee ID: %s", createAssignee)
		}
		input.AssigneeID = assigneeID
	} else {
		// 担当者選択（オプション）
		users, err := client.GetProjectUsers(ctx, projectKey)
		if err == nil && len(users) > 0 {
			userOpts := make([]ui.SelectOption, len(users)+1)
			userOpts[0] = ui.SelectOption{Value: "0", Description: "(unassigned)"}
			for i, u := range users {
				userOpts[i+1] = ui.SelectOption{
					Value:       fmt.Sprintf("%d", u.ID),
					Description: u.Name,
				}
			}

			selected, err := ui.SelectWithDesc("Assignee:", userOpts)
			if err != nil {
				return err
			}
			input.AssigneeID, _ = strconv.Atoi(selected)
		}
	}

	// 説明（ボディ）
	input.Description, err = cmdutil.ResolveBody(
		createBody,
		createBodyFile,
		createEditor,
		openEditor,
		func() (string, error) {
			return ui.Input("Body (optional):", "")
		},
	)
	if err != nil {
		return fmt.Errorf("failed to get body: %w", err)
	}

	// 期限
	if createDueDate != "" {
		input.DueDate = createDueDate
	}

	// 作成
	fmt.Println("Creating issue...")
	issue, err := client.CreateIssue(ctx, input)
	if err != nil {
		return fmt.Errorf("failed to create issue: %w", err)
	}

	ui.Success("Created issue %s", issue.IssueKey.Value)

	profile := cfg.CurrentProfile()
	url := fmt.Sprintf("https://%s.%s/view/%s", profile.Space, profile.Domain, issue.IssueKey.Value)
	fmt.Printf("URL: %s\n", ui.Cyan(url))

	return nil
}

func openEditor(initial string) (string, error) {
	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = "vi"
	}

	// 一時ファイル作成
	tmpfile, err := os.CreateTemp("", "backlog-*.md")
	if err != nil {
		return "", err
	}
	defer func() { _ = os.Remove(tmpfile.Name()) }()

	if initial != "" {
		_, _ = tmpfile.WriteString(initial)
	}
	_ = tmpfile.Close()

	// エディタ起動
	editorCmd := exec.Command(editor, tmpfile.Name())
	editorCmd.Stdin = os.Stdin
	editorCmd.Stdout = os.Stdout
	editorCmd.Stderr = os.Stderr

	if err := editorCmd.Run(); err != nil {
		return "", err
	}

	// 内容読み込み
	content, err := os.ReadFile(tmpfile.Name())
	if err != nil {
		return "", err
	}

	return strings.TrimSpace(string(content)), nil
}
