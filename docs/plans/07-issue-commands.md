# Phase 07: 課題コマンド

## 目標

- `backlog issue list` - 課題一覧表示
- `backlog issue view` - 課題詳細表示
- `backlog issue create` - 課題作成

## 1. 共通ヘルパー

### internal/cmd/helpers.go

```go
package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/yourorg/backlog-cli/internal/api"
	"github.com/yourorg/backlog-cli/internal/config"
)

// GetResolvedConfig はコマンドフラグと設定からResolvedConfigを取得する
func GetResolvedConfig(cmd *cobra.Command) (*config.Config, *config.ResolvedConfig, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to load config: %w", err)
	}
	
	// コマンドフラグを取得
	project, _ := cmd.Flags().GetString("project")
	space, _ := cmd.Flags().GetString("space")
	domain, _ := cmd.Flags().GetString("domain")
	output, _ := cmd.Flags().GetString("output")
	
	resolved, err := config.Resolve(cfg, config.ResolveOptions{
		Project: project,
		Space:   space,
		Domain:  domain,
		Output:  output,
	})
	if err != nil {
		return nil, nil, err
	}
	
	return cfg, resolved, nil
}

// GetAPIClient は認証済みAPIクライアントを取得する
func GetAPIClient(cmd *cobra.Command) (*api.Client, *config.ResolvedConfig, error) {
	cfg, resolved, err := GetResolvedConfig(cmd)
	if err != nil {
		return nil, nil, err
	}
	
	if resolved.Space == "" || resolved.Domain == "" {
		return nil, nil, fmt.Errorf("space and domain are required\nRun 'backlog auth login' first")
	}
	
	client, err := api.NewClientFromConfig(cfg, resolved)
	if err != nil {
		return nil, nil, fmt.Errorf("authentication required\nRun 'backlog auth login' first")
	}
	
	return client, resolved, nil
}

// RequireProject はプロジェクトが設定されていることを確認する
func RequireProject(resolved *config.ResolvedConfig) error {
	if resolved.Project == "" {
		return fmt.Errorf("project is required\nSpecify with -p/--project flag or set default with 'backlog config set client.default.project <key>'")
	}
	return nil
}
```

## 2. テーブル出力

### internal/ui/table.go

```go
package ui

import (
	"fmt"
	"io"
	"os"
	"strings"
	"text/tabwriter"
)

// Table はテーブル出力
type Table struct {
	writer  *tabwriter.Writer
	headers []string
	rows    [][]string
}

// NewTable は新しいテーブルを作成する
func NewTable(headers ...string) *Table {
	return &Table{
		headers: headers,
		rows:    make([][]string, 0),
	}
}

// AddRow は行を追加する
func (t *Table) AddRow(values ...string) {
	t.rows = append(t.rows, values)
}

// Render はテーブルを出力する
func (t *Table) Render(w io.Writer) {
	if w == nil {
		w = os.Stdout
	}
	
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	
	// ヘッダー
	fmt.Fprintln(tw, strings.Join(t.headers, "\t"))
	
	// 行
	for _, row := range t.rows {
		fmt.Fprintln(tw, strings.Join(row, "\t"))
	}
	
	tw.Flush()
}

// RenderWithColor は色付きでテーブルを出力する
func (t *Table) RenderWithColor(w io.Writer, colorEnabled bool) {
	if !colorEnabled {
		t.Render(w)
		return
	}
	
	if w == nil {
		w = os.Stdout
	}
	
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	
	// ヘッダー（太字）
	headerLine := make([]string, len(t.headers))
	for i, h := range t.headers {
		headerLine[i] = Bold(h)
	}
	fmt.Fprintln(tw, strings.Join(headerLine, "\t"))
	
	// 行
	for _, row := range t.rows {
		fmt.Fprintln(tw, strings.Join(row, "\t"))
	}
	
	tw.Flush()
}
```

### internal/ui/color.go

