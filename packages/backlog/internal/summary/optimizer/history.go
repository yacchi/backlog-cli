package optimizer

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

const (
	// HistoryFileName は履歴ファイル名
	HistoryFileName = "optimization_history.jsonl"
	// HistoryDir はキャッシュ内のサブディレクトリ名
	HistoryDir = "prompt"
)

// HistoryStore は最適化履歴の保存・読み込みを管理する
type HistoryStore struct {
	cacheDir string
}

// NewHistoryStore は新しいHistoryStoreを作成する
func NewHistoryStore(cacheDir string) (*HistoryStore, error) {
	historyDir := filepath.Join(cacheDir, HistoryDir)
	if err := os.MkdirAll(historyDir, 0o755); err != nil {
		return nil, fmt.Errorf("failed to create history directory: %w", err)
	}
	return &HistoryStore{cacheDir: cacheDir}, nil
}

// historyFilePath は履歴ファイルのパスを返す
func (h *HistoryStore) historyFilePath() string {
	return filepath.Join(h.cacheDir, HistoryDir, HistoryFileName)
}

// AppendEntry は履歴エントリを追記する
func (h *HistoryStore) AppendEntry(entry HistoryEntry) error {
	entry.Timestamp = time.Now()

	f, err := os.OpenFile(h.historyFilePath(), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("failed to open history file: %w", err)
	}
	defer func() { _ = f.Close() }()

	data, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("failed to marshal entry: %w", err)
	}

	if _, err := f.Write(append(data, '\n')); err != nil {
		return fmt.Errorf("failed to write entry: %w", err)
	}

	return nil
}

// WriteSessionStart はセッション開始を記録する
func (h *HistoryStore) WriteSessionStart(sessionID string, promptType PromptType, initialPrompt string, config OptimizationConfig) error {
	return h.AppendEntry(HistoryEntry{
		Type:      "session_start",
		SessionID: sessionID,
		Data: SessionStartData{
			PromptType:    promptType,
			InitialPrompt: initialPrompt,
			Config:        config,
		},
	})
}

// WriteIteration は反復結果を記録する
func (h *HistoryStore) WriteIteration(sessionID string, iteration OptimizationIteration) error {
	return h.AppendEntry(HistoryEntry{
		Type:      "iteration",
		SessionID: sessionID,
		Data:      iteration,
	})
}

// WriteSessionEnd はセッション終了を記録する
func (h *HistoryStore) WriteSessionEnd(sessionID string, status SessionStatus, finalPrompt string, finalScore int, errMsg string) error {
	return h.AppendEntry(HistoryEntry{
		Type:      "session_end",
		SessionID: sessionID,
		Data: SessionEndData{
			Status:      status,
			FinalPrompt: finalPrompt,
			FinalScore:  finalScore,
			Error:       errMsg,
		},
	})
}

// ReadAllEntries は全履歴エントリを読み込む
func (h *HistoryStore) ReadAllEntries() ([]HistoryEntry, error) {
	f, err := os.Open(h.historyFilePath())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to open history file: %w", err)
	}
	defer func() { _ = f.Close() }()

	var entries []HistoryEntry
	scanner := bufio.NewScanner(f)
	// プロンプトが長い場合があるためバッファサイズを拡大（10MB）
	const maxScanTokenSize = 10 * 1024 * 1024
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, maxScanTokenSize)
	for scanner.Scan() {
		var entry HistoryEntry
		if err := json.Unmarshal(scanner.Bytes(), &entry); err != nil {
			// 不正な行はスキップ
			continue
		}
		entries = append(entries, entry)
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("failed to read history file: %w", err)
	}

	return entries, nil
}

// GetSessionHistory は指定セッションの履歴を取得する
func (h *HistoryStore) GetSessionHistory(sessionID string) ([]HistoryEntry, error) {
	allEntries, err := h.ReadAllEntries()
	if err != nil {
		return nil, err
	}

	var sessionEntries []HistoryEntry
	for _, entry := range allEntries {
		if entry.SessionID == sessionID {
			sessionEntries = append(sessionEntries, entry)
		}
	}

	return sessionEntries, nil
}

// GetRecentSessions は最近のセッション一覧を取得する
func (h *HistoryStore) GetRecentSessions(limit int) ([]OptimizationSession, error) {
	allEntries, err := h.ReadAllEntries()
	if err != nil {
		return nil, err
	}

	// セッションIDでグループ化
	sessionMap := make(map[string]*OptimizationSession)
	for _, entry := range allEntries {
		session, exists := sessionMap[entry.SessionID]
		if !exists {
			session = &OptimizationSession{
				ID:     entry.SessionID,
				Status: SessionStatusRunning,
			}
			sessionMap[entry.SessionID] = session
		}

		switch entry.Type {
		case "session_start":
			if data, ok := entry.Data.(map[string]interface{}); ok {
				session.StartedAt = entry.Timestamp
				if pt, ok := data["prompt_type"].(string); ok {
					session.PromptType = PromptType(pt)
				}
				if ip, ok := data["initial_prompt"].(string); ok {
					session.InitialPrompt = ip
				}
			}
		case "iteration":
			if data, ok := entry.Data.(map[string]interface{}); ok {
				var iter OptimizationIteration
				iterBytes, _ := json.Marshal(data)
				_ = json.Unmarshal(iterBytes, &iter)
				session.Iterations = append(session.Iterations, iter)
			}
		case "session_end":
			if data, ok := entry.Data.(map[string]interface{}); ok {
				session.CompletedAt = entry.Timestamp
				if status, ok := data["status"].(string); ok {
					session.Status = SessionStatus(status)
				}
				if fp, ok := data["final_prompt"].(string); ok {
					session.FinalPrompt = fp
				}
				if errMsg, ok := data["error"].(string); ok {
					session.Error = errMsg
				}
			}
		}
	}

	// セッションをスライスに変換し、開始時刻でソート（新しい順）
	var sessions []OptimizationSession
	for _, session := range sessionMap {
		sessions = append(sessions, *session)
	}

	// 開始時刻で降順ソート
	for i := 0; i < len(sessions)-1; i++ {
		for j := i + 1; j < len(sessions); j++ {
			if sessions[j].StartedAt.After(sessions[i].StartedAt) {
				sessions[i], sessions[j] = sessions[j], sessions[i]
			}
		}
	}

	// limitで制限
	if limit > 0 && len(sessions) > limit {
		sessions = sessions[:limit]
	}

	return sessions, nil
}

// ClearHistory は履歴を全削除する
func (h *HistoryStore) ClearHistory() error {
	path := h.historyFilePath()
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil
	}
	return os.Remove(path)
}

// GetLatestCompletedSession は指定プロンプトタイプの最新の完了セッションを取得する
func (h *HistoryStore) GetLatestCompletedSession(promptType PromptType) (*OptimizationSession, error) {
	sessions, err := h.GetRecentSessions(0) // 全セッション取得
	if err != nil {
		return nil, err
	}

	for _, session := range sessions {
		if session.PromptType == promptType && session.Status == SessionStatusCompleted && session.FinalPrompt != "" {
			return &session, nil
		}
	}

	return nil, nil
}

// GetSessionByID は指定IDのセッションを取得する
func (h *HistoryStore) GetSessionByID(sessionID string) (*OptimizationSession, error) {
	sessions, err := h.GetRecentSessions(0) // 全セッション取得
	if err != nil {
		return nil, err
	}

	for _, session := range sessions {
		if session.ID == sessionID {
			return &session, nil
		}
	}

	return nil, nil
}
