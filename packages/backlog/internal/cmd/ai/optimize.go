package ai

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/yacchi/backlog-cli/packages/backlog/internal/api"
	"github.com/yacchi/backlog-cli/packages/backlog/internal/config"
	"github.com/yacchi/backlog-cli/packages/backlog/internal/summary"
	"github.com/yacchi/backlog-cli/packages/backlog/internal/summary/optimizer"
)

var (
	flagPromptType         string
	flagProjects           []string
	flagMaxIterations      int
	flagScoreThreshold     int
	flagEvaluationProvider string
	flagDryRun             bool
	flagShowHistory        bool
	flagNoWebSearch        bool
	flagVerbose            bool
)

// optimizeCmd はプロンプト最適化コマンド
var optimizeCmd = &cobra.Command{
	Use:   "optimize",
	Short: "Optimize AI summary prompts",
	Long: `Optimize AI summary prompts using an evaluation model.

This command uses a two-model approach:
1. Output model: Generates summaries using the current prompt
2. Evaluation model: Evaluates quality and suggests improvements

The optimization loop continues until the score threshold is reached
or the maximum number of iterations is completed.`,
	Example: `  # Optimize the issue_list prompt for a single project
  backlog ai prompt optimize --prompt-type issue_list --projects MYPROJ

  # Optimize using multiple projects
  backlog ai prompt optimize --projects PROJ1,PROJ2,PROJ3

  # Set custom thresholds
  backlog ai prompt optimize --max-iterations 10 --score-threshold 9

  # Run without web search (degraded mode)
  backlog ai prompt optimize --no-web-search

  # Show detailed prompts and outputs during optimization
  backlog ai prompt optimize --verbose

  # Show optimization history
  backlog ai prompt optimize --show-history

  # After optimization, apply the result with:
  backlog ai prompt apply --prompt-type issue_list`,
	RunE: runOptimize,
}

func init() {
	optimizeCmd.Flags().StringVar(&flagPromptType, "prompt-type", "issue_list", "Prompt type to optimize (issue_list or issue_view)")
	optimizeCmd.Flags().StringSliceVar(&flagProjects, "projects", nil, "Target projects for issue selection (comma-separated, empty for all projects)")
	optimizeCmd.Flags().IntVar(&flagMaxIterations, "max-iterations", 0, "Maximum iterations (0 = use config)")
	optimizeCmd.Flags().IntVar(&flagScoreThreshold, "score-threshold", 0, "Target score threshold (0 = use config)")
	optimizeCmd.Flags().StringVar(&flagEvaluationProvider, "evaluation-provider", "", "Evaluation model provider (empty = use config)")
	optimizeCmd.Flags().BoolVar(&flagDryRun, "dry-run", false, "Show what would be done without making changes")
	optimizeCmd.Flags().BoolVar(&flagShowHistory, "show-history", false, "Show optimization history")
	optimizeCmd.Flags().BoolVar(&flagNoWebSearch, "no-web-search", false, "Run without web search (degraded mode)")
	optimizeCmd.Flags().BoolVar(&flagVerbose, "verbose", false, "Show detailed prompts and outputs during optimization")
}

