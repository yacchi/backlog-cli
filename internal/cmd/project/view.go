package project

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/pkg/browser"
	"github.com/spf13/cobra"
	"github.com/yacchi/backlog-cli/internal/api"
	"github.com/yacchi/backlog-cli/internal/cmdutil"
	"github.com/yacchi/backlog-cli/internal/config"
	"github.com/yacchi/backlog-cli/internal/ui"
)

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
	project, err := client.GetProject(projectKey)
	if err != nil {
		return fmt.Errorf("failed to get project: %w", err)
	}

	// 出力
	switch profile.Output {
	case "json":
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(project)
	default:
		return renderProjectDetail(client, project, profile)
	}
}

func renderProjectDetail(client *api.Client, project *api.Project, profile *config.ResolvedProfile) error {
	// ヘッダー
	fmt.Printf("%s %s\n", ui.Bold(project.ProjectKey), project.Name)
	fmt.Println(strings.Repeat("─", 60))

	// ステータス
	if project.Archived {
		fmt.Printf("Status: %s\n", ui.Gray("Archived"))
	} else {
		fmt.Printf("Status: %s\n", ui.Green("Active"))
	}

	// 機能
	features := []string{}
	if project.UseWiki {
		features = append(features, "Wiki")
	}
	if project.UseFileSharing {
		features = append(features, "File Sharing")
	}
	if project.SubtaskingEnabled {
		features = append(features, "Subtasking")
	}
	if project.ChartEnabled {
		features = append(features, "Chart")
	}

	if len(features) > 0 {
		fmt.Printf("Features: %s\n", strings.Join(features, ", "))
	}

	fmt.Printf("Text Format: %s\n", project.TextFormattingRule)

	// 統計情報
	fmt.Println()
	fmt.Println(ui.Bold("Statistics"))
	fmt.Println(strings.Repeat("─", 60))

	// 課題種別
	issueTypes, err := client.GetIssueTypes(project.ProjectKey)
	if err == nil && len(issueTypes) > 0 {
		types := make([]string, len(issueTypes))
		for i, t := range issueTypes {
			types[i] = t.Name
		}
		fmt.Printf("Issue Types: %s\n", strings.Join(types, ", "))
	}

	// カテゴリー
	categories, err := client.GetCategories(project.ProjectKey)
	if err == nil && len(categories) > 0 {
		cats := make([]string, len(categories))
		for i, c := range categories {
			cats[i] = c.Name
		}
		fmt.Printf("Categories: %s\n", strings.Join(cats, ", "))
	}

	// バージョン
	versions, err := client.GetVersions(project.ProjectKey)
	if err == nil {
		activeVersions := 0
		for _, v := range versions {
			if !v.Archived {
				activeVersions++
			}
		}
		fmt.Printf("Versions: %d active, %d total\n", activeVersions, len(versions))
	}

	// メンバー
	users, err := client.GetProjectUsers(project.ProjectKey)
	if err == nil {
		fmt.Printf("Members: %d\n", len(users))
	}

	// URL
	fmt.Println()
	url := fmt.Sprintf("https://%s.%s/projects/%s", profile.Space, profile.Domain, project.ProjectKey)
	fmt.Printf("URL: %s\n", ui.Cyan(url))

	return nil
}
