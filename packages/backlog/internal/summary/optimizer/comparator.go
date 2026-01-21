package optimizer

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/yacchi/backlog-cli/packages/backlog/internal/summary"
)

// Comparator はプロンプト比較機能を提供する
type Comparator struct {
	provider   summary.Provider
	modelInfo  OutputModelInfo
	verbose    bool
	verboseOut io.Writer
}

// NewComparator は新しいComparatorを作成する
func NewComparator(provider summary.Provider, modelInfo OutputModelInfo) *Comparator {
	return &Comparator{
		provider:  provider,
		modelInfo: modelInfo,
	}
}

// SetVerbose は詳細出力モードを設定する
func (c *Comparator) SetVerbose(verbose bool, out io.Writer) {
	c.verbose = verbose
	c.verboseOut = out
}

// verbosePrintf は詳細モード時にメッセージを出力する
func (c *Comparator) verbosePrintf(format string, args ...interface{}) {
	if c.verbose && c.verboseOut != nil {
		_, _ = fmt.Fprintf(c.verboseOut, format, args...)
	}
}

// CompareResults は旧プロンプトと新プロンプトの結果を比較する
func (c *Comparator) CompareResults(ctx context.Context, promptType PromptType, oldPrompt string, oldResults []SampleResult, newPrompt string, newResults []SampleResult) (*ComparisonResult, error) {
	prompt := c.buildComparisonPrompt(promptType, oldPrompt, oldResults, newPrompt, newResults)

	c.verbosePrintf("\n%s\n", verboseSeparator("比較プロンプト"))
	c.verbosePrintf("%s\n", prompt)
	c.verbosePrintf("%s\n\n", verboseSeparatorEnd())

	output, err := c.provider.Summarize(ctx, prompt, "")
	if err != nil {
		return nil, fmt.Errorf("failed to compare results: %w", err)
	}

	c.verbosePrintf("%s\n", verboseSeparator("比較レスポンス"))
	c.verbosePrintf("%s\n", output)
	c.verbosePrintf("%s\n\n", verboseSeparatorEnd())

	return c.parseComparisonResult(output)
}

// buildComparisonPrompt は比較用のプロンプトを構築する
func (c *Comparator) buildComparisonPrompt(promptType PromptType, oldPrompt string, oldResults []SampleResult, newPrompt string, newResults []SampleResult) string {
	promptTypeDesc := "課題一覧用（1行50文字以内の要約）"
	if promptType == PromptTypeIssueView {
		promptTypeDesc = "課題詳細用（3文程度の要約）"
	}

	cb := "```" // code block marker

	// 結果を並べて表示
	var comparisonText strings.Builder
	for i := 0; i < len(oldResults) && i < len(newResults); i++ {
		comparisonText.WriteString(fmt.Sprintf("### %s\n", oldResults[i].IssueKey))
		comparisonText.WriteString("**入力（抜粋）:**\n" + cb + "\n")
		comparisonText.WriteString(truncateString(oldResults[i].Input, 300))
		comparisonText.WriteString("\n" + cb + "\n\n")
		comparisonText.WriteString("**プロンプトAの出力:**\n" + cb + "\n")
		comparisonText.WriteString(oldResults[i].Output)
		comparisonText.WriteString("\n" + cb + "\n\n")
		comparisonText.WriteString("**プロンプトBの出力:**\n" + cb + "\n")
		comparisonText.WriteString(newResults[i].Output)
		comparisonText.WriteString("\n" + cb + "\n\n---\n\n")
	}

	return fmt.Sprintf("あなたはAI要約の品質を評価する専門家です。\n\n"+
		"## タスク\n"+
		"2つのプロンプト（AとB）で生成された要約を比較し、どちらが優れているか判定してください。\n\n"+
		"## プロンプトタイプ\n%s\n\n"+
		"## 出力モデル情報\n%s\n\n"+
		"## プロンプトA（現在）\n"+cb+"\n%s\n"+cb+"\n\n"+
		"## プロンプトB（改善版）\n"+cb+"\n%s\n"+cb+"\n\n"+
		"## 比較対象\n%s\n"+
		"## 評価基準\n"+
		"1. **正確性**: 課題の要点を正確に捉えているか\n"+
		"2. **簡潔性**: 適切な長さで簡潔にまとまっているか\n"+
		"3. **明瞭性**: 読みやすく理解しやすいか\n"+
		"4. **一貫性**: フォーマットが一貫しているか\n"+
		"5. **有用性**: ユーザーにとって有用な情報を含んでいるか\n\n"+
		"## 出力形式\n"+
		"以下のJSON形式で出力してください：\n"+
		"{\n"+
		"  \"old_prompt_score\": <プロンプトAのスコア 1-10>,\n"+
		"  \"new_prompt_score\": <プロンプトBのスコア 1-10>,\n"+
		"  \"winner\": \"<old または new>\",\n"+
		"  \"reasoning\": \"<判定理由の詳細説明>\"\n"+
		"}\n",
		promptTypeDesc, c.formatModelInfo(), oldPrompt, newPrompt, comparisonText.String())
}

// formatModelInfo は出力モデル情報をフォーマットする
func (c *Comparator) formatModelInfo() string {
	args := strings.Join(c.modelInfo.Args, " ")
	return fmt.Sprintf("プロバイダー: %s\nコマンド: %s %s",
		c.modelInfo.ProviderName, c.modelInfo.Command, args)
}

// parseComparisonResult は比較結果をパースする
func (c *Comparator) parseComparisonResult(output string) (*ComparisonResult, error) {
	jsonStr := extractJSON(output)
	if jsonStr == "" {
		return nil, fmt.Errorf("no JSON found in output: %s", output)
	}

	var result ComparisonResult
	if err := json.Unmarshal([]byte(jsonStr), &result); err != nil {
		return nil, fmt.Errorf("failed to parse comparison result: %w", err)
	}

	// winner の正規化
	result.Winner = strings.ToLower(result.Winner)
	if result.Winner != "old" && result.Winner != "new" {
		// スコアで判定
		if result.NewPromptScore > result.OldPromptScore {
			result.Winner = "new"
		} else {
			result.Winner = "old"
		}
	}

	return &result, nil
}