```go
package ui

import (
	"fmt"
	"os"

	"golang.org/x/term"
)

var colorEnabled = true

func init() {
	// 色が使えるかチェック
	colorEnabled = term.IsTerminal(int(os.Stdout.Fd()))
}

// SetColorEnabled は色の有効/無効を設定する
func SetColorEnabled(enabled bool) {
	colorEnabled = enabled
}

// IsColorEnabled は色が有効かどうかを返す
func IsColorEnabled() bool {
	return colorEnabled
}

const (
	reset  = "\033[0m"
	bold   = "\033[1m"
	red    = "\033[31m"
	green  = "\033[32m"
	yellow = "\033[33m"
	blue   = "\033[34m"
	cyan   = "\033[36m"
	gray   = "\033[90m"
)

// Bold は太字にする
func Bold(s string) string {
	if !colorEnabled {
		return s
	}
	return bold + s + reset
}

// Red は赤色にする
func Red(s string) string {
	if !colorEnabled {
		return s
	}
	return red + s + reset
}

// Green は緑色にする
func Green(s string) string {
	if !colorEnabled {
		return s
	}
	return green + s + reset
}

// Yellow は黄色にする
func Yellow(s string) string {
	if !colorEnabled {
		return s
	}
	return yellow + s + reset
}

// Blue は青色にする
func Blue(s string) string {
	if !colorEnabled {
		return s
	}
	return blue + s + reset
}

// Cyan はシアン色にする
func Cyan(s string) string {
	if !colorEnabled {
		return s
	}
	return cyan + s + reset
}

// Gray はグレーにする
func Gray(s string) string {
	if !colorEnabled {
		return s
	}
	return gray + s + reset
}

// StatusColor はステータスに応じた色を返す
func StatusColor(status string) string {
	switch status {
	case "完了", "Closed", "Done":
		return Green(status)
	case "処理中", "In Progress":
		return Blue(status)
	case "未対応", "Open":
		return Yellow(status)
	default:
		return status
	}
}

// PriorityColor は優先度に応じた色を返す
func PriorityColor(priority string) string {
	switch priority {
	case "高", "High":
		return Red(priority)
	case "中", "Normal":
		return Yellow(priority)
	case "低", "Low":
		return Gray(priority)
	default:
		return priority
	}
}

// Success は成功メッセージを出力する
func Success(format string, args ...interface{}) {
	fmt.Printf(Green("✓ ")+format+"\n", args...)
}

// Error はエラーメッセージを出力する
func Error(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, Red("✗ ")+format+"\n", args...)
}

// Warning は警告メッセージを出力する
func Warning(format string, args ...interface{}) {
	fmt.Printf(Yellow("! ")+format+"\n", args...)
}
```

## 3. Issue List コマンド

### internal/cmd/issue/list.go

```go
package issue

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"

	"github.com/spf13/cobra"
	"github.com/yourorg/backlog-cli/internal/api"
	"github.com/yourorg/backlog-cli/internal/cmd"
	"github.com/yourorg/backlog-cli/internal/ui"
)

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List issues",
	Long: `List issues in a project.

By default, shows open issues assigned to you.

