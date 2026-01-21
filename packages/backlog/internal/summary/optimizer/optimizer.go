package optimizer

import (
	"context"
	"fmt"
	"io"
	"math/rand"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/yacchi/backlog-cli/packages/backlog/internal/api"
	"github.com/yacchi/backlog-cli/packages/backlog/internal/config"
	"github.com/yacchi/backlog-cli/packages/backlog/internal/summary"
)

// Optimizer はプロンプト最適化を実行する
type Optimizer struct {
	config        *OptimizationConfig
	outputModel   summary.Provider
	evalModel     summary.Provider
	issueSelector *IssueSelector
	evaluator     *Evaluator
	improver      *Improver
	comparator    *Comparator
	history       *HistoryStore
	out           io.Writer
	capabilities  *ProviderCapabilities
}

// NewOptimizer は新しいOptimizerを作成する
func NewOptimizer(
	cfg *OptimizationConfig,
	outputModel summary.Provider,
	evalModel summary.Provider,
	apiClient *api.Client,
	history *HistoryStore,
	out io.Writer,
) *Optimizer {
	issueSelector := NewIssueSelector(apiClient)
	evaluator := NewEvaluator(evalModel, cfg.OutputModel)
	improver := NewImprover(evalModel, cfg.OutputModel, cfg.PromptEngineeringTips)
	comparator := NewComparator(evalModel, cfg.OutputModel)

	opt := &Optimizer{
		config:        cfg,
		outputModel:   outputModel,
		evalModel:     evalModel,
		issueSelector: issueSelector,
		evaluator:     evaluator,
		improver:      improver,
		comparator:    comparator,
		history:       history,
		out:           out,
		capabilities:  &ProviderCapabilities{WebSearch: true}, // デフォルトは有効
	}

	// verbose設定を子コンポーネントに伝播
	if cfg.Verbose {
		evaluator.SetVerbose(true, out)
		improver.SetVerbose(true, out)
		comparator.SetVerbose(true, out)
	}

	return opt
}

// SetCapabilities は評価モデルの機能を設定する
func (o *Optimizer) SetCapabilities(capabilities *ProviderCapabilities) {
	o.capabilities = capabilities
}

// OptimizeResult は最適化の結果
type OptimizeResult struct {
	SessionID   string
	FinalPrompt string
	FinalScore  int
	Iterations  int
	Success     bool
	Error       string
}

