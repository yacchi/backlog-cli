package optimizer

import (
	"context"
	"fmt"
	"io"
	"regexp"
	"strings"

	"github.com/yacchi/backlog-cli/packages/backlog/internal/summary"
)

// Improver はプロンプト改善機能を提供する
type Improver struct {
	provider              summary.Provider
	modelInfo             OutputModelInfo
	promptEngineeringTips string
	verbose               bool
	verboseOut            io.Writer
}

// NewImprover は新しいImproverを作成する
func NewImprover(provider summary.Provider, modelInfo OutputModelInfo, tips string) *Improver {
	return &Improver{
		provider:              provider,
		modelInfo:             modelInfo,
		promptEngineeringTips: tips,
	}
}

// SetVerbose は詳細出力モードを設定する
func (i *Improver) SetVerbose(verbose bool, out io.Writer) {
	i.verbose = verbose
	i.verboseOut = out
}

// verbosePrintf は詳細モード時にメッセージを出力する
func (i *Improver) verbosePrintf(format string, args ...interface{}) {
	if i.verbose && i.verboseOut != nil {
		_, _ = fmt.Fprintf(i.verboseOut, format, args...)
	}
}

// ImprovePrompt は評価結果を基にプロンプトを改善する
func (i *Improver) ImprovePrompt(ctx context.Context, promptType PromptType, currentPrompt string, evaluation *EvaluationResult, samples []SampleResult) (string, error) {
	return i.ImprovePromptWithHistory(ctx, promptType, currentPrompt, evaluation, samples, nil)
}

// ImprovePromptWithHistory は失敗した改善の履歴を考慮してプロンプトを改善する
func (i *Improver) ImprovePromptWithHistory(ctx context.Context, promptType PromptType, currentPrompt string, evaluation *EvaluationResult, samples []SampleResult, failedImprovements []string) (string, error) {
	prompt := i.buildImprovementPromptWithHistory(promptType, currentPrompt, evaluation, samples, failedImprovements)

	i.verbosePrintf("\n%s\n", verboseSeparator("改善プロンプト"))
	i.verbosePrintf("%s\n", prompt)
	i.verbosePrintf("%s\n\n", verboseSeparatorEnd())

	output, err := i.provider.Summarize(ctx, prompt, "")
	if err != nil {
		return "", fmt.Errorf("failed to improve prompt: %w", err)
	}

	i.verbosePrintf("%s\n", verboseSeparator("改善レスポンス"))
	i.verbosePrintf("%s\n", output)
	i.verbosePrintf("%s\n\n", verboseSeparatorEnd())

	improved, err := i.extractImprovedPrompt(output)
	if err != nil {
		return "", err
	}

	// 出力形式テンプレートが含まれていない場合は自動追加
	return ensureOutputFormat(improved, promptType), nil
}

