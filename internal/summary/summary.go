package summary

import (
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/ramenjuniti/lexrankmmr"
)

var (
	// Markdownの見出し、リスト、引用などの記号を除去するための正規表現
	// 行頭の #, *, -, > を削除
	reMarkdownHead = regexp.MustCompile(`(?m)^[\s#\*\->]+`)

	// 強調記号 **, __, ~~ を削除
	reMarkdownDeco = regexp.MustCompile(`[\*~_]{2,}`)

	// URLを除去
	reURL = regexp.MustCompile(`https?://[\w!?/+\-_~=;.,*&@#$%()'[\\\]]+`)

	// コードブロック（```...```）の除去は難しいが、``` だけの行は削除したい
	reCodeFence = regexp.MustCompile("`{3,}`")

	// 記号のみの行判定（英数字・ひらがな・カタカナ・漢字が含まれていない）
	reSymbolOnly = regexp.MustCompile(`^[[:punct:][:space:]]*$`)
)

// Summarize summarises the given text into `sentenceCount` sentences.
func Summarize(text string, sentenceCount int) (string, error) {
	if strings.TrimSpace(text) == "" {
		return "", nil
	}

	// テキストの正規化とクリーニング
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
	// 1. URL除去
	text = reURL.ReplaceAllString(text, "")

	// 2. コードフェンス除去
	text = reCodeFence.ReplaceAllString(text, "")

	// 3. Markdown装飾除去 (行頭)
	text = reMarkdownHead.ReplaceAllString(text, "")

	// 4. Markdown強調除去 (文中)
	text = reMarkdownDeco.ReplaceAllString(text, "")

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

	// 改行で分割し、フィルタリング
	lines := strings.Split(text, "\n")
	var validLines []string
	for _, line := range lines {
		line = strings.TrimSpace(line)

		// 空行除去
		if line == "" {
			continue
		}

		// 記号のみの行を除去
		if reSymbolOnly.MatchString(line) {
			continue
		}

		// 短すぎる行を除去（例: 5文字未満）
		// LexRankの品質向上のため、ある程度の情報量がある文のみを通す。
		if utf8.RuneCountInString(line) < 5 {
			continue
		}

		validLines = append(validLines, line)
	}

	if len(validLines) == 0 {
		return ""
	}

	// LexRankMMRへ渡すために「。」で結合し、末尾にも「。」をつける。
	return strings.Join(validLines, "。") + "。"
}