// Optimize は最適化を実行する
func (o *Optimizer) Optimize(ctx context.Context, promptType PromptType, initialPrompt string) (*OptimizeResult, error) {
	sessionID := uuid.New().String()

	// セッション開始を記録
	if err := o.history.WriteSessionStart(sessionID, promptType, initialPrompt, *o.config); err != nil {
		o.printf("警告: 履歴の記録に失敗しました: %v\n", err)
	}

	o.printf("%s プロンプトの最適化を開始...\n", promptType)
	o.printf("目標スコア: %d\n", o.config.ScoreThreshold)
	o.printf("最大反復回数: %d\n\n", o.config.MaxIterations)

	// モデル情報を表示
	o.printf("[モデル情報]\n")
	if cmdInfo, ok := o.outputModel.(summary.CommandInfo); ok {
		o.printf("出力モデル: %s\n", cmdInfo.GetCommandInfo())
	} else {
		o.printf("出力モデル: %s\n", o.config.OutputModel.ProviderName)
	}
	if cmdInfo, ok := o.evalModel.(summary.CommandInfo); ok {
		o.printf("評価モデル: %s\n", cmdInfo.GetCommandInfo())
	} else {
		o.printf("評価モデル: %s\n", o.config.EvaluationProvider)
	}
	o.printf("\n")

	// 課題プールを作成（評価モデルが候補から選定）
	o.printf("[課題プール作成]\n")
	issuePool, err := o.createIssuePool(ctx)
	if err != nil {
		return o.failSession(sessionID, fmt.Errorf("課題プール作成に失敗: %w", err))
	}
	o.printf("課題プール準備完了: %d件\n\n", len(issuePool))

	currentPrompt := initialPrompt
	var bestScore int
	var bestPrompt string
	var failedImprovements []string // 旧プロンプトに負けた改善案を記録

	// 最適化ループ
	for iteration := 1; iteration <= o.config.MaxIterations; iteration++ {
		o.printf("[反復 %d/%d]\n", iteration, o.config.MaxIterations)

		// プールからランダムにサンプルを選択
		issueData := sampleFromPool(issuePool, o.config.SampleCount)
		o.printf("評価課題: %s\n", formatIssueKeys(issueData))

		iterResult, err := o.runIterationWithHistory(ctx, sessionID, iteration, promptType, currentPrompt, issueData, failedImprovements)
		if err != nil {
			o.printf("  エラー: %v\n", err)
			continue
		}

		// ベストスコアを更新
		if iterResult.Evaluation.Score > bestScore {
			bestScore = iterResult.Evaluation.Score
			bestPrompt = iterResult.CurrentPrompt
		}

		// 終了条件チェック
		if iterResult.Evaluation.Score >= o.config.ScoreThreshold {
			o.printf("\n目標スコア達成! (%d/%d)\n", iterResult.Evaluation.Score, o.config.ScoreThreshold)
			return o.completeSession(sessionID, currentPrompt, bestScore)
		}

		// 比較結果に応じてプロンプトを更新
		if iterResult.SelectedPrompt == "new" && iterResult.ImprovedPrompt != "" {
			// 新プロンプトが勝利: プロンプトを更新し、失敗履歴をクリア
			currentPrompt = iterResult.ImprovedPrompt
			failedImprovements = nil
			o.printf("  → 新プロンプトを採用\n")
		} else if iterResult.SelectedPrompt == "old" && iterResult.ImprovedPrompt != "" {
			// 旧プロンプトが勝利: 失敗した改善として記録（最大3件保持）
			failedImprovements = append(failedImprovements, iterResult.ImprovedPrompt)
			if len(failedImprovements) > 3 {
				failedImprovements = failedImprovements[len(failedImprovements)-3:]
			}
			o.printf("  → 旧プロンプトを維持（失敗した改善として記録）\n")
		}

		o.printf("\n")
	}

	// 最大反復回数到達
	o.printf("\n最大反復回数に到達しました。\n")
	if bestPrompt != "" {
		currentPrompt = bestPrompt
	}

	return o.completeSession(sessionID, currentPrompt, bestScore)
}

// createIssuePool は評価モデルを使って課題プールを作成する
func (o *Optimizer) createIssuePool(ctx context.Context) ([]summary.IssueInput, error) {
	// 候補課題を取得
	o.printf("候補課題を取得中...\n")
	candidates, err := o.issueSelector.FetchCandidateIssues(ctx, o.config.TargetProjects, o.config.CandidateCount)
	if err != nil {
		return nil, err
	}

	if len(candidates) == 0 {
		return nil, fmt.Errorf("候補課題が見つかりません")
	}
	o.printf("  取得完了: %d件\n", len(candidates))

	// プールサイズを決定（各反復で異なる課題を使用するため、SampleCount * 反復回数分を確保）
	poolSize := o.config.SampleCount * o.config.MaxIterations
	if poolSize > len(candidates) {
		poolSize = len(candidates)
	}
	// 最低でもSampleCount * 2は確保
	if poolSize < o.config.SampleCount*2 && len(candidates) >= o.config.SampleCount*2 {
		poolSize = o.config.SampleCount * 2
	}

	// 評価モデルで選定
	o.printf("評価モデルが精度調整に適した課題を選定中...\n")
	selectedIssues, err := o.evaluator.SelectIssues(ctx, candidates, poolSize)
	if err != nil {
		return nil, fmt.Errorf("課題選定に失敗: %w", err)
	}

	o.printf("  選定完了: %d件\n", len(selectedIssues.SelectedIssues))
	for _, issue := range selectedIssues.SelectedIssues {
		o.printf("    %s: %s\n", issue.IssueKey, issue.SelectionReason)
	}
	o.printf("  選定基準: %s\n", selectedIssues.SelectionCriteria)

	// 選定された課題のデータを取得
	o.printf("課題データを取得中...\n")
	issueData, err := o.fetchIssueData(ctx, selectedIssues.SelectedIssues)
	if err != nil {
		return nil, fmt.Errorf("課題データ取得に失敗: %w", err)
	}

	return issueData, nil
}

