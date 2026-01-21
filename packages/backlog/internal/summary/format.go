package summary

import (
	"bufio"
	"regexp"
	"strings"
)

// IssueInput は要約対象の課題入力データ
type IssueInput struct {
	Key         string
	Title       string
	Description string
	Comments    []string
}

// SummaryResult は要約結果
type SummaryResult struct {
	Key     string
	Summary string
}

// 区切り記号のパターン（行末のスペースを許容）
var keyPattern = regexp.MustCompile(`^===\s*([A-Z0-9_]+-\d+)\s*===\s*$`)

// FormatInput は課題リストを入力形式にフォーマットする
func FormatInput(issues []IssueInput) string {
	var sb strings.Builder

	for i, issue := range issues {
		if i > 0 {
			sb.WriteString("\n")
		}

		// 課題キー
		sb.WriteString("=== ")
		sb.WriteString(issue.Key)
		sb.WriteString(" ===\n")

		// タイトル
		if issue.Title != "" {
			sb.WriteString("タイトル: ")
			sb.WriteString(issue.Title)
			sb.WriteString("\n")
		}

		// 説明文
		if issue.Description != "" {
			sb.WriteString(issue.Description)
			sb.WriteString("\n")
		}

		// コメント
		if len(issue.Comments) > 0 {
			sb.WriteString("---コメント---\n")
			for _, comment := range issue.Comments {
				sb.WriteString(comment)
				sb.WriteString("\n")
			}
		}
	}

	return sb.String()
}

// FormatSingleInput は単一課題を入力形式にフォーマットする
func FormatSingleInput(issue IssueInput) string {
	return FormatInput([]IssueInput{issue})
}

// ParseOutput は出力を課題キー→要約のマップにパースする
func ParseOutput(output string) map[string]string {
	result := make(map[string]string)

	scanner := bufio.NewScanner(strings.NewReader(output))
	var currentKey string
	var currentSummary strings.Builder

	for scanner.Scan() {
		line := scanner.Text()

		// 新しい課題キーの開始を検出
		if matches := keyPattern.FindStringSubmatch(line); matches != nil {
			// 前の課題の要約を保存
			if currentKey != "" {
				result[currentKey] = strings.TrimSpace(currentSummary.String())
			}

			// 新しい課題キーを設定
			currentKey = matches[1]
			currentSummary.Reset()
			continue
		}

		// 現在の課題の要約に追加
		if currentKey != "" {
			if currentSummary.Len() > 0 {
				currentSummary.WriteString("\n")
			}
			currentSummary.WriteString(line)
		}
	}

	// 最後の課題の要約を保存
	if currentKey != "" {
		result[currentKey] = strings.TrimSpace(currentSummary.String())
	}

	return result
}

// ParseSingleOutput は単一課題の出力をパースする
func ParseSingleOutput(output, expectedKey string) string {
	result := ParseOutput(output)
	if summary, ok := result[expectedKey]; ok {
		return summary
	}

	// キーが見つからない場合は、出力全体を要約として扱う
	// （AIが区切り記号なしで直接要約を返した場合）
	return strings.TrimSpace(output)
}

// TruncateSummary は要約を指定文字数で切り詰める
func TruncateSummary(summary string, maxLen int) string {
	if maxLen <= 0 {
		return summary
	}

	// 改行を空白に置換（1行表示用）
	summary = strings.ReplaceAll(summary, "\n", " ")
	summary = strings.TrimSpace(summary)

	runes := []rune(summary)
	if len(runes) <= maxLen {
		return summary
	}

	return string(runes[:maxLen]) + "..."
}
