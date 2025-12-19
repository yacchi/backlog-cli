package ui

import (
	"time"

	"github.com/yacchi/backlog-cli/internal/config"
)

// FieldFormatter はフィールド値のフォーマットを行う共通ユーティリティ
type FieldFormatter struct {
	Timezone       string
	DateTimeFormat string
	FieldConfig    map[string]config.ResolvedFieldConfig
	location       *time.Location
}

// NewFieldFormatter は新しいFieldFormatterを作成する
func NewFieldFormatter(timezone, dateTimeFormat string, fieldConfig map[string]config.ResolvedFieldConfig) *FieldFormatter {
	return &FieldFormatter{
		Timezone:       timezone,
		DateTimeFormat: dateTimeFormat,
		FieldConfig:    fieldConfig,
	}
}

// getLocation はタイムゾーンのLocationを取得する（キャッシュ付き）
func (f *FieldFormatter) getLocation() *time.Location {
	if f.location != nil {
		return f.location
	}
	if f.Timezone != "" {
		if loc, err := time.LoadLocation(f.Timezone); err == nil {
			f.location = loc
			return f.location
		}
	}
	f.location = time.Local
	return f.location
}

// FormatString はフィールドの値をフォーマットする（truncate等）
func (f *FieldFormatter) FormatString(s, field string) string {
	cfg, ok := f.FieldConfig[field]
	if !ok {
		return s
	}
	// truncate
	if cfg.MaxWidth > 0 {
		s = Truncate(s, cfg.MaxWidth)
	}
	return s
}

// FormatDateTime は日時文字列をフォーマットする
func (f *FieldFormatter) FormatDateTime(dateStr, field string) string {
	if dateStr == "" {
		return "-"
	}

	// フィールド固有のフォーマットを確認
	format := f.DateTimeFormat
	if cfg, ok := f.FieldConfig[field]; ok && cfg.TimeFormat != "" {
		format = cfg.TimeFormat
	}

	// ISO 8601形式をパース
	t, err := time.Parse(time.RFC3339, dateStr)
	if err != nil {
		// パースに失敗した場合は元の文字列を返す
		return dateStr
	}

	// タイムゾーンを適用
	t = t.In(f.getLocation())
	return t.Format(format)
}

// FormatDate は日付文字列をフォーマットする（日付のみ、時刻なし）
func (f *FieldFormatter) FormatDate(dateStr, field string) string {
	if dateStr == "" {
		return "-"
	}

	// フィールド固有のフォーマットを確認（デフォルトは2006-01-02）
	format := "2006-01-02"
	if cfg, ok := f.FieldConfig[field]; ok && cfg.TimeFormat != "" {
		format = cfg.TimeFormat
	}

	// ISO 8601形式をパース（日時形式）
	t, err := time.Parse(time.RFC3339, dateStr)
	if err != nil {
		// 日付のみの形式も試す
		t, err = time.Parse("2006-01-02", dateStr)
		if err != nil {
			// パースに失敗した場合は元の文字列を返す
			return dateStr
		}
	}

	// タイムゾーンを適用
	t = t.In(f.getLocation())
	return t.Format(format)
}

// Truncate は文字列を指定した最大幅で切り詰める
func Truncate(s string, max int) string {
	if max <= 0 {
		return s
	}
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	if max <= 3 {
		return string(runes[:max])
	}
	return string(runes[:max-3]) + "..."
}
