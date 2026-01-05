package ui

import (
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"
	"text/tabwriter"
	"unicode/utf8"
)

// Table はテーブル出力
type Table struct {
	headers []string
	rows    [][]string
}

// NewTable は新しいテーブルを作成する
func NewTable(headers ...string) *Table {
	return &Table{
		headers: headers,
		rows:    make([][]string, 0),
	}
}

// AddRow は行を追加する
func (t *Table) AddRow(values ...string) {
	t.rows = append(t.rows, values)
}

// Render はテーブルを出力する
func (t *Table) Render(w io.Writer) {
	if w == nil {
		w = os.Stdout
	}

	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)

	// ヘッダー
	_, _ = fmt.Fprintln(tw, strings.Join(t.headers, "\t"))

	// 行
	for _, row := range t.rows {
		_, _ = fmt.Fprintln(tw, strings.Join(row, "\t"))
	}

	_ = tw.Flush()
}

// RenderWithColor は色付きでテーブルを出力する
// ANSIエスケープシーケンスを考慮してカラム幅を揃える
func (t *Table) RenderWithColor(w io.Writer, colorEnabled bool) {
	if !colorEnabled {
		t.Render(w)
		return
	}

	if w == nil {
		w = os.Stdout
	}

	// 各カラムの最大表示幅を計算
	colWidths := t.calculateColumnWidths()

	// ヘッダー出力（太字）
	for i, h := range t.headers {
		if i > 0 {
			_, _ = fmt.Fprint(w, "  ")
		}
		_, _ = fmt.Fprint(w, padRight(Bold(h), colWidths[i], displayWidth(h)))
	}
	_, _ = fmt.Fprintln(w)

	// 行出力
	for _, row := range t.rows {
		for i, cell := range row {
			if i > 0 {
				_, _ = fmt.Fprint(w, "  ")
			}
			if i < len(colWidths) {
				_, _ = fmt.Fprint(w, padRight(cell, colWidths[i], displayWidth(cell)))
			} else {
				_, _ = fmt.Fprint(w, cell)
			}
		}
		_, _ = fmt.Fprintln(w)
	}
}

// calculateColumnWidths は各カラムの最大表示幅を計算する
func (t *Table) calculateColumnWidths() []int {
	if len(t.headers) == 0 {
		return nil
	}

	widths := make([]int, len(t.headers))

	// ヘッダーの幅
	for i, h := range t.headers {
		widths[i] = displayWidth(h)
	}

	// 各行の幅
	for _, row := range t.rows {
		for i, cell := range row {
			if i < len(widths) {
				w := displayWidth(cell)
				if w > widths[i] {
					widths[i] = w
				}
			}
		}
	}

	return widths
}

// ansiEscapeRegex はANSIエスケープシーケンスにマッチする正規表現
var ansiEscapeRegex = regexp.MustCompile(`\x1b\[[0-9;]*m`)

// osc8Regex はOSC 8ハイパーリンクのエスケープシーケンスにマッチする正規表現
// フォーマット: \x1b]8;;URL\x1b\\ (開始) または \x1b]8;;\x1b\\ (終了)
var osc8Regex = regexp.MustCompile(`\x1b\]8;;[^\x1b]*\x1b\\`)

// displayWidth はANSIエスケープシーケンスを除いた表示幅を返す
// 全角文字は幅2としてカウント
func displayWidth(s string) int {
	// OSC 8ハイパーリンクを除去
	clean := osc8Regex.ReplaceAllString(s, "")
	// ANSIエスケープシーケンスを除去
	clean = ansiEscapeRegex.ReplaceAllString(clean, "")

	width := 0
	for _, r := range clean {
		if isWideRune(r) {
			width += 2
		} else {
			width++
		}
	}
	return width
}

// isWideRune は全角文字かどうかを判定する
func isWideRune(r rune) bool {
	// CJK文字、全角記号などを全角として扱う
	// 簡易的な判定: ルーンが3バイト以上のUTF-8でエンコードされる場合
	return utf8.RuneLen(r) >= 3
}

// padRight は文字列を指定幅まで右側にスペースでパディングする
// currentWidth は現在の表示幅（ANSIエスケープシーケンスを除いた幅）
func padRight(s string, targetWidth, currentWidth int) string {
	if currentWidth >= targetWidth {
		return s
	}
	return s + strings.Repeat(" ", targetWidth-currentWidth)
}
