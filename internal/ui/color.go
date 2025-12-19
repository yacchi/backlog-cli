package ui

import (
	"fmt"
	"os"

	"golang.org/x/term"
)

var colorEnabled = true
var hyperlinkEnabled = false

func init() {
	// 色が使えるかチェック
	colorEnabled = term.IsTerminal(int(os.Stdout.Fd()))
}

// SetColorEnabled は色の有効/無効を設定する
func SetColorEnabled(enabled bool) {
	colorEnabled = enabled
}

// IsColorEnabled は色が有効かどうかを返す
func IsColorEnabled() bool {
	return colorEnabled
}

// SetHyperlinkEnabled はハイパーリンクの有効/無効を設定する
func SetHyperlinkEnabled(enabled bool) {
	hyperlinkEnabled = enabled
}

// IsHyperlinkEnabled はハイパーリンクが有効かどうかを返す
func IsHyperlinkEnabled() bool {
	return hyperlinkEnabled && colorEnabled
}

const (
	reset  = "\033[0m"
	bold   = "\033[1m"
	red    = "\033[31m"
	green  = "\033[32m"
	yellow = "\033[33m"
	blue   = "\033[34m"
	cyan   = "\033[36m"
	gray   = "\033[90m"
)

// Bold は太字にする
func Bold(s string) string {
	if !colorEnabled {
		return s
	}
	return bold + s + reset
}

// Red は赤色にする
func Red(s string) string {
	if !colorEnabled {
		return s
	}
	return red + s + reset
}

// Green は緑色にする
func Green(s string) string {
	if !colorEnabled {
		return s
	}
	return green + s + reset
}

// Yellow は黄色にする
func Yellow(s string) string {
	if !colorEnabled {
		return s
	}
	return yellow + s + reset
}

// Blue は青色にする
func Blue(s string) string {
	if !colorEnabled {
		return s
	}
	return blue + s + reset
}

// Cyan はシアン色にする
func Cyan(s string) string {
	if !colorEnabled {
		return s
	}
	return cyan + s + reset
}

// Gray はグレーにする
func Gray(s string) string {
	if !colorEnabled {
		return s
	}
	return gray + s + reset
}

// StatusColor はステータスに応じた色を返す
func StatusColor(status string) string {
	switch status {
	case "完了", "Closed", "Done":
		return Green(status)
	case "処理中", "In Progress":
		return Blue(status)
	case "未対応", "Open":
		return Yellow(status)
	default:
		return status
	}
}

// PriorityColor は優先度に応じた色を返す
func PriorityColor(priority string) string {
	switch priority {
	case "高", "High":
		return Red(priority)
	case "中", "Normal":
		return Yellow(priority)
	case "低", "Low":
		return Gray(priority)
	default:
		return priority
	}
}

// Success は成功メッセージを出力する
func Success(format string, args ...interface{}) {
	fmt.Printf(Green("✓ ")+format+"\n", args...)
}

// Error はエラーメッセージを出力する
func Error(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, Red("✗ ")+format+"\n", args...)
}

// Warning は警告メッセージを出力する
func Warning(format string, args ...interface{}) {
	fmt.Printf(Yellow("! ")+format+"\n", args...)
}

// Info は情報メッセージを出力する
func Info(format string, args ...interface{}) {
	fmt.Printf(Blue("ℹ ")+format+"\n", args...)
}

// Hyperlink はターミナルハイパーリンク（OSC 8）を生成する
// 対応ターミナル: iTerm2, Windows Terminal, GNOME Terminal (3.26+), Konsole (18.07+), foot など
// フォーマット: \e]8;;URL\e\\LABEL\e]8;;\e\\
func Hyperlink(url, label string) string {
	if !IsHyperlinkEnabled() {
		return label
	}
	return fmt.Sprintf("\033]8;;%s\033\\%s\033]8;;\033\\", url, label)
}