// sampleFromPool はプールからランダムにサンプルを選択する
func sampleFromPool(pool []summary.IssueInput, count int) []summary.IssueInput {
	if count >= len(pool) {
		// プール全体を返す（シャッフルして順序を変える）
		shuffled := make([]summary.IssueInput, len(pool))
		copy(shuffled, pool)
		rand.Shuffle(len(shuffled), func(i, j int) {
			shuffled[i], shuffled[j] = shuffled[j], shuffled[i]
		})
		return shuffled
	}

	// Fisher-Yatesシャッフルでランダムサンプリング
	indices := make([]int, len(pool))
	for i := range indices {
		indices[i] = i
	}
	rand.Shuffle(len(indices), func(i, j int) {
		indices[i], indices[j] = indices[j], indices[i]
	})

	result := make([]summary.IssueInput, count)
	for i := 0; i < count; i++ {
		result[i] = pool[indices[i]]
	}
	return result
}

// formatIssueKeys は課題キーのリストをフォーマットする
func formatIssueKeys(issues []summary.IssueInput) string {
	keys := make([]string, len(issues))
	for i, issue := range issues {
		keys[i] = issue.Key
	}
	return strings.Join(keys, ", ")
}

// fetchIssueData は選定された課題のデータを取得する
func (o *Optimizer) fetchIssueData(ctx context.Context, selected []SelectedIssue) ([]summary.IssueInput, error) {
	var issueData []summary.IssueInput

	for _, sel := range selected {
		issue, comments, err := o.issueSelector.FetchIssueWithComments(ctx, sel.IssueKey, 10)
		if err != nil {
			o.printf("  警告: %s の取得に失敗: %v\n", sel.IssueKey, err)
			continue
		}

		issueData = append(issueData, summary.IssueInput{
			Key:         issue.Key,
			Title:       issue.Summary,
			Description: issue.Description,
			Comments:    comments,
		})
	}

	if len(issueData) == 0 {
		return nil, fmt.Errorf("課題データを取得できませんでした")
	}

	return issueData, nil
}

