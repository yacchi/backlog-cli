package optimizer

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"strings"

	"github.com/yacchi/backlog-cli/packages/backlog/internal/summary"
)

// Evaluator は評価モデルを使用した評価・選定・改善を行う
type Evaluator struct {
	provider   summary.Provider
	modelInfo  OutputModelInfo
	verbose    bool
	verboseOut io.Writer
}

// NewEvaluator は新しいEvaluatorを作成する
func NewEvaluator(provider summary.Provider, modelInfo OutputModelInfo) *Evaluator {
	return &Evaluator{
		provider:  provider,
		modelInfo: modelInfo,
	}
}

// SetVerbose は詳細出力モードを設定する
func (e *Evaluator) SetVerbose(verbose bool, out io.Writer) {
	e.verbose = verbose
	e.verboseOut = out
}

// verbosePrintf は詳細モード時にメッセージを出力する
func (e *Evaluator) verbosePrintf(format string, args ...interface{}) {
	if e.verbose && e.verboseOut != nil {
		_, _ = fmt.Fprintf(e.verboseOut, format, args...)
	}
}

// SelectIssues は評価モデルを使って精度調整に適した課題を選定する
func (e *Evaluator) SelectIssues(ctx context.Context, candidates []CandidateIssue, count int) (*IssueSelectionResult, error) {
	prompt := e.buildSelectionPrompt(candidates, count)

	e.verbosePrintf("\n%s\n", verboseSeparator("課題選定プロンプト"))
	e.verbosePrintf("%s\n", prompt)
	e.verbosePrintf("%s\n\n", verboseSeparatorEnd())

	output, err := e.provider.Summarize(ctx, prompt, "")
	if err != nil {
		return nil, fmt.Errorf("failed to select issues: %w", err)
	}

	e.verbosePrintf("%s\n", verboseSeparator("課題選定レスポンス"))
	e.verbosePrintf("%s\n", output)
	e.verbosePrintf("%s\n\n", verboseSeparatorEnd())

	return e.parseSelectionResult(output, candidates)
}

// buildSelectionPrompt は課題選定用のプロンプトを構築する
func (e *Evaluator) buildSelectionPrompt(candidates []CandidateIssue, count int) string {
	candidateList := FormatCandidatesForSelection(candidates)

	return fmt.Sprintf(`あなたはAI要約機能の精度調整のための課題選定を行うアシスタントです。

## タスク
以下の課題候補から、AI要約の精度調整に最も適した%d件を選んでください。

## 選定基準
1. **説明文の充実度**: 説明文が十分に記載されている課題を優先
2. **多様性**: 技術的内容とビジネス内容が混在するように選択
3. **タイプの多様性**: バグ、機能要望、タスクなど異なるタイプを含める
4. **コメントの有無**: コメントがある課題を一部含める（issue_view評価用）
5. **長さのバリエーション**: 短い説明と長い説明の両方を含める

## 出力モデル情報
%s

## 候補課題一覧
%s

## 出力形式
以下のJSON形式で出力してください：
{
  "selected_keys": ["PROJ-001", "PROJ-002", ...],
  "selection_criteria": "選定基準の説明",
  "reasons": {
    "PROJ-001": "選定理由",
    "PROJ-002": "選定理由",
    ...
  }
}
`, count, e.formatModelInfo(), candidateList)
}

// formatModelInfo は出力モデル情報をフォーマットする
func (e *Evaluator) formatModelInfo() string {
	args := strings.Join(e.modelInfo.Args, " ")
	return fmt.Sprintf("プロバイダー: %s\nコマンド: %s %s",
		e.modelInfo.ProviderName, e.modelInfo.Command, args)
}

// parseSelectionResult は選定結果をパースする
func (e *Evaluator) parseSelectionResult(output string, candidates []CandidateIssue) (*IssueSelectionResult, error) {
	// JSONブロックを抽出
	jsonStr := extractJSON(output)
	if jsonStr == "" {
		return nil, fmt.Errorf("no JSON found in output: %s", output)
	}

	var parsed struct {
		SelectedKeys      []string          `json:"selected_keys"`
		SelectionCriteria string            `json:"selection_criteria"`
		Reasons           map[string]string `json:"reasons"`
	}

	if err := json.Unmarshal([]byte(jsonStr), &parsed); err != nil {
		return nil, fmt.Errorf("failed to parse selection result: %w", err)
	}

	// 候補からマッチする課題を取得
	candidateMap := make(map[string]CandidateIssue)
	for _, c := range candidates {
		candidateMap[c.Key] = c
	}

	var selected []SelectedIssue
	for _, key := range parsed.SelectedKeys {
		if c, ok := candidateMap[key]; ok {
			selected = append(selected, SelectedIssue{
				IssueKey:        key,
				IssueType:       c.IssueType,
				Description:     c.Summary,
				CommentCount:    c.CommentCount,
				SelectionReason: parsed.Reasons[key],
			})
		}
	}

	return &IssueSelectionResult{
		SelectedIssues:    selected,
		TotalCandidates:   len(candidates),
		SelectionCriteria: parsed.SelectionCriteria,
	}, nil
}

