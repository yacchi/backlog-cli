package summary

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/yacchi/backlog-cli/packages/backlog/internal/config"
	"github.com/yacchi/backlog-cli/packages/backlog/internal/debug"
)

// Summarizer はAI要約を行う構造体
type Summarizer struct {
	provider Provider
	config   *config.ResolvedAISummary
}

// NewSummarizer は設定からSummarizerを作成する
func NewSummarizer(cfg *config.ResolvedAISummary) (*Summarizer, error) {
	if !cfg.Enabled {
		return nil, fmt.Errorf("AI summary is not enabled")
	}

	provider, err := NewCommandProvider(cfg)
	if err != nil {
		return nil, err
	}

	return &Summarizer{
		provider: provider,
		config:   cfg,
	}, nil
}

// SummarizeBatch は複数の課題を一括で要約する
// バッチサイズと並列実行数を考慮して処理する
// 要約取得に失敗した課題はリトライする
func (s *Summarizer) SummarizeBatch(ctx context.Context, issues []IssueInput) (map[string]string, error) {
	if len(issues) == 0 {
		return make(map[string]string), nil
	}

	// 設定から各パラメータを取得
	batchSize := s.config.GetBatchSize()
	concurrency := s.config.GetConcurrency()
	retryCount := s.config.RetryCount
	retryDelay := time.Duration(s.config.RetryDelay) * time.Second

	// 入力課題のキーを収集（リトライ対象の特定に使用）
	issueMap := make(map[string]IssueInput, len(issues))
	for _, issue := range issues {
		issueMap[issue.Key] = issue
	}

	// 初回実行
	results, err := s.summarizeBatchWithConcurrency(ctx, issues, batchSize, concurrency)
	if err != nil {
		return nil, err
	}

	// リトライ処理
	for retry := 0; retry < retryCount; retry++ {
		// 失敗した課題（結果が取れなかった課題）を特定
		var failedIssues []IssueInput
		for key, issue := range issueMap {
			if _, ok := results[key]; !ok {
				failedIssues = append(failedIssues, issue)
			}
		}

		// 全て取得できていればリトライ不要
		if len(failedIssues) == 0 {
			break
		}

		debug.Log("AI summary: retrying failed issues",
			"retry", retry+1,
			"max_retries", retryCount,
			"failed_count", len(failedIssues),
		)

		// リトライ前に待機
		select {
		case <-ctx.Done():
			return results, ctx.Err()
		case <-time.After(retryDelay):
		}

		// 失敗した課題をリトライ
		retryResults, retryErr := s.summarizeBatchWithConcurrency(ctx, failedIssues, batchSize, concurrency)
		if retryErr != nil {
			debug.Log("AI summary: retry failed",
				"retry", retry+1,
				"error", retryErr,
			)
			continue
		}

		// リトライ結果をマージ
		for k, v := range retryResults {
			results[k] = v
		}
	}

	// 最終的に取得できなかった課題をログ出力
	var finalFailedKeys []string
	for key := range issueMap {
		if _, ok := results[key]; !ok {
			finalFailedKeys = append(finalFailedKeys, key)
		}
	}
	if len(finalFailedKeys) > 0 {
		debug.Log("AI summary: some issues could not be summarized",
			"failed_keys", finalFailedKeys,
		)
	}

	return results, nil
}