Examples:
  backlog issue list
  backlog issue list --all
  backlog issue list --assignee @me --status 1,2
  backlog issue list --keyword "search term"`,
	RunE: runList,
}

var (
	listAssignee string
	listStatus   string
	listAll      bool
	listLimit    int
	listKeyword  string
	listMine     bool
)

func init() {
	listCmd.Flags().StringVarP(&listAssignee, "assignee", "a", "", "Filter by assignee (user ID or @me)")
	listCmd.Flags().StringVar(&listStatus, "status", "", "Filter by status IDs (comma-separated)")
	listCmd.Flags().BoolVar(&listAll, "all", false, "Show all issues (no assignee filter)")
	listCmd.Flags().IntVarP(&listLimit, "limit", "l", 20, "Maximum number of issues to show")
	listCmd.Flags().StringVarP(&listKeyword, "keyword", "k", "", "Search keyword")
	listCmd.Flags().BoolVar(&listMine, "mine", false, "Show only my issues")
}

func runList(c *cobra.Command, args []string) error {
	client, resolved, err := cmd.GetAPIClient(c)
	if err != nil {
		return err
	}
	
	if err := cmd.RequireProject(resolved); err != nil {
		return err
	}
	
	// プロジェクト情報取得
	project, err := client.GetProject(resolved.Project)
	if err != nil {
		return fmt.Errorf("failed to get project: %w", err)
	}
	
	// オプション構築
	opts := &api.IssueListOptions{
		ProjectIDs: []int{project.ID},
		Count:      listLimit,
		Sort:       "updated",
		Order:      "desc",
	}
	
	if listKeyword != "" {
		opts.Keyword = listKeyword
	}
	
	// 担当者フィルター
	if !listAll {
		if listMine || listAssignee == "@me" || listAssignee == "" {
			// 自分の課題
			me, err := client.GetCurrentUser()
			if err != nil {
				return fmt.Errorf("failed to get current user: %w", err)
			}
			opts.AssigneeIDs = []int{me.ID}
		} else if listAssignee != "" {
			// 指定ユーザー
			assigneeID, err := strconv.Atoi(listAssignee)
			if err != nil {
				return fmt.Errorf("invalid assignee ID: %s", listAssignee)
			}
			opts.AssigneeIDs = []int{assigneeID}
		}
	}
	
	// ステータスフィルター
	if listStatus != "" {
		statusIDs, err := parseIntList(listStatus)
		if err != nil {
			return fmt.Errorf("invalid status IDs: %w", err)
		}
		opts.StatusIDs = statusIDs
	}
	
	// 課題取得
	issues, err := client.GetIssues(opts)
	if err != nil {
		return fmt.Errorf("failed to get issues: %w", err)
	}
	
	if len(issues) == 0 {
		fmt.Println("No issues found")
		return nil
	}
	
	// 出力
	switch resolved.Output {
	case "json":
		return outputJSON(issues)
	default:
		return outputTable(issues)
	}
}

func outputTable(issues []api.Issue) {
	table := ui.NewTable("KEY", "STATUS", "PRIORITY", "ASSIGNEE", "SUMMARY")
	
	for _, issue := range issues {
		assignee := "-"
		if issue.Assignee != nil {
			assignee = issue.Assignee.Name
		}
		
		table.AddRow(
			issue.IssueKey,
			ui.StatusColor(issue.Status.Name),
			ui.PriorityColor(issue.Priority.Name),
			assignee,
			truncate(issue.Summary, 50),
		)
	}
	
	table.RenderWithColor(os.Stdout, ui.IsColorEnabled())
	return nil
}

func outputJSON(v interface{}) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-3] + "..."
}

func parseIntList(s string) ([]int, error) {
	var result []int
	for _, part := range strings.Split(s, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		n, err := strconv.Atoi(part)
		if err != nil {
			return nil, err
		}
		result = append(result, n)
	}
	return result, nil
}
```

## 4. Issue View コマンド

### internal/cmd/issue/view.go

```go
package issue

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/yourorg/backlog-cli/internal/cmd"
	"github.com/yourorg/backlog-cli/internal/ui"
)

var viewCmd = &cobra.Command{
	Use:   "view <issue-key>",
	Short: "View an issue",
	Long: `View detailed information about an issue.