// buildImprovementPromptWithHistory は失敗履歴を考慮した改善用プロンプトを構築する
func (i *Improver) buildImprovementPromptWithHistory(promptType PromptType, currentPrompt string, evaluation *EvaluationResult, samples []SampleResult, failedImprovements []string) string {
	promptTypeDesc := "課題一覧用（1行50文字以内の要約）"
	constraints := `- 各課題につき1行（50文字以内）で要点をまとめる
- 出力形式は「=== 課題キー ===」の後に要約文`

	if promptType == PromptTypeIssueView {
		promptTypeDesc = "課題詳細用（3文程度の要約）"
		constraints = `- 3文程度で要約
- 背景、現状、次のアクションがわかるように
- 出力形式は「=== 課題キー ===」の後に要約文（複数行可）`
	}

	// サンプルの入出力例
	cb := "```" // code block marker
	var samplesText strings.Builder
	for idx, s := range samples {
		if idx >= 2 { // 最大2つのサンプルのみ表示
			break
		}
		samplesText.WriteString(fmt.Sprintf("### サンプル %d: %s\n", idx+1, s.IssueKey))
		samplesText.WriteString("入力（抜粋）:\n" + cb + "\n")
		samplesText.WriteString(truncateString(s.Input, 300))
		samplesText.WriteString("\n" + cb + "\n")
		samplesText.WriteString("現在の出力:\n" + cb + "\n")
		samplesText.WriteString(s.Output)
		samplesText.WriteString("\n" + cb + "\n\n")
	}

	// プロンプトエンジニアリングのヒント（設定から取得、なければデフォルト）
	tips := i.promptEngineeringTips
	if tips == "" {
		tips = `1. **明確な役割定義**: AIに具体的な役割を与える
2. **出力形式の明示**: 期待する出力形式を具体的に示す
3. **制約条件の明示**: 文字数制限や行数制限を明確に
4. **例示**: 良い出力例を示す
5. **段階的指示**: 複雑なタスクは段階的に分解
6. **品質基準**: 期待する品質を具体的に示す`
	}

	// 失敗した改善のセクションを構築
	var failedSection string
	if len(failedImprovements) > 0 {
		var failedText strings.Builder
		failedText.WriteString("## 【重要】過去に失敗した改善\n")
		failedText.WriteString("以下のプロンプト改善は比較評価で旧プロンプトに負けました。\n")
		failedText.WriteString("**これらとは異なるアプローチ**で改善を試みてください。\n\n")
		for idx, failed := range failedImprovements {
			failedText.WriteString(fmt.Sprintf("### 失敗した改善案 %d:\n", idx+1))
			failedText.WriteString(cb + "\n")
			failedText.WriteString(truncateString(failed, 500))
			failedText.WriteString("\n" + cb + "\n\n")
		}
		failedSection = failedText.String()
	}

	// 必須の出力形式セクション（改善プロンプトの末尾に必ず含める）
	requiredOutputSection := fmt.Sprintf("# 出力形式（厳守）\n%s", getOutputFormatTemplate(promptType))

	return fmt.Sprintf("あなたはプロンプトエンジニアリングの専門家です。\n\n"+
		"## タスク\n"+
		"以下の評価フィードバックを基に、AI要約プロンプトを改善してください。\n\n"+
		"## プロンプトタイプ\n%s\n\n"+
		"## 出力モデル情報\n%s\n\n"+
		"## 現在のプロンプト\n"+cb+"\n%s\n"+cb+"\n\n"+
		"## 評価結果\n"+
		"- スコア: %d/10\n"+
		"- フィードバック: %s\n"+
		"- 強み: %s\n"+
		"- 弱み: %s\n"+
		"- 改善提案: %s\n\n"+
		"%s"+ // 失敗した改善セクション（ある場合のみ）
		"## サンプル入出力\n%s\n"+
		"## 制約条件\n%s\n\n"+
		"## プロンプトエンジニアリングのヒント\n%s\n\n"+
		"## 【最重要】必須の出力形式セクション\n"+
		"改善後のプロンプトの**末尾には、必ず以下の出力形式セクションをそのまま含めてください**。\n"+
		"このセクションがないと、AIの応答をパースできず要約が認識されません。\n\n"+
		"**以下を改善プロンプトの末尾にそのままコピーしてください:**\n"+
		cb+"\n%s\n"+cb+"\n\n"+
		"## 出力形式\n"+
		"改善されたプロンプトを出力してください。\n"+
		"**必ず末尾に上記の「# 出力形式（厳守）」セクションを含めること。**\n\n"+
		"出力例:\n"+
		cb+"prompt\n"+
		"# 役割\n"+
		"あなたは...\n\n"+
		"# タスク\n"+
		"...\n\n"+
		"# 出力形式（厳守）\n"+
		"%s\n"+
		cb+"\n",
		promptTypeDesc, i.formatModelInfo(), currentPrompt,
		evaluation.Score,
		evaluation.Feedback,
		strings.Join(evaluation.Strengths, ", "),
		strings.Join(evaluation.Weaknesses, ", "),
		strings.Join(evaluation.Suggestions, ", "),
		failedSection,
		samplesText.String(),
		constraints,
		tips,
		requiredOutputSection,
		getOutputFormatTemplate(promptType))
}

// formatModelInfo は出力モデル情報をフォーマットする
func (i *Improver) formatModelInfo() string {
	args := strings.Join(i.modelInfo.Args, " ")
	return fmt.Sprintf("プロバイダー: %s\nコマンド: %s %s",
		i.modelInfo.ProviderName, i.modelInfo.Command, args)
}

