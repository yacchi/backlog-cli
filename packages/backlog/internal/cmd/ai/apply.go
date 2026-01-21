package ai

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/yacchi/backlog-cli/packages/backlog/internal/config"
	"github.com/yacchi/backlog-cli/packages/backlog/internal/summary/optimizer"
)

var (
	applyFlagPromptType string
	applyFlagSessionID  string
	applyFlagShow       bool
)

// applyCmd はプロンプト適用コマンド
var applyCmd = &cobra.Command{
	Use:   "apply",
	Short: "Apply optimized prompt from history",
	Long: `Apply the best prompt from a previous optimization session to your config.

By default, applies the most recent completed session for the specified prompt type.
You can also specify a session ID to apply a specific session.`,
	Example: `  # Apply the latest optimized issue_list prompt
  backlog ai prompt apply --prompt-type issue_list

  # Apply the latest optimized issue_view prompt
  backlog ai prompt apply --prompt-type issue_view

  # Show the prompt without applying
  backlog ai prompt apply --prompt-type issue_list --show

  # Apply a specific session
  backlog ai prompt apply --session-id abc12345`,
	RunE: runApply,
}

func init() {
	applyCmd.Flags().StringVar(&applyFlagPromptType, "prompt-type", "issue_list", "Prompt type to apply (issue_list or issue_view)")
	applyCmd.Flags().StringVar(&applyFlagSessionID, "session-id", "", "Specific session ID to apply (optional)")
	applyCmd.Flags().BoolVar(&applyFlagShow, "show", false, "Show the prompt without applying")
}

func runApply(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()

	// 設定を読み込み
	cfg, err := config.Load(ctx)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	resolved := cfg.Resolved()

	// キャッシュディレクトリを取得
	cacheDir, err := resolved.Cache.GetCacheDir()
	if err != nil {
		return fmt.Errorf("failed to get cache directory: %w", err)
	}

	// 履歴ストアを作成
	historyStore, err := optimizer.NewHistoryStore(cacheDir)
	if err != nil {
		return fmt.Errorf("failed to create history store: %w", err)
	}

	// プロンプトタイプを検証
	promptType := optimizer.PromptType(applyFlagPromptType)
	if promptType != optimizer.PromptTypeIssueList && promptType != optimizer.PromptTypeIssueView {
		return fmt.Errorf("invalid prompt type: %s (use issue_list or issue_view)", applyFlagPromptType)
	}

	// セッションを取得
	var session *optimizer.OptimizationSession
	if applyFlagSessionID != "" {
		session, err = historyStore.GetSessionByID(applyFlagSessionID)
		if err != nil {
			return fmt.Errorf("failed to get session: %w", err)
		}
		if session == nil {
			return fmt.Errorf("session not found: %s", applyFlagSessionID)
		}
		// セッションのプロンプトタイプを使用
		promptType = session.PromptType
	} else {
		session, err = historyStore.GetLatestCompletedSession(promptType)
		if err != nil {
			return fmt.Errorf("failed to get latest session: %w", err)
		}
		if session == nil {
			return fmt.Errorf("no completed optimization session found for %s", promptType)
		}
	}

	if session.FinalPrompt == "" {
		return fmt.Errorf("session has no final prompt (status: %s)", session.Status)
	}

	// 設定パスを決定
	configPath := config.PathAiSummaryPromptsIssueList
	if promptType == optimizer.PromptTypeIssueView {
		configPath = config.PathAiSummaryPromptsIssueView
	}

	// セッション情報を表示
	fmt.Printf("[セッション情報]\n")
	fmt.Printf("  ID:         %s\n", session.ID[:8])
	fmt.Printf("  タイプ:     %s\n", session.PromptType)
	fmt.Printf("  完了日時:   %s\n", session.CompletedAt.Format("2006-01-02 15:04:05"))
	fmt.Printf("  反復回数:   %d\n", len(session.Iterations))
	fmt.Println()

	// プロンプト内容を表示（インデント付き）
	fmt.Printf("[プロンプト内容]\n")
	for _, line := range strings.Split(session.FinalPrompt, "\n") {
		fmt.Printf("  %s\n", line)
	}
	fmt.Println()

	if applyFlagShow {
		fmt.Printf("適用先: %s\n", configPath)
		return nil
	}

	// 設定に適用
	if err := cfg.Set(configPath, session.FinalPrompt); err != nil {
		return fmt.Errorf("failed to apply prompt: %w", err)
	}

	if err := cfg.Save(ctx); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	fmt.Printf("設定に保存しました: %s\n", configPath)
	return nil
}
