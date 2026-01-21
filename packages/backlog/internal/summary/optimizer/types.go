// Package optimizer はAI要約プロンプトの自動最適化機能を提供する
package optimizer

import (
	"time"
)

// PromptType はプロンプトの種類を表す
type PromptType string

const (
	// PromptTypeIssueList は課題一覧用プロンプト
	PromptTypeIssueList PromptType = "issue_list"
	// PromptTypeIssueView は課題詳細用プロンプト
	PromptTypeIssueView PromptType = "issue_view"
)

// SessionStatus は最適化セッションの状態を表す
type SessionStatus string

const (
	// SessionStatusRunning は実行中
	SessionStatusRunning SessionStatus = "running"
	// SessionStatusCompleted は正常完了
	SessionStatusCompleted SessionStatus = "completed"
	// SessionStatusFailed は失敗
	SessionStatusFailed SessionStatus = "failed"
)

// OptimizationSession は1回の最適化実行を表す
type OptimizationSession struct {
	ID            string                  `json:"id"`
	StartedAt     time.Time               `json:"started_at"`
	CompletedAt   time.Time               `json:"completed_at,omitempty"`
	PromptType    PromptType              `json:"prompt_type"`
	InitialPrompt string                  `json:"initial_prompt"`
	FinalPrompt   string                  `json:"final_prompt,omitempty"`
	Iterations    []OptimizationIteration `json:"iterations"`
	Status        SessionStatus           `json:"status"`
	Error         string                  `json:"error,omitempty"`
}

// OptimizationIteration は最適化ループの1反復を表す
type OptimizationIteration struct {
	Number         int               `json:"number"`
	Timestamp      time.Time         `json:"timestamp"`
	CurrentPrompt  string            `json:"current_prompt"`
	SampleResults  []SampleResult    `json:"sample_results"`
	Evaluation     EvaluationResult  `json:"evaluation"`
	ImprovedPrompt string            `json:"improved_prompt,omitempty"`
	Comparison     *ComparisonResult `json:"comparison,omitempty"`
	SelectedPrompt string            `json:"selected_prompt"` // "current" or "improved"
}

// SampleResult はサンプル課題の要約結果を表す
type SampleResult struct {
	IssueKey    string        `json:"issue_key"`
	IssueType   string        `json:"issue_type,omitempty"`
	Input       string        `json:"input"`
	Output      string        `json:"output"`
	ElapsedTime time.Duration `json:"elapsed_time"`
}

// EvaluationResult は評価モデルによる評価結果を表す
type EvaluationResult struct {
	Score       int      `json:"score"` // 1-10
	Feedback    string   `json:"feedback"`
	Strengths   []string `json:"strengths"`
	Weaknesses  []string `json:"weaknesses"`
	Suggestions []string `json:"suggestions"`
}

// ComparisonResult は新旧プロンプトの比較結果を表す
type ComparisonResult struct {
	OldPromptScore int    `json:"old_prompt_score"`
	NewPromptScore int    `json:"new_prompt_score"`
	Winner         string `json:"winner"` // "old" or "new"
	Reasoning      string `json:"reasoning"`
}

// SelectedIssue は評価モデルが選定した課題を表す
type SelectedIssue struct {
	IssueKey        string `json:"issue_key"`
	IssueType       string `json:"issue_type"`
	Description     string `json:"description"`
	CommentCount    int    `json:"comment_count"`
	SelectionReason string `json:"selection_reason"`
}

// IssueSelectionResult は課題選定の結果を表す
type IssueSelectionResult struct {
	SelectedIssues    []SelectedIssue `json:"selected_issues"`
	TotalCandidates   int             `json:"total_candidates"`
	SelectionCriteria string          `json:"selection_criteria"`
}

// OutputModelInfo は出力モデルの情報を表す
type OutputModelInfo struct {
	ProviderName string   `json:"provider_name"`
	Command      string   `json:"command"`
	Args         []string `json:"args"`
}

// OptimizationConfig は最適化の設定を表す
type OptimizationConfig struct {
	EvaluationProvider    string
	ScoreThreshold        int
	MaxIterations         int
	CandidateCount        int // 候補課題の取得件数
	SampleCount           int // 実際に評価に使用する課題数
	TargetProjects        []string
	OutputModelContext    string
	PromptEngineeringTips string
	OutputModel           OutputModelInfo
	Verbose               bool // 詳細表示モード
}

// HistoryEntry はJSONL履歴の1エントリを表す
type HistoryEntry struct {
	Type      string      `json:"type"` // "session_start", "iteration", "session_end"
	SessionID string      `json:"session_id"`
	Timestamp time.Time   `json:"timestamp"`
	Data      interface{} `json:"data,omitempty"`
}

// SessionStartData はセッション開始時のデータ
type SessionStartData struct {
	PromptType    PromptType         `json:"prompt_type"`
	InitialPrompt string             `json:"initial_prompt"`
	Config        OptimizationConfig `json:"config"`
}

// SessionEndData はセッション終了時のデータ
type SessionEndData struct {
	Status      SessionStatus `json:"status"`
	FinalPrompt string        `json:"final_prompt,omitempty"`
	FinalScore  int           `json:"final_score,omitempty"`
	Error       string        `json:"error,omitempty"`
}
