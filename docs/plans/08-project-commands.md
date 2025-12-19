# Phase 08: プロジェクトコマンド

## 目標

- `backlog project list` - プロジェクト一覧表示
- `backlog project view` - プロジェクト詳細表示
- `backlog project init` - .backlog.yaml 作成

## 1. Project List コマンド

### internal/cmd/project/list.go

```go
package project

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/yourorg/backlog-cli/internal/cmd"
	"github.com/yourorg/backlog-cli/internal/ui"
)

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List projects",
	Long: `List all accessible projects.

Examples:
  backlog project list
  backlog project list -o json`,
	RunE: runList,
}

var listArchived bool

func init() {
	listCmd.Flags().BoolVar(&listArchived, "archived", false, "Include archived projects")
}

func runList(c *cobra.Command, args []string) error {
	client, resolved, err := cmd.GetAPIClient(c)
	if err != nil {
		return err
	}
	
	projects, err := client.GetProjects()
	if err != nil {
		return fmt.Errorf("failed to get projects: %w", err)
	}
	
	if len(projects) == 0 {
		fmt.Println("No projects found")
		return nil
	}
	
	// アーカイブフィルター
	if !listArchived {
		filtered := make([]api.Project, 0)
		for _, p := range projects {
			if !p.Archived {
				filtered = append(filtered, p)
			}
		}
		projects = filtered
	}
	
	// 出力
	switch resolved.Output {
	case "json":
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(projects)
	default:
		return outputProjectTable(projects, resolved.Project)
	}
}

func outputProjectTable(projects []api.Project, currentProject string) error {
	table := ui.NewTable("KEY", "NAME", "STATUS")
	
	for _, p := range projects {
		key := p.ProjectKey
		if key == currentProject {
			key = ui.Green(key + " ✓")
		}
		
		status := "active"
		if p.Archived {
			status = ui.Gray("archived")
		}
		
		table.AddRow(key, p.Name, status)
	}
	
	table.RenderWithColor(os.Stdout, ui.IsColorEnabled())
	return nil
}
```

## 2. Project View コマンド

### internal/cmd/project/view.go

```go
package project

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/yourorg/backlog-cli/internal/cmd"
	"github.com/yourorg/backlog-cli/internal/ui"
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
	client, resolved, err := cmd.GetAPIClient(c)
	if err != nil {
		return err
	}
	
	projectKey := resolved.Project
	if len(args) > 0 {
		projectKey = args[0]
	}
	
	if projectKey == "" {
		return fmt.Errorf("project key is required")
	}
	
	// ブラウザで開く
	if viewWeb {
		url := fmt.Sprintf("https://%s.%s/projects/%s", resolved.Space, resolved.Domain, projectKey)
		return browser.OpenURL(url)
	}
	
	// プロジェクト情報取得
	project, err := client.GetProject(projectKey)
	if err != nil {
		return fmt.Errorf("failed to get project: %w", err)
	}
	
	// 出力
	switch resolved.Output {
	case "json":
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(project)
	default:
		return renderProjectDetail(client, project, resolved)
	}
}

func renderProjectDetail(client *api.Client, project *api.Project, resolved *config.ResolvedConfig) error {
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
	url := fmt.Sprintf("https://%s.%s/projects/%s", resolved.Space, resolved.Domain, project.ProjectKey)
	fmt.Printf("URL: %s\n", ui.Cyan(url))
	
	return nil
}
```

## 3. Project Init コマンド

### internal/cmd/project/init.go

```go
package project

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/yourorg/backlog-cli/internal/cmd"
	"github.com/yourorg/backlog-cli/internal/config"
	"github.com/yourorg/backlog-cli/internal/ui"
)

var initCmd = &cobra.Command{
	Use:   "init [project-key]",
	Short: "Initialize project configuration",
	Long: `Create a .backlog.yaml file in the current directory.

This file can be committed to your repository to share project settings
with your team.

