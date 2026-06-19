package issue

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"slices"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	"github.com/yacchi/backlog-cli/packages/backlog/internal/api"
	"github.com/yacchi/backlog-cli/packages/backlog/internal/cmdutil"
	"github.com/yacchi/backlog-cli/packages/backlog/internal/ui"
)

var createCmd = &cobra.Command{
	Use:     "create",
	Aliases: []string{"new"},
	Short:   "Create a new issue",
	Long: `Create a new issue in the project.

Without the title or body text supplied through flags, the command will
interactively prompt for the required information. When standard input is
not a terminal, provide --title, --type, and --priority.

Examples:
  # Interactive mode
  backlog issue create

  # Non-interactive mode
  backlog issue create --title "Bug fix" --body "Details here" --type Bug --priority 3 --assignee @me

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
	createTitle       string
	createBody        string
	createBodyFile    string
	createType        string
	createPriority    int
	createAssignee    string
	createDueDate     string
	createEditor      bool
	createMilestones  string
	createCategories  string
	createAttachFiles []string
)

type createPromptState struct {
	Title    string
	Type     string
	Priority int
}

func init() {
	createCmd.Flags().StringVarP(&createTitle, "title", "t", "", "Issue title (summary)")
	createCmd.Flags().StringVarP(&createBody, "body", "b", "", "Issue body (description)")
	createCmd.Flags().StringVarP(&createBodyFile, "body-file", "F", "", "Read body text from file (use \"-\" to read from standard input)")
	createCmd.Flags().StringVar(&createType, "type", "", "Issue type ID or name")
	createCmd.Flags().IntVar(&createPriority, "priority", 0, "Priority ID")
	createCmd.Flags().StringVarP(&createAssignee, "assignee", "a", "", "Assignee (user ID, userId, display name, or @me)")
	createCmd.Flags().StringVar(&createDueDate, "due", "", "Due date (YYYY-MM-DD)")
	createCmd.Flags().BoolVarP(&createEditor, "editor", "e", false, "Open editor to write the body")
	createCmd.Flags().StringVarP(&createMilestones, "milestone", "m", "", "Milestone IDs or names (comma-separated)")
	createCmd.Flags().StringVar(&createCategories, "category", "", "Category IDs or names (comma-separated)")
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

	interactive := ui.IsInteractiveInput()
	if !interactive {
		if err := validateNonInteractiveCreateFlags(createPromptState{
			Title:    createTitle,
			Type:     createType,
			Priority: createPriority,
		}, issueTypes); err != nil {
			return err
		}
	}

	// 入力
	input := &api.CreateIssueInput{
		ProjectID: project.ID,
	}

	// 件名（タイトル）
	if createTitle != "" {
		input.Summary = createTitle
	} else if interactive {
		input.Summary, err = ui.Input("Title:", "")
		if err != nil {
			return err
		}
		if input.Summary == "" {
			return fmt.Errorf("title is required")
		}
	} else {
		return fmt.Errorf("--title is required when not running interactively")
	}

	// 課題種別
	if createType != "" {
		issueTypeIDs, err := cmdutil.ResolveIssueTypeIDs(ctx, client, projectKey, createType)
		if err != nil {
			return fmt.Errorf("failed to resolve issue type: %w", err)
		}
		input.IssueTypeID = issueTypeIDs[0]
	} else if interactive {
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
	} else {
		return fmt.Errorf("--type is required when not running interactively")
	}

	// 優先度
	if createPriority > 0 {
		input.PriorityID = createPriority
	} else if interactive {
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
	} else {
		return fmt.Errorf("--priority is required when not running interactively")
	}

	// 担当者
	if createAssignee != "" {
		assigneeID, err := cmdutil.ResolveProjectAssigneeID(ctx, client, projectKey, createAssignee)
		if err != nil {
			return fmt.Errorf("failed to resolve assignee: %w", err)
		}
		input.AssigneeID = assigneeID
	} else if interactive {
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
	var interactiveBodyInput func() (string, error)
	if interactive {
		interactiveBodyInput = func() (string, error) {
			return ui.Input("Body (optional):", "")
		}
	}
	input.Description, err = cmdutil.ResolveBody(
		createBody,
		createBodyFile,
		createEditor,
		openEditor,
		interactiveBodyInput,
	)
	if err != nil {
		return fmt.Errorf("failed to get body: %w", err)
	}

	// 期限
	if createDueDate != "" {
		input.DueDate = createDueDate
	}

	// マイルストーン
	if createMilestones != "" {
		milestoneIDs, err := cmdutil.ResolveMilestoneIDs(ctx, client, projectKey, createMilestones)
		if err != nil {
			return fmt.Errorf("failed to resolve milestones: %w", err)
		}
		input.MilestoneIDs = milestoneIDs
	}

	// カテゴリ
	if createCategories != "" {
		categoryIDs, err := cmdutil.ResolveCategoryIDs(ctx, client, projectKey, createCategories)
		if err != nil {
			return fmt.Errorf("failed to resolve categories: %w", err)
		}
		input.CategoryIDs = categoryIDs
	}

	// 添付ファイルのアップロード
	if len(createAttachFiles) > 0 {
		attachmentIDs, err := cmdutil.UploadFiles(ctx, client, createAttachFiles)
		if err != nil {
			return err
		}
		input.AttachmentIDs = attachmentIDs
	}

	// 作成
	profile := cfg.CurrentProfile()
	if profile.Output != "json" {
		fmt.Println("Creating issue...")
	}
	issue, err := client.CreateIssue(ctx, input)
	if err != nil {
		return fmt.Errorf("failed to create issue: %w", err)
	}

	switch profile.Output {
	case "json":
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(issue)
	default:
		ui.Success("Created issue %s", issue.IssueKey.Value)
		url := fmt.Sprintf("https://%s/view/%s", profile.Space, issue.IssueKey.Value)
		fmt.Printf("URL: %s\n", ui.Cyan(url))
		return nil
	}
}

func validateNonInteractiveCreateFlags(state createPromptState, issueTypes []api.IssueType) error {
	var missing []string
	if state.Title == "" {
		missing = append(missing, "--title")
	}
	if state.Type == "" {
		missing = append(missing, "--type")
	}
	if state.Priority <= 0 {
		missing = append(missing, "--priority")
	}
	if len(missing) == 0 {
		return nil
	}

	lines := []string{
		fmt.Sprintf("%s required when not running interactively", joinCreateFlags(missing)),
	}

	if slices.Contains(missing, "--title") {
		lines = append(lines, "", "Use --title <text> to set the issue title.")
	}
	if slices.Contains(missing, "--type") {
		lines = append(lines, "", "Use one of the following for --type:")
		if len(issueTypes) == 0 {
			lines = append(lines, "  --type <issue-type-id|issue-type-name>")
		} else {
			for _, issueType := range issueTypes {
				lines = append(lines, fmt.Sprintf("  --type %s # ID: %d", issueType.Name, issueType.ID))
			}
		}
	}
	if slices.Contains(missing, "--priority") {
		lines = append(lines, "",
			"Use one of the following for --priority:",
			"  --priority 2 # 高",
			"  --priority 3 # 中",
			"  --priority 4 # 低",
		)
	}

	lines = append(lines, "", "Run 'backlog issue create --help' for usage.")
	return errors.New(strings.Join(lines, "\n"))
}

func joinCreateFlags(flags []string) string {
	switch len(flags) {
	case 0:
		return ""
	case 1:
		return flags[0]
	case 2:
		return flags[0] + " and " + flags[1]
	default:
		return strings.Join(flags[:len(flags)-1], ", ") + ", and " + flags[len(flags)-1]
	}
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