// Evaluate は要約結果を評価する
func (e *Evaluator) Evaluate(ctx context.Context, promptType PromptType, currentPrompt string, samples []SampleResult) (*EvaluationResult, error) {
	prompt := e.buildEvaluationPrompt(promptType, currentPrompt, samples)

	e.verbosePrintf("\n%s\n", verboseSeparator("評価プロンプト"))
	e.verbosePrintf("%s\n", prompt)
	e.verbosePrintf("%s\n\n", verboseSeparatorEnd())

	output, err := e.provider.Summarize(ctx, prompt, "")
	if err != nil {
		return nil, fmt.Errorf("failed to evaluate: %w", err)
	}

	e.verbosePrintf("%s\n", verboseSeparator("評価レスポンス"))
	e.verbosePrintf("%s\n", output)
	e.verbosePrintf("%s\n\n", verboseSeparatorEnd())

	return e.parseEvaluationResult(output)
}

// buildEvaluationPrompt は評価用のプロンプトを構築する
func (e *Evaluator) buildEvaluationPrompt(promptType PromptType, currentPrompt string, samples []SampleResult) string {
	cb := "```" // code block marker

	var samplesText strings.Builder
	for i, s := range samples {
		samplesText.WriteString(fmt.Sprintf("### サンプル %d: %s\n", i+1, s.IssueKey))
		samplesText.WriteString("**入力:**\n" + cb + "\n")
		samplesText.WriteString(truncateString(s.Input, 500))
		samplesText.WriteString("\n" + cb + "\n")
		samplesText.WriteString("**出力:**\n" + cb + "\n")
		samplesText.WriteString(s.Output)
		samplesText.WriteString("\n" + cb + "\n\n")
	}

	promptTypeDesc := "課題一覧用（1行50文字以内の要約）"
	if promptType == PromptTypeIssueView {
		promptTypeDesc = "課題詳細用（3文程度の要約）"
	}

	return fmt.Sprintf("あなたはAI要約の品質を評価する専門家です。\n\n"+
		"## タスク\n"+
		"以下の要約結果を評価し、改善点を提案してください。\n\n"+
		"## プロンプトタイプ\n%s\n\n"+
		"## 出力モデル情報\n%s\n\n"+
		"## 現在のプロンプト\n"+cb+"\n%s\n"+cb+"\n\n"+
		"## サンプル入出力\n%s\n"+
		"## 評価基準\n"+
		"1. **正確性** (1-10): 要約が課題の要点を正確に捉えているか\n"+
		"2. **簡潔性** (1-10): 適切な長さで簡潔にまとまっているか\n"+
		"3. **明瞭性** (1-10): 読みやすく理解しやすいか\n"+
		"4. **一貫性** (1-10): サンプル間でフォーマットが一貫しているか\n"+
		"5. **アクション可能性** (1-10): 次のアクションが明確か（issue_view用）\n\n"+
		"## 出力形式\n"+
		"以下のJSON形式で出力してください：\n"+
		"{\n"+
		"  \"score\": <総合スコア 1-10>,\n"+
		"  \"feedback\": \"<詳細なフィードバック>\",\n"+
		"  \"strengths\": [\"<強み1>\", \"<強み2>\"],\n"+
		"  \"weaknesses\": [\"<弱み1>\", \"<弱み2>\"],\n"+
		"  \"suggestions\": [\"<改善提案1>\", \"<改善提案2>\"]\n"+
		"}\n",
		promptTypeDesc, e.formatModelInfo(), currentPrompt, samplesText.String())
}

// parseEvaluationResult は評価結果をパースする
func (e *Evaluator) parseEvaluationResult(output string) (*EvaluationResult, error) {
	jsonStr := extractJSON(output)
	if jsonStr == "" {
		return nil, fmt.Errorf("no JSON found in output: %s", output)
	}

	var result EvaluationResult
	if err := json.Unmarshal([]byte(jsonStr), &result); err != nil {
		return nil, fmt.Errorf("failed to parse evaluation result: %w", err)
	}

	return &result, nil
}

// extractJSON は出力からJSONブロックを抽出する
func extractJSON(output string) string {
	// ```json ... ``` ブロックを検索
	codeBlockPattern := regexp.MustCompile("(?s)```(?:json)?\\s*\\n?(\\{.*?\\})\\s*```")
	if matches := codeBlockPattern.FindStringSubmatch(output); len(matches) > 1 {
		return matches[1]
	}

	// 直接JSONを検索
	jsonPattern := regexp.MustCompile(`(?s)\{[^{}]*(?:\{[^{}]*\}[^{}]*)*\}`)
	if matches := jsonPattern.FindStringSubmatch(output); len(matches) > 0 {
		return matches[0]
	}

	return ""
}

// truncateString は文字列を指定長で切り詰める
func truncateString(s string, maxLen int) string {
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	return string(runes[:maxLen]) + "..."
}

// verboseSeparator は詳細出力用のセパレータを生成する
func verboseSeparator(title string) string {
	return fmt.Sprintf("╔══════════════════════════════════════════════════════════════════════════════╗\n║ %-76s ║\n╠══════════════════════════════════════════════════════════════════════════════╣", title)
}

// verboseSeparatorEnd は詳細出力用のセパレータ終端を生成する
func verboseSeparatorEnd() string {
	return "╚══════════════════════════════════════════════════════════════════════════════╝"
}