// summarizeBatchWithConcurrency は並列処理でバッチ要約を実行する
func (s *Summarizer) summarizeBatchWithConcurrency(ctx context.Context, issues []IssueInput, batchSize, concurrency int) (map[string]string, error) {
	// バッチサイズ以下なら従来通り一括処理
	if len(issues) <= batchSize {
		return s.summarizeBatchInternal(ctx, issues)
	}

	debug.Log("AI summary: batch processing",
		"total_issues", len(issues),
		"batch_size", batchSize,
		"concurrency", concurrency,
	)

	// バッチに分割
	batches := splitIntoBatches(issues, batchSize)

	// 結果格納
	results := make(map[string]string)
	var mu sync.Mutex

	// セマフォ（並列実行数制限）
	sem := make(chan struct{}, concurrency)
	var wg sync.WaitGroup

	// エラー収集（部分的な失敗を許容）
	var errors []error
	var errMu sync.Mutex

	for i, batch := range batches {
		batch := batch
		batchNum := i + 1
		wg.Add(1)

		go func() {
			defer wg.Done()

			// セマフォ取得
			sem <- struct{}{}
			defer func() { <-sem }()

			debug.Log("AI summary: processing batch",
				"batch", batchNum,
				"total_batches", len(batches),
				"issues_in_batch", len(batch),
			)

			// 各バッチを処理
			batchResult, err := s.summarizeBatchInternal(ctx, batch)
			if err != nil {
				errMu.Lock()
				errors = append(errors, fmt.Errorf("batch %d failed: %w", batchNum, err))
				errMu.Unlock()
				return
			}

			// 結果をマージ
			mu.Lock()
			for k, v := range batchResult {
				results[k] = v
			}
			mu.Unlock()

			debug.Log("AI summary: batch completed",
				"batch", batchNum,
				"results", len(batchResult),
			)
		}()
	}

	wg.Wait()

	// 全バッチ失敗の場合のみエラーを返す
	if len(errors) > 0 && len(results) == 0 {
		return nil, fmt.Errorf("all batches failed: %v", errors)
	}

	// 部分的な失敗はログに記録するが処理続行
	if len(errors) > 0 {
		debug.Log("AI summary: some batches failed",
			"failed_count", len(errors),
			"errors", errors,
		)
	}

	return results, nil
}

// summarizeBatchInternal は単一バッチの課題を要約する（内部用）
func (s *Summarizer) summarizeBatchInternal(ctx context.Context, issues []IssueInput) (map[string]string, error) {
	// 入力をフォーマット
	input := FormatInput(issues)

	// プロンプトテンプレート取得
	prompt := s.config.Prompts.IssueList
	if prompt == "" {
		prompt = defaultIssueListPrompt
	}

	// AI呼び出し
	output, err := s.provider.Summarize(ctx, prompt, input)
	if err != nil {
		return nil, fmt.Errorf("AI summarization failed: %w", err)
	}

	// 出力をパース
	return ParseOutput(output), nil
}

// splitIntoBatches は課題リストを指定サイズのバッチに分割する
func splitIntoBatches(issues []IssueInput, batchSize int) [][]IssueInput {
	var batches [][]IssueInput
	for i := 0; i < len(issues); i += batchSize {
		end := i + batchSize
		if end > len(issues) {
			end = len(issues)
		}
		batches = append(batches, issues[i:end])
	}
	return batches
}

// SummarizeSingle は単一の課題を要約する
// プロンプトテンプレートを使用して、課題詳細を要約する
func (s *Summarizer) SummarizeSingle(ctx context.Context, issue IssueInput) (string, error) {
	// 入力をフォーマット
	input := FormatSingleInput(issue)

	// プロンプトテンプレート取得
	prompt := s.config.Prompts.IssueView
	if prompt == "" {
		prompt = defaultIssueViewPrompt
	}

	// AI呼び出し
	output, err := s.provider.Summarize(ctx, prompt, input)
	if err != nil {
		return "", fmt.Errorf("AI summarization failed: %w", err)
	}

	// 出力をパース（単一課題）
	return ParseSingleOutput(output, issue.Key), nil
}

// デフォルトのプロンプトテンプレート
const defaultIssueListPrompt = `以下の課題一覧を要約してください。
各課題につき1行（50文字以内）で要点をまとめてください。

出力形式:
=== 課題キー ===
要約文`

const defaultIssueViewPrompt = `以下の課題を3文程度で要約してください。
背景、現状、次のアクションがわかるように。

出力形式:
=== 課題キー ===
要約文（複数行可）`