// runIterationWithHistory は失敗履歴を考慮した1回の最適化反復を実行する
func (o *Optimizer) runIterationWithHistory(ctx context.Context, sessionID string, iterNum int, promptType PromptType, currentPrompt string, issueData []summary.IssueInput, failedImprovements []string) (*OptimizationIteration, error) {
	iter := &OptimizationIteration{
		Number:        iterNum,
		Timestamp:     time.Now(),
		CurrentPrompt: currentPrompt,
	}

	// 現在のプロンプトを表示
	o.printf("[現在のプロンプト]\n")
	for _, line := range strings.Split(currentPrompt, "\n") {
		o.printf("  %s\n", line)
	}
	o.printf("\n")

	// 要約生成
	o.printf("現在のプロンプトで要約生成中...\n")
	samples, err := o.generateSamples(ctx, promptType, currentPrompt, issueData)
	if err != nil {
		return nil, fmt.Errorf("要約生成に失敗: %w", err)
	}
	iter.SampleResults = samples

	// 評価
	o.printf("評価中...\n")
	evaluation, err := o.evaluator.Evaluate(ctx, promptType, currentPrompt, samples)
	if err != nil {
		return nil, fmt.Errorf("評価に失敗: %w", err)
	}
	iter.Evaluation = *evaluation

	o.printf("  スコア: %d/10\n", evaluation.Score)
	if len(evaluation.Strengths) > 0 {
		o.printf("  強み:\n")
		for _, s := range evaluation.Strengths {
			o.printf("    - %s\n", s)
		}
	}
	if len(evaluation.Weaknesses) > 0 {
		o.printf("  弱み:\n")
		for _, w := range evaluation.Weaknesses {
			o.printf("    - %s\n", w)
		}
	}

	// 目標スコア未達の場合は改善
	if evaluation.Score < o.config.ScoreThreshold {
		o.printf("改善プロンプト生成中...\n")

		// 失敗履歴がある場合は表示
		if len(failedImprovements) > 0 {
			o.printf("  過去に失敗した改善: %d件（異なるアプローチを試行）\n", len(failedImprovements))
		}

		var improved string
		var improveErr error

		if o.capabilities != nil && o.capabilities.WebSearch {
			o.printf("  ベストプラクティス収集中（Web検索）...\n")
			o.printf("  改善案を生成中...\n")
			improved, improveErr = o.improver.ImprovePromptWithWebSearchAndHistory(ctx, promptType, currentPrompt, evaluation, samples, failedImprovements)
		} else {
			o.printf("  改善案を生成中（Web検索なし）...\n")
			improved, improveErr = o.improver.ImprovePromptWithHistory(ctx, promptType, currentPrompt, evaluation, samples, failedImprovements)
		}

		if improveErr != nil {
			o.printf("  警告: 改善生成に失敗: %v\n", improveErr)
		} else {
			iter.ImprovedPrompt = improved

			// 改善されたプロンプトを表示
			o.printf("[改善後のプロンプト]\n")
			for _, line := range strings.Split(improved, "\n") {
				o.printf("  %s\n", line)
			}
			o.printf("\n")

			// 比較
			o.printf("新旧プロンプト比較中...\n")
			newSamples, err := o.generateSamples(ctx, promptType, improved, issueData)
			if err != nil {
				o.printf("  警告: 新プロンプトでの要約生成に失敗: %v\n", err)
			} else {
				comparison, err := o.comparator.CompareResults(ctx, promptType, currentPrompt, samples, improved, newSamples)
				if err != nil {
					o.printf("  警告: 比較に失敗: %v\n", err)
				} else {
					iter.Comparison = comparison
					iter.SelectedPrompt = comparison.Winner
					o.printf("  勝者: %sプロンプト (%d vs %d)\n",
						winnerLabel(comparison.Winner),
						comparison.OldPromptScore,
						comparison.NewPromptScore)
				}
			}
		}
	}

	// 履歴に記録
	if err := o.history.WriteIteration(sessionID, *iter); err != nil {
		o.printf("警告: 履歴の記録に失敗しました: %v\n", err)
	}

	return iter, nil
}

// generateSamples は課題データから要約サンプルを生成する
func (o *Optimizer) generateSamples(ctx context.Context, promptType PromptType, prompt string, issueData []summary.IssueInput) ([]SampleResult, error) {
	if promptType == PromptTypeIssueList {
		return o.generateSamplesBatch(ctx, prompt, issueData)
	}
	return o.generateSamplesParallel(ctx, prompt, issueData)
}

