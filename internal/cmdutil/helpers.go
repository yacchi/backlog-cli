package cmdutil

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/yacchi/backlog-cli/internal/api"
	"github.com/yacchi/backlog-cli/internal/config"
)

// GetConfigStore はConfigStoreを取得する
// グローバルフラグはrootCmd.PersistentPreRunEで適用済み
func GetConfigStore(cmd *cobra.Command) (*config.Store, error) {
	cfg, err := config.Load(cmd.Context())
	if err != nil {
		return nil, fmt.Errorf("failed to load config: %w", err)
	}
	return cfg, nil
}

// GetAPIClient は認証済みAPIクライアントを取得する
func GetAPIClient(cmd *cobra.Command) (*api.Client, *config.Store, error) {
	cfg, err := GetConfigStore(cmd)
	if err != nil {
		return nil, nil, err
	}

	space, domain := GetSpaceDomain(cfg)
	if space == "" || domain == "" {
		return nil, nil, fmt.Errorf("space and domain are required\nRun 'backlog auth login' first")
	}

	client, err := api.NewClientFromConfig(cfg)
	if err != nil {
		return nil, nil, fmt.Errorf("authentication required\nRun 'backlog auth login' first")
	}

	return client, cfg, nil
}

// GetSpaceDomain はspace/domainを取得する
// プロジェクト設定（.backlog.yaml）を優先し、なければプロファイル設定を使用
func GetSpaceDomain(cfg *config.Store) (space, domain string) {
	project := cfg.Project()
	profile := cfg.CurrentProfile()

	// プロジェクト設定を優先
	if project != nil && project.Space != "" {
		space = project.Space
	} else if profile != nil {
		space = profile.Space
	}

	if project != nil && project.Domain != "" {
		domain = project.Domain
	} else if profile != nil {
		domain = profile.Domain
	}

	return space, domain
}

// RequireProject はプロジェクトが設定されていることを確認する
func RequireProject(cfg *config.Store) error {
	projectKey := GetCurrentProject(cfg)
	if projectKey == "" {
		return fmt.Errorf("project is required\nSpecify with -p/--project flag, set in .backlog.yaml, or 'backlog config set profile.default.project <key>'")
	}
	return nil
}

// GetCurrentProject は現在のプロジェクトキーを取得する
// プロジェクト設定（.backlog.yaml）を優先し、なければプロファイル設定を使用
func GetCurrentProject(cfg *config.Store) string {
	// プロジェクト設定を優先
	project := cfg.Project()
	if project != nil && project.Name != "" {
		return project.Name
	}

	// フォールバック: プロファイル設定
	profile := cfg.CurrentProfile()
	if profile != nil {
		return profile.Project
	}
	return ""
}

// ParseIssueKey は課題キーを解析してプロジェクトキーと課題番号を返す
// 形式: "PROJECT-123" -> ("PROJECT", "123", true)
// 数字のみの場合: "123" -> ("", "123", false)
func ParseIssueKey(issueKey string) (projectKey, issueNumber string, hasProject bool) {
	idx := strings.LastIndex(issueKey, "-")
	if idx == -1 {
		// ハイフンなし = 数字のみとみなす
		return "", issueKey, false
	}
	return issueKey[:idx], issueKey[idx+1:], true
}

// ResolveIssueKey は課題キーを解決し、必要に応じてプロジェクトキーを補完または抽出する
// 戻り値:
//   - resolvedKey: 解決済みの課題キー（PROJECT-123形式）
//   - projectKey: 課題キーから抽出または補完に使用したプロジェクトキー
//
// 動作:
//   - "PROJECT-123" + configProject="" -> resolvedKey="PROJECT-123", projectKey="PROJECT"（抽出）
//   - "PROJECT-123" + configProject="OTHER" -> resolvedKey="PROJECT-123", projectKey="PROJECT"（課題キー優先）
//   - "123" + configProject="PROJ" -> resolvedKey="PROJ-123", projectKey="PROJ"（補完）
//   - "123" + configProject="" -> resolvedKey="123", projectKey=""（補完不可）
func ResolveIssueKey(issueKey, configProject string) (resolvedKey, projectKey string) {
	parsed, _, hasProject := ParseIssueKey(issueKey)

	if hasProject {
		// 課題キーにプロジェクトが含まれている場合、そのプロジェクトキーを使用
		return issueKey, parsed
	}

	// 数字のみの場合、設定からプロジェクトキーを補完
	if configProject != "" {
		return configProject + "-" + issueKey, configProject
	}

	// 補完不可
	return issueKey, ""
}
