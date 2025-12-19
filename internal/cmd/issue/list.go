package issue

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	"github.com/yacchi/backlog-cli/internal/api"
	"github.com/yacchi/backlog-cli/internal/backlog"
	"github.com/yacchi/backlog-cli/internal/cmdutil"
	"github.com/yacchi/backlog-cli/internal/config"
	"github.com/yacchi/backlog-cli/internal/ui"
)

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List issues",
	Long: `List issues in a project.

Examples:
  backlog issue list
  backlog issue list --mine
  backlog issue list --assignee @me --status 1,2
  backlog issue list --keyword "search term"`,
	RunE: runList,
}

var (
	listAssignee string
	listStatus   string
	listLimit    int
	listKeyword  string
	listMine     bool
)

func init() {
	listCmd.Flags().StringVarP(&listAssignee, "assignee", "a", "", "Filter by assignee (user ID or @me)")
	listCmd.Flags().StringVar(&listStatus, "status", "", "Filter by status IDs (comma-separated)")
	listCmd.Flags().IntVarP(&listLimit, "limit", "l", 20, "Maximum number of issues to show")
	listCmd.Flags().StringVarP(&listKeyword, "keyword", "k", "", "Search keyword")
	listCmd.Flags().BoolVar(&listMine, "mine", false, "Show only my issues")
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

	// プロジェクト情報取得
	project, err := client.GetProject(projectKey)
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
	if listMine || listAssignee == "@me" {
		// 自分の課題
		me, err := client.GetCurrentUser()
		if err != nil {
			return fmt.Errorf("failed to get current user: %w", err)
		}
		opts.AssigneeIDs = []int{me.ID.Value}
	} else if listAssignee != "" {
		// 指定ユーザー
		assigneeID, err := strconv.Atoi(listAssignee)
		if err != nil {
			return fmt.Errorf("invalid assignee ID: %s", listAssignee)
		}
		opts.AssigneeIDs = []int{assigneeID}
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
	profile := cfg.CurrentProfile()
	display := cfg.Display()
	switch profile.Output {
	case "json":
		return outputJSON(issues)
	default:
		outputTable(issues, profile, display)
		return nil
	}
}

func outputTable(issues []backlog.Issue, profile *config.ResolvedProfile, display *config.ResolvedDisplay) {
	fields := display.IssueListFields
	fieldConfig := display.IssueFieldConfig

	// ハイパーリンク設定
	ui.SetHyperlinkEnabled(display.Hyperlink)

	// ヘッダー生成
	headers := make([]string, len(fields))
	for i, f := range fields {
		if cfg, ok := fieldConfig[f]; ok && cfg.Header != "" {
			headers[i] = cfg.Header
		} else {
			headers[i] = strings.ToUpper(f)
		}
	}

	table := ui.NewTable(headers...)

	// フィールドフォーマッターを作成
	formatter := ui.NewFieldFormatter(display.Timezone, display.DateTimeFormat, fieldConfig)

	// ベースURL生成
	baseURL := fmt.Sprintf("https://%s.%s", profile.Space, profile.Domain)

	for _, issue := range issues {
		row := make([]string, len(fields))
		for i, f := range fields {
			row[i] = getIssueFieldValue(issue, f, formatter, baseURL)
		}
		table.AddRow(row...)
	}

	table.RenderWithColor(os.Stdout, ui.IsColorEnabled())
}

func getIssueFieldValue(issue backlog.Issue, field string, f *ui.FieldFormatter, baseURL string) string {
	switch field {
	case "key":
		key := issue.IssueKey.Value
		url := fmt.Sprintf("%s/view/%s", baseURL, key)
		return ui.Hyperlink(url, key)
	case "status":
		if issue.Status.IsSet() && issue.Status.Value.Name.IsSet() {
			return ui.StatusColor(issue.Status.Value.Name.Value)
		}
		return "-"
	case "priority":
		if issue.Priority.IsSet() && issue.Priority.Value.Name.IsSet() {
			return ui.PriorityColor(issue.Priority.Value.Name.Value)
		}
		return "-"
	case "assignee":
		if issue.Assignee.IsSet() && issue.Assignee.Value.Name.IsSet() {
			return f.FormatString(issue.Assignee.Value.Name.Value, field)
		}
		return "-"
	case "summary":
		return f.FormatString(issue.Summary.Value, field)
	case "type":
		if issue.IssueType.IsSet() && issue.IssueType.Value.Name.IsSet() {
			return issue.IssueType.Value.Name.Value
		}
		return "-"
	case "created":
		return f.FormatDateTime(issue.Created.Value, field)
	case "updated":
		return f.FormatDateTime(issue.Updated.Value, field)
	case "created_user":
		if issue.CreatedUser.IsSet() && issue.CreatedUser.Value.Name.IsSet() {
			return f.FormatString(issue.CreatedUser.Value.Name.Value, field)
		}
		return "-"
	case "due_date":
		if issue.DueDate.IsSet() && !issue.DueDate.IsNull() {
			return f.FormatDate(issue.DueDate.Value, field)
		}
		return "-"
	case "start_date":
		if issue.StartDate.IsSet() && !issue.StartDate.IsNull() {
			return f.FormatDate(issue.StartDate.Value, field)
		}
		return "-"
	case "category":
		if len(issue.Category) > 0 {
			names := make([]string, len(issue.Category))
			for i, c := range issue.Category {
				if c.Name.IsSet() {
					names[i] = c.Name.Value
				}
			}
			return f.FormatString(strings.Join(names, ", "), field)
		}
		return "-"
	case "milestone":
		if len(issue.Milestone) > 0 {
			names := make([]string, len(issue.Milestone))
			for i, m := range issue.Milestone {
				if m.Name.IsSet() {
					names[i] = m.Name.Value
				}
			}
			return f.FormatString(strings.Join(names, ", "), field)
		}
		return "-"
	case "version":
		if len(issue.Versions) > 0 {
			names := make([]string, len(issue.Versions))
			for i, v := range issue.Versions {
				if v.Name.IsSet() {
					names[i] = v.Name.Value
				}
			}
			return f.FormatString(strings.Join(names, ", "), field)
		}
		return "-"
	case "url":
		return fmt.Sprintf("%s/view/%s", baseURL, issue.IssueKey.Value)
	default:
		return "-"
	}
}

func outputJSON(v interface{}) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
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