Examples:
  backlog project init
  backlog project init PROJ
  backlog project init --space other-space --domain backlog.com`,
	RunE: runInit,
}

var (
	initSpace  string
	initDomain string
	initForce  bool
)

func init() {
	initCmd.Flags().StringVar(&initSpace, "space", "", "Backlog space (optional)")
	initCmd.Flags().StringVar(&initDomain, "domain", "", "Backlog domain (optional)")
	initCmd.Flags().BoolVarP(&initForce, "force", "f", false, "Overwrite existing file")
}

func runInit(c *cobra.Command, args []string) error {
	// 既存ファイルチェック
	configPath := ".backlog.yaml"
	if _, err := os.Stat(configPath); err == nil && !initForce {
		return fmt.Errorf(".backlog.yaml already exists\nUse --force to overwrite")
	}
	
	var projectKey string
	
	if len(args) > 0 {
		projectKey = args[0]
	} else {
		// プロジェクト選択
		client, resolved, err := cmd.GetAPIClient(c)
		if err != nil {
			// 認証されていない場合は手動入力
			projectKey, err = ui.Input("Project key:", "")
			if err != nil {
				return err
			}
		} else {
			// プロジェクト一覧から選択
			projects, err := client.GetProjects()
			if err != nil {
				return fmt.Errorf("failed to get projects: %w", err)
			}
			
			if len(projects) == 0 {
				return fmt.Errorf("no projects found")
			}
			
			opts := make([]ui.SelectOption, len(projects))
			for i, p := range projects {
				opts[i] = ui.SelectOption{
					Value:       p.ProjectKey,
					Description: p.Name,
				}
			}
			
			projectKey, err = ui.SelectWithDesc("Select project:", opts)
			if err != nil {
				return err
			}
			
			// デフォルト値を設定
			if initSpace == "" && resolved.Space != "" {
				initSpace = resolved.Space
			}
			if initDomain == "" && resolved.Domain != "" {
				initDomain = resolved.Domain
			}
		}
	}
	
	if projectKey == "" {
		return fmt.Errorf("project key is required")
	}
	
	// 設定ファイル作成
	cfg := config.ProjectConfig{
		Project: projectKey,
	}
	
	// スペースとドメインはデフォルトと異なる場合のみ記載
	globalCfg, _ := config.Load()
	if initSpace != "" && (globalCfg == nil || initSpace != globalCfg.Client.Default.Space) {
		cfg.Space = initSpace
	}
	if initDomain != "" && (globalCfg == nil || initDomain != globalCfg.Client.Default.Domain) {
		cfg.Domain = initDomain
	}
	
	// 書き込み
	if err := config.SaveProjectConfig(&cfg, configPath); err != nil {
		return fmt.Errorf("failed to write config: %w", err)
	}
	
	ui.Success("Created %s", configPath)
	
	// 内容表示
	fmt.Println()
	fmt.Printf("project: %s\n", cfg.Project)
	if cfg.Space != "" {
		fmt.Printf("space: %s\n", cfg.Space)
	}
	if cfg.Domain != "" {
		fmt.Printf("domain: %s\n", cfg.Domain)
	}
	
	// .gitignore チェック
	gitignorePath := ".gitignore"
	if _, err := os.Stat(gitignorePath); err == nil {
		fmt.Println()
		fmt.Println(ui.Gray("Note: .backlog.yaml can be committed to share settings with your team"))
	}
	
	return nil
}
```

### internal/config/project.go (追加)

```go
// SaveProjectConfig はプロジェクト設定を保存する
func SaveProjectConfig(cfg *ProjectConfig, path string) error {
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}
	
	return os.WriteFile(path, data, 0644)
}
```

## 4. Project コマンド登録

### internal/cmd/project/project.go

```go
package project

import (
	"github.com/spf13/cobra"
)

var ProjectCmd = &cobra.Command{
	Use:   "project",
	Short: "Manage Backlog projects",
	Long:  "Work with Backlog projects.",
}

func init() {
	ProjectCmd.AddCommand(listCmd)
	ProjectCmd.AddCommand(viewCmd)
	ProjectCmd.AddCommand(initCmd)
}
```

## 5. Config コマンド

### internal/cmd/config/config.go

```go
package config

import (
	"github.com/spf13/cobra"
)

var ConfigCmd = &cobra.Command{
	Use:   "config",
	Short: "Manage configuration",
	Long:  "Get and set configuration options.",
}

func init() {
	ConfigCmd.AddCommand(getCmd)
	ConfigCmd.AddCommand(setCmd)
	ConfigCmd.AddCommand(listCmd)
	ConfigCmd.AddCommand(pathCmd)
}
```

### internal/cmd/config/get.go

```go
package config

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/yourorg/backlog-cli/internal/config"
)

var getCmd = &cobra.Command{
	Use:   "get <key>",
	Short: "Get a configuration value",
	Long: `Get a configuration value.

Examples:
  backlog config get client.default.space
  backlog config get client.default.project`,
	Args: cobra.ExactArgs(1),
	RunE: runGet,
}

func runGet(c *cobra.Command, args []string) error {
	key := args[0]
	
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}
	
	value, err := getConfigValue(cfg, key)
	if err != nil {
		return err
	}
	
	fmt.Println(value)
	return nil
}

func getConfigValue(cfg *config.Config, key string) (string, error) {
	parts := strings.Split(key, ".")
	
	switch {
	case len(parts) >= 3 && parts[0] == "client" && parts[1] == "default":
		switch parts[2] {
		case "relay_server":
			return cfg.Client.Default.RelayServer, nil
		case "space":
			return cfg.Client.Default.Space, nil
		case "domain":
			return cfg.Client.Default.Domain, nil
		case "project":
			return cfg.Client.Default.Project, nil
		case "output":
			return cfg.Client.Default.Output, nil
		case "color":
			return cfg.Client.Default.Color, nil
		}
	case len(parts) >= 3 && parts[0] == "client" && parts[1] == "auth":
		switch parts[2] {
		case "callback_port":
			return fmt.Sprintf("%d", cfg.Client.Auth.CallbackPort), nil
		case "timeout":
			return fmt.Sprintf("%d", cfg.Client.Auth.Timeout), nil
		case "no_browser":
			return fmt.Sprintf("%v", cfg.Client.Auth.NoBrowser), nil
		}
	}
	
	return "", fmt.Errorf("unknown config key: %s", key)
}
```

