package project

import (
	"fmt"
	"strings"

	"github.com/pkg/browser"
	"github.com/spf13/cobra"
	"github.com/yacchi/backlog-cli/packages/backlog/internal/api"
	"github.com/yacchi/backlog-cli/packages/backlog/internal/cmdutil"
	"github.com/yacchi/backlog-cli/packages/backlog/internal/config"
	"github.com/yacchi/backlog-cli/packages/backlog/internal/ui"
)

// ProjectDetail はプロジェクト情報にメタデータを付加した構造体
type ProjectDetail struct {
	api.Project
	Statuses     []api.Status    `json:"statuses,omitempty"`
	IssueTypes   []api.IssueType `json:"issueTypes,omitempty"`
	Categories   []api.Category  `json:"categories,omitempty"`
	MemberCount  int             `json:"memberCount,omitempty"`
	VersionCount int             `json:"versionCount,omitempty"`
}

var viewCmd = &cobra.Command{
	Use:   "view [project-key]",
	Short: "View project details",
	Long: `View detailed information about a project.

If no project key is provided, uses the default project.

Examples:
  backlog project view
  backlog project view PROJ
  backlog project view PROJ --web`,
	RunE: runView,
}

var viewWeb bool

func init() {
	viewCmd.Flags().BoolVarP(&viewWeb, "web", "w", false, "Open in browser")
}

func runView(c *cobra.Command, args []string) error {
	client, cfg, err := cmdutil.GetAPIClient(c)
	if err != nil {
		return err
	}

	profile := cfg.CurrentProfile()
	projectKey := profile.Project
	if len(args) > 0 {
		projectKey = args[0]
	}

	if projectKey == "" {
		return fmt.Errorf("project key is required")
	}

	// ブラウザで開く
	if viewWeb {
		url := fmt.Sprintf("https://%s.%s/projects/%s", profile.Space, profile.Domain, projectKey)
		return browser.OpenURL(url)
	}

	// プロジェクト情報取得
	ctx := c.Context()
	project, err := client.GetProject(ctx, projectKey)
	if err != nil {
		return fmt.Errorf("failed to get project: %w", err)
	}

	// プロジェクトメタデータを取得
	detail := &ProjectDetail{Project: *project}
	if statuses, err := client.GetStatuses(ctx, projectKey); err == nil {
		detail.Statuses = statuses
	}
	if issueTypes, err := client.GetIssueTypes(ctx, projectKey); err == nil {
		detail.IssueTypes = issueTypes
	}
	if categories, err := client.GetCategories(ctx, projectKey); err == nil {
		detail.Categories = categories
	}
	if versions, err := client.GetVersions(ctx, projectKey); err == nil {
		detail.VersionCount = len(versions)
	}
	if users, err := client.GetProjectUsers(ctx, projectKey); err == nil {
		detail.MemberCount = len(users)
	}

	// 出力
	switch profile.Output {
	case "json":
		return cmdutil.OutputJSONFromProfile(detail, profile.JSONFields, profile.JQ, profile.Template)
	default:
		return renderProjectDetail(detail, profile)
	}
}

func renderProjectDetail(detail *ProjectDetail, profile *config.ResolvedProfile) error {
	// ヘッダー
	fmt.Printf("%s %s\n", ui.Bold(detail.ProjectKey), detail.Name)
	fmt.Println(strings.Repeat("─", 60))

	// プロジェクトステータス
	if detail.Archived {
		fmt.Printf("Status: %s\n", ui.Gray("Archived"))
	} else {
		fmt.Printf("Status: %s\n", ui.Green("Active"))
	}

	// 機能
	features := []string{}
	if detail.UseWiki {
		features = append(features, "Wiki")
	}
	if detail.UseFileSharing {
		features = append(features, "File Sharing")
	}
	if detail.SubtaskingEnabled {
		features = append(features, "Subtasking")
	}
	if detail.ChartEnabled {
		features = append(features, "Chart")
	}

	if len(features) > 0 {
		fmt.Printf("Features: %s\n", strings.Join(features, ", "))
	}

	fmt.Printf("Text Format: %s\n", detail.TextFormattingRule)

	// メタデータ
	fmt.Println()
	fmt.Println(ui.Bold("Metadata"))
	fmt.Println(strings.Repeat("─", 60))

	// ステータス一覧
	if len(detail.Statuses) > 0 {
		statuses := make([]string, len(detail.Statuses))
		for i, s := range detail.Statuses {
			statuses[i] = fmt.Sprintf("%s (ID:%d)", s.Name, s.ID)
		}
		fmt.Printf("Statuses: %s\n", strings.Join(statuses, ", "))
	}

	// 課題種別
	if len(detail.IssueTypes) > 0 {
		types := make([]string, len(detail.IssueTypes))
		for i, t := range detail.IssueTypes {
			types[i] = fmt.Sprintf("%s (ID:%d)", t.Name, t.ID)
		}
		fmt.Printf("Issue Types: %s\n", strings.Join(types, ", "))
	}

	// カテゴリー
	if len(detail.Categories) > 0 {
		cats := make([]string, len(detail.Categories))
		for i, c := range detail.Categories {
			cats[i] = c.Name
		}
		fmt.Printf("Categories: %s\n", strings.Join(cats, ", "))
	}

	// 統計
	if detail.VersionCount > 0 || detail.MemberCount > 0 {
		fmt.Println()
		fmt.Println(ui.Bold("Statistics"))
		fmt.Println(strings.Repeat("─", 60))
		if detail.VersionCount > 0 {
			fmt.Printf("Versions: %d\n", detail.VersionCount)
		}
		if detail.MemberCount > 0 {
			fmt.Printf("Members: %d\n", detail.MemberCount)
		}
	}

	// URL
	fmt.Println()
	url := fmt.Sprintf("https://%s.%s/projects/%s", profile.Space, profile.Domain, detail.ProjectKey)
	fmt.Printf("URL: %s\n", ui.Cyan(url))

	return nil
}