// generateSamplesBatch は複数課題をまとめて1回のAPI呼び出しで処理する（issue_list用）
func (o *Optimizer) generateSamplesBatch(ctx context.Context, prompt string, issueData []summary.IssueInput) ([]SampleResult, error) {
	start := time.Now()

	// 全課題をまとめてフォーマット
	input := summary.FormatInput(issueData)

	o.printf("  %d件の課題を一括処理中...\n", len(issueData))

	// verbose: 出力モデルへの入力を表示
	if o.config.Verbose {
		o.printf("\n%s\n", verboseSeparator("出力モデル - プロンプト"))
		o.printf("%s\n", prompt)
		o.printf("%s\n\n", verboseSeparatorEnd())
		o.printf("%s\n", verboseSeparator("出力モデル - 入力データ"))
		o.printf("%s\n", input)
		o.printf("%s\n\n", verboseSeparatorEnd())
	}

	// 要約生成（1回のAPI呼び出し）
	output, err := o.outputModel.Summarize(ctx, prompt, input)
	if err != nil {
		return nil, fmt.Errorf("バッチ要約生成に失敗: %w", err)
	}

	// verbose: 出力モデルのレスポンスを表示
	if o.config.Verbose {
		o.printf("%s\n", verboseSeparator("出力モデル - レスポンス"))
		o.printf("%s\n", output)
		o.printf("%s\n\n", verboseSeparatorEnd())
	}

	elapsed := time.Since(start)
	o.printf("  一括生成完了 (%.1f秒)\n", elapsed.Seconds())

	// 出力をパースして個別の要約を抽出
	summaries := summary.ParseOutput(output)

	var samples []SampleResult
	for _, issue := range issueData {
		issueSummary, found := summaries[issue.Key]
		if !found {
			o.printf("  警告: %s の要約が出力に含まれていません\n", issue.Key)
			issueSummary = ""
		}

		// 比較プロンプトで個別課題を表示できるよう、課題ごとの入力を記録
		singleInput := summary.FormatSingleInput(issue)

		samples = append(samples, SampleResult{
			IssueKey:    issue.Key,
			Input:       singleInput,
			Output:      issueSummary,
			ElapsedTime: elapsed,
		})
	}

	if len(samples) == 0 {
		return nil, fmt.Errorf("サンプルを生成できませんでした")
	}

	return samples, nil
}

// generateSamplesParallel は課題を並行処理する（issue_view用）
func (o *Optimizer) generateSamplesParallel(ctx context.Context, prompt string, issueData []summary.IssueInput) ([]SampleResult, error) {
	type result struct {
		index  int
		sample SampleResult
		err    error
	}

	results := make(chan result, len(issueData))

	// verbose: プロンプトを表示（並列処理なので最初に1回だけ）
	if o.config.Verbose {
		o.printf("\n%s\n", verboseSeparator("出力モデル - プロンプト"))
		o.printf("%s\n", prompt)
		o.printf("%s\n\n", verboseSeparatorEnd())
	}

	// 並行処理で要約を生成
	for i, issue := range issueData {
		go func() {
			start := time.Now()
			input := summary.FormatSingleInput(issue)

			output, err := o.outputModel.Summarize(ctx, prompt, input)
			if err != nil {
				results <- result{index: i, err: fmt.Errorf("%s: %w", issue.Key, err)}
				return
			}

			elapsed := time.Since(start)
			results <- result{
				index: i,
				sample: SampleResult{
					IssueKey:    issue.Key,
					Input:       input,
					Output:      output,
					ElapsedTime: elapsed,
				},
			}
		}()
	}

	// 結果を収集
	samples := make([]SampleResult, len(issueData))
	var errors []string
	completed := 0

	for completed < len(issueData) {
		res := <-results
		completed++

		if res.err != nil {
			o.printf("  失敗: %v\n", res.err)
			errors = append(errors, res.err.Error())
		} else {
			samples[res.index] = res.sample
			o.printf("  %s: 生成完了 (%.1f秒)\n", res.sample.IssueKey, res.sample.ElapsedTime.Seconds())
		}
	}

	// エラーがあった課題を除去
	var validSamples []SampleResult
	for _, s := range samples {
		if s.IssueKey != "" {
			validSamples = append(validSamples, s)
		}
	}

	if len(validSamples) == 0 {
		return nil, fmt.Errorf("サンプルを生成できませんでした: %s", strings.Join(errors, "; "))
	}

	// verbose: 各課題の入出力を表示
	if o.config.Verbose {
		for _, s := range validSamples {
			o.printf("%s\n", verboseSeparator(fmt.Sprintf("出力モデル - %s 入力", s.IssueKey)))
			o.printf("%s\n", s.Input)
			o.printf("%s\n\n", verboseSeparatorEnd())
			o.printf("%s\n", verboseSeparator(fmt.Sprintf("出力モデル - %s レスポンス", s.IssueKey)))
			o.printf("%s\n", s.Output)
			o.printf("%s\n\n", verboseSeparatorEnd())
		}
	}

	return validSamples, nil
}