### internal/cmd/config/set.go

```go
package config

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	"github.com/yourorg/backlog-cli/internal/config"
	"github.com/yourorg/backlog-cli/internal/ui"
)

var setCmd = &cobra.Command{
	Use:   "set <key> <value>",
	Short: "Set a configuration value",
	Long: `Set a configuration value.

Examples:
  backlog config set client.default.space mycompany
  backlog config set client.default.project PROJ`,
	Args: cobra.ExactArgs(2),
	RunE: runSet,
}

func runSet(c *cobra.Command, args []string) error {
	key := args[0]
	value := args[1]
	
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}
	
	if err := setConfigValue(cfg, key, value); err != nil {
		return err
	}
	
	if err := config.Save(cfg); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}
	
	ui.Success("Set %s = %s", key, value)
	return nil
}

func setConfigValue(cfg *config.Config, key, value string) error {
	parts := strings.Split(key, ".")
	
	switch {
	case len(parts) >= 3 && parts[0] == "client" && parts[1] == "default":
		switch parts[2] {
		case "relay_server":
			cfg.Client.Default.RelayServer = value
		case "space":
			cfg.Client.Default.Space = value
		case "domain":
			cfg.Client.Default.Domain = value
		case "project":
			cfg.Client.Default.Project = value
		case "output":
			cfg.Client.Default.Output = value
		case "color":
			cfg.Client.Default.Color = value
		default:
			return fmt.Errorf("unknown config key: %s", key)
		}
	case len(parts) >= 3 && parts[0] == "client" && parts[1] == "auth":
		switch parts[2] {
		case "callback_port":
			port, err := strconv.Atoi(value)
			if err != nil {
				return fmt.Errorf("invalid port: %s", value)
			}
			cfg.Client.Auth.CallbackPort = port
		case "timeout":
			timeout, err := strconv.Atoi(value)
			if err != nil {
				return fmt.Errorf("invalid timeout: %s", value)
			}
			cfg.Client.Auth.Timeout = timeout
		case "no_browser":
			cfg.Client.Auth.NoBrowser = value == "true" || value == "1"
		default:
			return fmt.Errorf("unknown config key: %s", key)
		}
	default:
		return fmt.Errorf("unknown config key: %s", key)
	}
	
	return nil
}
```

### internal/cmd/config/list.go

```go
package config

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/yourorg/backlog-cli/internal/config"
)

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List all configuration values",
	RunE:  runList,
}

func runList(c *cobra.Command, args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}
	
	fmt.Println("# Client settings")
	fmt.Printf("client.default.relay_server=%s\n", cfg.Client.Default.RelayServer)
	fmt.Printf("client.default.space=%s\n", cfg.Client.Default.Space)
	fmt.Printf("client.default.domain=%s\n", cfg.Client.Default.Domain)
	fmt.Printf("client.default.project=%s\n", cfg.Client.Default.Project)
	fmt.Printf("client.default.output=%s\n", cfg.Client.Default.Output)
	fmt.Printf("client.default.color=%s\n", cfg.Client.Default.Color)
	fmt.Println()
	fmt.Println("# Auth settings")
	fmt.Printf("client.auth.callback_port=%d\n", cfg.Client.Auth.CallbackPort)
	fmt.Printf("client.auth.timeout=%d\n", cfg.Client.Auth.Timeout)
	fmt.Printf("client.auth.no_browser=%v\n", cfg.Client.Auth.NoBrowser)
	
	return nil
}
```

### internal/cmd/config/path.go

```go
package config

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/yourorg/backlog-cli/internal/config"
)

var pathCmd = &cobra.Command{
	Use:   "path",
	Short: "Show configuration file path",
	RunE:  runPath,
}

func runPath(c *cobra.Command, args []string) error {
	path, err := config.ConfigPath()
	if err != nil {
		return err
	}
	
	fmt.Println(path)
	
	// プロジェクトローカル設定も表示
	projectCfg, projectPath, err := config.FindProjectConfig()
	if err == nil && projectCfg != nil {
		fmt.Printf("\nProject config: %s\n", projectPath)
	}
	
	return nil
}
```

## 完了条件

- [ ] `backlog project list` でプロジェクト一覧が表示される
- [ ] 現在のプロジェクトにチェックマークがつく
- [ ] `backlog project view` でプロジェクト詳細が表示される
- [ ] `backlog project init` で .backlog.yaml が作成される
- [ ] 対話的にプロジェクトを選択できる
- [ ] `backlog config get/set/list/path` が動作する

## 次のステップ

`09-additional-commands.md` に進んで追加コマンドを実装してください。