Examples:
  backlog issue view PROJ-123
  backlog issue view PROJ-123 --comments`,
	Args: cobra.ExactArgs(1),
	RunE: runView,
}

var (
	viewComments bool
	viewWeb      bool
)

func init() {
	viewCmd.Flags().BoolVarP(&viewComments, "comments", "c", false, "Show comments")
	viewCmd.Flags().BoolVarP(&viewWeb, "web", "w", false, "Open in browser")
}

func runView(c *cobra.Command, args []string) error {
	issueKey := args[0]
	
	client, resolved, err := cmd.GetAPIClient(c)
	if err != nil {
		return err
	}
	
	// ブラウザで開く
	if viewWeb {
		url := fmt.Sprintf("https://%s.%s/view/%s", resolved.Space, resolved.Domain, issueKey)
		return browser.OpenURL(url)
	}
	
	// 課題取得
	issue, err := client.GetIssue(issueKey)
	if err != nil {
		return fmt.Errorf("failed to get issue: %w", err)
	}
	
	// 出力
	switch resolved.Output {
	case "json":
		return outputJSON(issue)
	default:
		return renderIssueDetail(issue, resolved)
	}
}

func renderIssueDetail(issue *api.Issue, resolved *config.ResolvedConfig) error {
	// ヘッダー
	fmt.Printf("%s %s\n", ui.Bold(issue.IssueKey), issue.Summary)
	fmt.Println(strings.Repeat("─", 60))
	
	// メタ情報
	fmt.Printf("Status:     %s\n", ui.StatusColor(issue.Status.Name))
	fmt.Printf("Type:       %s\n", issue.IssueType.Name)
	fmt.Printf("Priority:   %s\n", ui.PriorityColor(issue.Priority.Name))
	
	if issue.Assignee != nil {
		fmt.Printf("Assignee:   %s\n", issue.Assignee.Name)
	} else {
		fmt.Printf("Assignee:   %s\n", ui.Gray("(unassigned)"))
	}
	
	fmt.Printf("Created:    %s by %s\n", formatDate(issue.Created), issue.CreatedUser.Name)
	if issue.UpdatedUser != nil {
		fmt.Printf("Updated:    %s by %s\n", formatDate(issue.Updated), issue.UpdatedUser.Name)
	}
	
	if issue.DueDate != "" {
		fmt.Printf("Due Date:   %s\n", issue.DueDate)
	}
	
	// カテゴリー
	if len(issue.Category) > 0 {
		cats := make([]string, len(issue.Category))
		for i, c := range issue.Category {
			cats[i] = c.Name
		}
		fmt.Printf("Categories: %s\n", strings.Join(cats, ", "))
	}
	
	// マイルストーン
	if len(issue.Milestone) > 0 {
		milestones := make([]string, len(issue.Milestone))
		for i, m := range issue.Milestone {
			milestones[i] = m.Name
		}
		fmt.Printf("Milestone:  %s\n", strings.Join(milestones, ", "))
	}
	
	// 説明
	if issue.Description != "" {
		fmt.Println()
		fmt.Println(ui.Bold("Description"))
		fmt.Println(strings.Repeat("─", 60))
		fmt.Println(issue.Description)
	}
	
	// URL
	fmt.Println()
	url := fmt.Sprintf("https://%s.%s/view/%s", resolved.Space, resolved.Domain, issue.IssueKey)
	fmt.Printf("URL: %s\n", ui.Cyan(url))
	
	// コメント
	if viewComments {
		comments, err := client.GetComments(issue.IssueKey, &api.CommentListOptions{
			Count: 10,
			Order: "desc",
		})
		if err == nil && len(comments) > 0 {
			fmt.Println()
			fmt.Println(ui.Bold("Recent Comments"))
			fmt.Println(strings.Repeat("─", 60))
			
			for _, comment := range comments {
				fmt.Printf("\n%s %s\n", ui.Bold(comment.CreatedUser.Name), ui.Gray(formatDate(comment.Created)))
				fmt.Println(comment.Content)
			}
		}
	}
	
	return nil
}

func formatDate(dateStr string) string {
	// "2021-01-02T15:04:05Z" 形式を "2021-01-02 15:04" に変換
	if len(dateStr) >= 16 {
		return dateStr[:10] + " " + dateStr[11:16]
	}
	return dateStr
}
```

## 5. Issue Create コマンド

### internal/cmd/issue/create.go

```go
package issue

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/spf13/cobra"
	"github.com/yourorg/backlog-cli/internal/api"
	"github.com/yourorg/backlog-cli/internal/cmd"
	"github.com/yourorg/backlog-cli/internal/ui"
)

var createCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a new issue",
	Long: `Create a new issue in the project.

If summary is not provided, an interactive prompt will be shown.