// extractImprovedPrompt は出力から改善されたプロンプトを抽出する
func (i *Improver) extractImprovedPrompt(output string) (string, error) {
	// ```prompt ... ``` ブロックを検索
	promptBlockPattern := regexp.MustCompile("(?s)```prompt\\s*\\n?(.*?)\\s*```")
	if matches := promptBlockPattern.FindStringSubmatch(output); len(matches) > 1 {
		return strings.TrimSpace(matches[1]), nil
	}

	// ``` ... ``` ブロックを検索（言語指定なし）
	codeBlockPattern := regexp.MustCompile("(?s)```\\s*\\n?(.*?)\\s*```")
	if matches := codeBlockPattern.FindStringSubmatch(output); len(matches) > 1 {
		return strings.TrimSpace(matches[1]), nil
	}

	// コードブロックがない場合は出力全体を返す
	return strings.TrimSpace(output), nil
}

// getOutputFormatTemplate はプロンプトタイプに応じた出力形式テンプレートを返す
func getOutputFormatTemplate(promptType PromptType) string {
	if promptType == PromptTypeIssueView {
		return `=== {課題キー} ===
{背景を説明する文}
{現状・問題点を説明する文}
{次のアクション・解決策を説明する文}`
	}
	// PromptTypeIssueList
	return `=== {課題キー} ===
{50文字以内の要約文}`
}

// ensureOutputFormat は改善されたプロンプトに出力形式テンプレートが含まれていることを保証する
func ensureOutputFormat(prompt string, promptType PromptType) string {
	// 出力形式の区切り記号 "=== " がプロンプトに含まれているかチェック
	// この区切り記号はAIの応答をパースするために必須
	if strings.Contains(prompt, "=== ") {
		return prompt
	}

	// 出力形式テンプレートを追加
	template := getOutputFormatTemplate(promptType)
	return prompt + "\n\n# 出力形式（厳守）\n" + template
}

// ImprovePromptWithWebSearch はWeb検索でベストプラクティスを収集してから改善する
// 注: この機能は評価モデルがWeb検索機能を持っている場合に使用
func (i *Improver) ImprovePromptWithWebSearch(ctx context.Context, promptType PromptType, currentPrompt string, evaluation *EvaluationResult, samples []SampleResult) (string, error) {
	return i.ImprovePromptWithWebSearchAndHistory(ctx, promptType, currentPrompt, evaluation, samples, nil)
}

// ImprovePromptWithWebSearchAndHistory はWeb検索と失敗履歴を考慮してプロンプトを改善する
func (i *Improver) ImprovePromptWithWebSearchAndHistory(ctx context.Context, promptType PromptType, currentPrompt string, evaluation *EvaluationResult, samples []SampleResult, failedImprovements []string) (string, error) {
	prompt := i.buildImprovementPromptWithWebSearchAndHistory(promptType, currentPrompt, evaluation, samples, failedImprovements)

	i.verbosePrintf("\n%s\n", verboseSeparator("改善プロンプト (Web検索あり)"))
	i.verbosePrintf("%s\n", prompt)
	i.verbosePrintf("%s\n\n", verboseSeparatorEnd())

	output, err := i.provider.Summarize(ctx, prompt, "")
	if err != nil {
		return "", fmt.Errorf("failed to improve prompt with web search: %w", err)
	}

	i.verbosePrintf("%s\n", verboseSeparator("改善レスポンス (Web検索あり)"))
	i.verbosePrintf("%s\n", output)
	i.verbosePrintf("%s\n\n", verboseSeparatorEnd())

	improved, err := i.extractImprovedPrompt(output)
	if err != nil {
		return "", err
	}

	// 出力形式テンプレートが含まれていない場合は自動追加
	return ensureOutputFormat(improved, promptType), nil
}

// buildImprovementPromptWithWebSearchAndHistory はWeb検索と失敗履歴を含む改善プロンプトを構築する
func (i *Improver) buildImprovementPromptWithWebSearchAndHistory(promptType PromptType, currentPrompt string, evaluation *EvaluationResult, samples []SampleResult, failedImprovements []string) string {
	basePrompt := i.buildImprovementPromptWithHistory(promptType, currentPrompt, evaluation, samples, failedImprovements)

	bestPracticesInstructions := `
## ベストプラクティスの活用
改善プロンプトを作成する際は、あなたの知識・Web検索・公式ドキュメント等、あらゆる手段を駆使して最新のベストプラクティスを収集・活用してください：

- プロンプトエンジニアリングの最新手法
- 出力モデル固有のプロンプトガイドライン
- 要約タスクに効果的なプロンプト設計パターン

収集した情報を基に、より効果的なプロンプトを作成してください。
`

	return bestPracticesInstructions + "\n" + basePrompt
}