func runOptimize(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()

	// 設定を読み込み
	cfg, err := config.Load(ctx)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	resolved := cfg.Resolved()

	// AI要約が有効か確認
	if !resolved.AISummary.Enabled {
		return fmt.Errorf("AI summary is not enabled; enable it in config first")
	}

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

	// 履歴表示モード
	if flagShowHistory {
		return showHistory(historyStore)
	}

	// 最適化設定を構築
	optConfig := optimizer.BuildOptimizationConfig(resolved)

	// フラグで上書き
	if flagMaxIterations > 0 {
		optConfig.MaxIterations = flagMaxIterations
	}
	if flagScoreThreshold > 0 {
		optConfig.ScoreThreshold = flagScoreThreshold
	}
	if flagEvaluationProvider != "" {
		optConfig.EvaluationProvider = flagEvaluationProvider
	}
	if len(flagProjects) > 0 {
		optConfig.TargetProjects = flagProjects
	}
	if flagVerbose {
		optConfig.Verbose = true
	}

	// プロンプトタイプを検証
	promptType := optimizer.PromptType(flagPromptType)
	if promptType != optimizer.PromptTypeIssueList && promptType != optimizer.PromptTypeIssueView {
		return fmt.Errorf("invalid prompt type: %s (use issue_list or issue_view)", flagPromptType)
	}

	// 現在のプロンプトを取得
	var initialPrompt string
	if promptType == optimizer.PromptTypeIssueList {
		initialPrompt = resolved.AISummary.Prompts.IssueList
	} else {
		initialPrompt = resolved.AISummary.Prompts.IssueView
	}

	if flagDryRun {
		fmt.Println("Dry run mode - showing configuration:")
		fmt.Printf("  Prompt type: %s\n", promptType)
		if len(optConfig.TargetProjects) > 0 {
			fmt.Printf("  Target projects: %s\n", strings.Join(optConfig.TargetProjects, ", "))
		} else {
			fmt.Println("  Target projects: (all projects)")
		}
		fmt.Printf("  Evaluation provider: %s\n", optConfig.EvaluationProvider)
		fmt.Printf("  Score threshold: %d\n", optConfig.ScoreThreshold)
		fmt.Printf("  Max iterations: %d\n", optConfig.MaxIterations)
		fmt.Printf("  Output model: %s\n", optConfig.OutputModel.ProviderName)
		fmt.Println("\nCurrent prompt:")
		fmt.Println("================")
		fmt.Println(initialPrompt)
		fmt.Println("================")
		return nil
	}

	// 出力モデルを作成
	outputModel, err := summary.NewCommandProvider(&resolved.AISummary)
	if err != nil {
		return fmt.Errorf("failed to create output model: %w", err)
	}

	// 評価モデルを作成
	evalProvider := resolved.AISummary.GetProvider(optConfig.EvaluationProvider)
	if evalProvider == nil {
		return fmt.Errorf("evaluation provider %q not found", optConfig.EvaluationProvider)
	}

	// 評価モデル用の設定を作成（評価タスクは複雑なため、専用のタイムアウトを使用）
	evalTimeout := resolved.AISummary.Optimization.EvaluationTimeout
	if evalTimeout == 0 {
		evalTimeout = 300 // デフォルト5分
	}
	evalConfig := &config.ResolvedAISummary{
		Provider:  optConfig.EvaluationProvider,
		Timeout:   evalTimeout,
		Providers: resolved.AISummary.Providers,
	}
	evalModel, err := summary.NewCommandProvider(evalConfig)
	if err != nil {
		return fmt.Errorf("failed to create evaluation model: %w", err)
	}

	// APIクライアントを作成
	apiClient, err := api.NewClientFromConfig(cfg)
	if err != nil {
		return fmt.Errorf("failed to create API client: %w", err)
	}

	// オプティマイザーを作成
	opt := optimizer.NewOptimizer(
		optConfig,
		outputModel,
		evalModel,
		apiClient,
		historyStore,
		os.Stdout,
	)

	// Web検索機能の設定
	capabilities := &optimizer.ProviderCapabilities{WebSearch: !flagNoWebSearch}
	if flagNoWebSearch {
		fmt.Println("Web検索なしモードで実行します。")
	}
	opt.SetCapabilities(capabilities)

	// 最適化を実行
	result, err := opt.Optimize(ctx, promptType, initialPrompt)
	if err != nil {
		return err
	}

	if !result.Success {
		return fmt.Errorf("optimization failed: %s", result.Error)
	}

	// 適用方法を案内
	if result.FinalPrompt != "" {
		fmt.Println("\nTo apply this prompt, run:")
		fmt.Printf("  backlog ai prompt apply --prompt-type %s\n", promptType)
	}

	return nil
}

func showHistory(store *optimizer.HistoryStore) error {
	sessions, err := store.GetRecentSessions(10)
	if err != nil {
		return fmt.Errorf("failed to get history: %w", err)
	}

	if len(sessions) == 0 {
		fmt.Println("No optimization history found.")
		return nil
	}

	fmt.Println("Recent optimization sessions:")
	fmt.Println()

	for _, session := range sessions {
		statusIcon := "?"
		switch session.Status {
		case optimizer.SessionStatusCompleted:
			statusIcon = "v"
		case optimizer.SessionStatusFailed:
			statusIcon = "x"
		case optimizer.SessionStatusRunning:
			statusIcon = ">"
		}

		fmt.Printf("[%s] %s (%s)\n", statusIcon, session.ID[:8], session.PromptType)
		fmt.Printf("    Started: %s\n", session.StartedAt.Format("2006-01-02 15:04:05"))
		if !session.CompletedAt.IsZero() {
			fmt.Printf("    Completed: %s\n", session.CompletedAt.Format("2006-01-02 15:04:05"))
		}
		fmt.Printf("    Iterations: %d\n", len(session.Iterations))
		if session.Error != "" {
			fmt.Printf("    Error: %s\n", session.Error)
		}
		fmt.Println()
	}

	return nil
}