Examples:
  backlog issue create
  backlog issue create --summary "Bug fix" --type 1
  backlog issue create -s "Feature request" -a @me`,
	RunE: runCreate,
}

var (
	createSummary     string
	createDescription string
	createTypeID      int
	createPriorityID  int
	createAssignee    string
	createDueDate     string
	createEditor      bool
)

func init() {
	createCmd.Flags().StringVarP(&createSummary, "summary", "s", "", "Issue summary")
	createCmd.Flags().StringVarP(&createDescription, "description", "d", "", "Issue description")
	createCmd.Flags().IntVarP(&createTypeID, "type", "t", 0, "Issue type ID")
	createCmd.Flags().IntVar(&createPriorityID, "priority", 0, "Priority ID")
	createCmd.Flags().StringVarP(&createAssignee, "assignee", "a", "", "Assignee (user ID or @me)")
	createCmd.Flags().StringVar(&createDueDate, "due", "", "Due date (YYYY-MM-DD)")
	createCmd.Flags().BoolVarP(&createEditor, "editor", "e", false, "Open editor for description")
}

func runCreate(c *cobra.Command, args []string) error {
	client, resolved, err := cmd.GetAPIClient(c)
	if err != nil {
		return err
	}
	
	if err := cmd.RequireProject(resolved); err != nil {
		return err
	}
	
	// プロジェクト情報取得
	project, err := client.GetProject(resolved.Project)
	if err != nil {
		return fmt.Errorf("failed to get project: %w", err)
	}
	
	// 課題種別を取得
	issueTypes, err := client.GetIssueTypes(resolved.Project)
	if err != nil {
		return fmt.Errorf("failed to get issue types: %w", err)
	}
	
	// 入力
	input := &api.CreateIssueInput{
		ProjectID: project.ID,
	}
	
	// 件名
	if createSummary != "" {
		input.Summary = createSummary
	} else {
		input.Summary, err = ui.Input("Summary:", "")
		if err != nil {
			return err
		}
		if input.Summary == "" {
			return fmt.Errorf("summary is required")
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
	if createPriorityID > 0 {
		input.PriorityID = createPriorityID
	} else {
		// デフォルトは「中」(ID=3)
		input.PriorityID = 3
		
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
		me, err := client.GetCurrentUser()
		if err != nil {
			return fmt.Errorf("failed to get current user: %w", err)
		}
		input.AssigneeID = me.ID
	} else if createAssignee != "" {
		assigneeID, err := strconv.Atoi(createAssignee)
		if err != nil {
			return fmt.Errorf("invalid assignee ID: %s", createAssignee)
		}
		input.AssigneeID = assigneeID
	} else {
		// 担当者選択（オプション）
		users, err := client.GetProjectUsers(resolved.Project)
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
	
	// 説明
	if createDescription != "" {
		input.Description = createDescription
	} else if createEditor {
		// エディタで編集
		content, err := openEditor("")
		if err != nil {
			return fmt.Errorf("failed to open editor: %w", err)
		}
		input.Description = content
	} else {
		// 説明入力（オプション）
		input.Description, _ = ui.Input("Description (optional):", "")
	}
	
	// 期限
	if createDueDate != "" {
		input.DueDate = createDueDate
	}
	
	// 作成
	fmt.Println("Creating issue...")
	issue, err := client.CreateIssue(input)
	if err != nil {
		return fmt.Errorf("failed to create issue: %w", err)
	}
	
	ui.Success("Created issue %s", issue.IssueKey)
	
	url := fmt.Sprintf("https://%s.%s/view/%s", resolved.Space, resolved.Domain, issue.IssueKey)
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
	defer os.Remove(tmpfile.Name())
	
	if initial != "" {
		tmpfile.WriteString(initial)
	}
	tmpfile.Close()
	
	// エディタ起動
	cmd := exec.Command(editor, tmpfile.Name())
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	
	if err := cmd.Run(); err != nil {
		return "", err
	}
	
	// 内容読み込み
	content, err := os.ReadFile(tmpfile.Name())
	if err != nil {
		return "", err
	}
	
	return strings.TrimSpace(string(content)), nil
}
```

## 6. Issue コマンド登録

### internal/cmd/issue/issue.go

```go
package issue

import (
	"github.com/spf13/cobra"
)

var IssueCmd = &cobra.Command{
	Use:   "issue",
	Short: "Manage Backlog issues",
	Long:  "Work with Backlog issues.",
}

func init() {
	IssueCmd.AddCommand(listCmd)
	IssueCmd.AddCommand(viewCmd)
	IssueCmd.AddCommand(createCmd)
}
```

## 完了条件

- [ ] `backlog issue list` で課題一覧が表示される
- [ ] `-a @me` で自分の課題のみ表示される
- [ ] `--all` で全課題が表示される
- [ ] `--keyword` で検索できる
- [ ] `-o json` でJSON出力できる
- [ ] `backlog issue view PROJ-123` で課題詳細が表示される
- [ ] `--comments` でコメントも表示される
- [ ] `--web` でブラウザで開ける
- [ ] `backlog issue create` で課題が作成できる
- [ ] 対話的に入力できる
- [ ] `--editor` でエディタが開ける

## 次のステップ

`08-project-commands.md` に進んでプロジェクトコマンドを実装してください。
