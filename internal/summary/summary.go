package summary

import (
	"fmt"
	"strings"

	"github.com/ramenjuniti/lexrankmmr"
)

// Summarize summarises the given text into `sentenceCount` sentences.
func Summarize(text string, sentenceCount int) (string, error) {
	if strings.TrimSpace(text) == "" {
		return "", nil
	}

	// テキストの正規化とクリーニング
	// LexRankMMRが空の文（TF-IDFベクトルがゼロ）に遭遇するとエラーになるため、
	// 事前に確実に空文を除去し、整形して渡す必要がある。
	cleanText := normalizeText(text)
	if cleanText == "" {
		return "", nil
	}

	// LexRankMMRの初期化
	options := []lexrankmmr.Option{
		lexrankmmr.MaxLines(sentenceCount),
		lexrankmmr.MaxCharacters(100000), 
	}

	data, err := lexrankmmr.New(options...)
	if err != nil {
		return "", fmt.Errorf("failed to initialize lexrankmmr: %w", err)
	}

	// 要約実行
	if err := data.Summarize(cleanText); err != nil {
		return "", fmt.Errorf("summarization failed: %w", err)
	}

	// 結果取得
	var summaries []string
	for _, score := range data.LineLimitedSummary {
		summaries = append(summaries, score.Sentence)
	}

	if len(summaries) == 0 {
		return "", nil
	}

	return strings.Join(summaries, ""), nil
}

func normalizeText(text string) string {
	// 改行や区切り文字を統一的な区切り（改行）に置換
	replacer := strings.NewReplacer(
		"\r\n", "\n",
		"\r", "\n",
		"。", "\n",
		"！", "\n",
		"？", "\n",
		"!", "\n",
		"?", "\n",
	)
	text = replacer.Replace(text)

	// 改行で分割し、空行や空白のみの行を除去
	lines := strings.Split(text, "\n")
	var validLines []string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" {
			validLines = append(validLines, line)
		}
	}

	if len(validLines) == 0 {
		return ""
	}

	// LexRankMMRへ渡すために「。」で結合し、末尾にも「。」をつける。
	// LexRankMMRは内部で「。」を区切り文字に置換し、Splitした最後の要素（空文字）を捨てる仕様のため、
	// これで正しく文分割されるはず。
	return strings.Join(validLines, "。" ) + "。"
}
