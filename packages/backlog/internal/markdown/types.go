package markdown

import "time"

// Mode represents detected markdown mode.
type Mode string

const (
	ModeBacklog  Mode = "backlog"
	ModeMarkdown Mode = "markdown"
	ModeUnknown  Mode = "unknown"
)

// WarningType represents a warning category.
type WarningType string

const (
	WarningColorMacro       WarningType = "color_macro"
	WarningTableHeaderH     WarningType = "table_header_h"
	WarningTableHeaderCell  WarningType = "table_header_cell"
	WarningTableCellMerge   WarningType = "table_cell_merge"
	WarningThumbnailMacro   WarningType = "thumbnail_macro"
	WarningUnknownHashMacro WarningType = "unknown_hash_macro"
	WarningUnknownBrace     WarningType = "unknown_brace_macro"
	WarningWikiLinkAmbig    WarningType = "wiki_link_ambiguous"
	WarningEmphasisAmbig    WarningType = "emphasis_ambiguous"
)

// RuleID represents a conversion rule identifier.
type RuleID string

const (
	RuleHeadingAsterisk RuleID = "heading_asterisk"
	RuleQuoteBlock      RuleID = "quote_block"
	RuleCodeBlock       RuleID = "code_block"
	RuleEmphasisBold    RuleID = "emphasis_bold"
	RuleEmphasisItalic  RuleID = "emphasis_italic"
	RuleStrikethrough   RuleID = "strikethrough"
	RuleBacklogLink     RuleID = "backlog_link"
	RuleTOC             RuleID = "toc"
	RuleLineBreak       RuleID = "line_break"
	RuleListPlus        RuleID = "list_plus"
	RuleListDashSpace   RuleID = "list_dash_space"
	RuleTableSeparator  RuleID = "table_separator"
	RuleImageMacro      RuleID = "image_macro"
)

// DetectResult represents detection output.
type DetectResult struct {
	Mode  Mode
	Score int
}

// ConvertOptions controls conversion behavior.
type ConvertOptions struct {
	Force           bool
	LineBreak       string
	WarnOnly        bool
	ItemType        string
	ItemID          int
	ParentID        int
	ProjectKey      string
	ItemKey         string
	URL             string
	AttachmentNames []string
	UnsafeRules     map[RuleID]bool
}

// ConvertResult represents conversion output.
type ConvertResult struct {
	Output       string
	Mode         Mode
	Score        int
	Warnings     map[WarningType]int
	WarningLines map[WarningType][]int
	Rules        []RuleID
	ItemType     string
	ItemID       int
	ParentID     int
	ProjectKey   string
	ItemKey      string
	URL          string
}

// CacheEntry represents a JSONL cache record.
type CacheEntry struct {
	TS            time.Time             `json:"ts"`
	ItemType      string                `json:"item_type"`
	ItemID        int                   `json:"item_id"`
	ParentID      int                   `json:"parent_id,omitempty"`
	ProjectKey    string                `json:"project_key"`
	ItemKey       string                `json:"item_key,omitempty"`
	URL           string                `json:"url,omitempty"`
	DetectedMode  Mode                  `json:"detected_mode"`
	Score         int                   `json:"score"`
	Warnings      map[WarningType]int   `json:"warnings"`
	WarningLines  map[WarningType][]int `json:"warning_lines,omitempty"`
	RulesApplied  []RuleID              `json:"rules_applied"`
	InputHash     string                `json:"input_hash"`
	OutputHash    string                `json:"output_hash"`
	InputExcerpt  string                `json:"input_excerpt"`
	OutputExcerpt string                `json:"output_excerpt"`
	InputRaw      string                `json:"input_raw,omitempty"`
	OutputRaw     string                `json:"output_raw,omitempty"`
}