// completeSession はセッションを正常終了する
func (o *Optimizer) completeSession(sessionID, finalPrompt string, finalScore int) (*OptimizeResult, error) {
	if err := o.history.WriteSessionEnd(sessionID, SessionStatusCompleted, finalPrompt, finalScore, ""); err != nil {
		o.printf("警告: 履歴の記録に失敗しました: %v\n", err)
	}

	o.printf("\n最適化完了!\n")
	o.printf("最終スコア: %d/10\n", finalScore)
	o.printf("\n最適化されたプロンプト:\n")
	o.printf("================\n")
	o.printf("%s\n", finalPrompt)
	o.printf("================\n")

	return &OptimizeResult{
		SessionID:   sessionID,
		FinalPrompt: finalPrompt,
		FinalScore:  finalScore,
		Success:     true,
	}, nil
}

// failSession はセッションを失敗で終了する
func (o *Optimizer) failSession(sessionID string, err error) (*OptimizeResult, error) {
	errMsg := err.Error()
	if writeErr := o.history.WriteSessionEnd(sessionID, SessionStatusFailed, "", 0, errMsg); writeErr != nil {
		o.printf("警告: 履歴の記録に失敗しました: %v\n", writeErr)
	}

	return &OptimizeResult{
		SessionID: sessionID,
		Success:   false,
		Error:     errMsg,
	}, err
}

// printf は出力に書き込む
func (o *Optimizer) printf(format string, args ...interface{}) {
	if o.out != nil {
		_, _ = fmt.Fprintf(o.out, format, args...)
	}
}

// winnerLabel は勝者ラベルを返す
func winnerLabel(winner string) string {
	if winner == "new" {
		return "新"
	}
	return "旧"
}

// BuildOptimizationConfig は設定からOptimizationConfigを構築する
func BuildOptimizationConfig(cfg *config.ResolvedConfig) *OptimizationConfig {
	opt := cfg.AISummary.Optimization
	provider := cfg.AISummary.GetActiveProvider()

	var modelInfo OutputModelInfo
	if provider != nil {
		modelInfo = OutputModelInfo{
			ProviderName: cfg.AISummary.Provider,
			Command:      provider.Command,
			Args:         provider.Args,
		}
	}

	// OutputModelContextのプレースホルダーを置換
	outputCtx := opt.OutputModelContext
	outputCtx = strings.ReplaceAll(outputCtx, "{provider_name}", modelInfo.ProviderName)
	outputCtx = strings.ReplaceAll(outputCtx, "{command}", modelInfo.Command)
	outputCtx = strings.ReplaceAll(outputCtx, "{args}", strings.Join(modelInfo.Args, " "))

	// デフォルト値を設定
	candidateCount := opt.CandidateCount
	if candidateCount == 0 {
		candidateCount = 100 // デフォルト: API最大の100件
	}
	sampleCount := opt.SampleCount
	if sampleCount == 0 {
		sampleCount = 5 // デフォルト: 5件
	}

	return &OptimizationConfig{
		EvaluationProvider:    opt.EvaluationProvider,
		ScoreThreshold:        opt.ScoreThreshold,
		MaxIterations:         opt.MaxIterations,
		CandidateCount:        candidateCount,
		SampleCount:           sampleCount,
		TargetProjects:        opt.TargetProjects,
		OutputModelContext:    outputCtx,
		PromptEngineeringTips: opt.PromptEngineeringTips,
		OutputModel:           modelInfo,
	}
}
