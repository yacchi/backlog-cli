package relay

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/yacchi/backlog-cli/internal/config"
)

// AuditEvent は監査イベント
type AuditEvent struct {
	Timestamp time.Time `json:"timestamp"`
	SessionID string    `json:"session_id,omitempty"` // 認証フロー全体を紐付けるID
	Action    string    `json:"action"`
	UserID    string    `json:"user_id,omitempty"`
	UserName  string    `json:"user_name,omitempty"`
	UserEmail string    `json:"user_email,omitempty"`
	Space     string    `json:"space"`
	Domain    string    `json:"domain"`
	Project   string    `json:"project,omitempty"`
	ClientIP  string    `json:"client_ip"`
	UserAgent string    `json:"user_agent"`
	Result    string    `json:"result"` // success, error
	Error     string    `json:"error,omitempty"`
}

// sessionIDLength はセッションIDとして使用するstateの先頭文字数
const sessionIDLength = 12

// ExtractSessionID はstateからセッションIDを抽出する
func ExtractSessionID(state string) string {
	if len(state) >= sessionIDLength {
		return state[:sessionIDLength]
	}
	return state
}

// AuditAction は監査アクション
const (
	AuditActionAuthStart     = "auth_start"
	AuditActionAuthCallback  = "auth_callback"
	AuditActionTokenExchange = "token_exchange"
	AuditActionTokenRefresh  = "token_refresh"
	AuditActionAccessDenied  = "access_denied"
)

// AuditLogger は監査ログ出力
type AuditLogger struct {
	enabled    bool
	output     string
	filePath   string
	webhookURL string

	file   *os.File
	mu     sync.Mutex
	client *http.Client
}

// NewAuditLogger は新しい監査ロガーを作成する
func NewAuditLogger(cfg *config.Store) (*AuditLogger, error) {
	server := cfg.Server()
	al := &AuditLogger{
		enabled:    server.AuditEnabled,
		output:     server.AuditOutput,
		filePath:   server.AuditFilePath,
		webhookURL: server.AuditWebhookURL,
		client: &http.Client{
			Timeout: time.Duration(server.AuditWebhookTimeout) * time.Second,
		},
	}

	if !al.enabled {
		return al, nil
	}

	// ファイル出力の場合はファイルを開く
	if al.output == "file" && al.filePath != "" {
		f, err := os.OpenFile(al.filePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			return nil, fmt.Errorf("failed to open audit log file: %w", err)
		}
		al.file = f
	}

	return al, nil
}

// Log は監査イベントを記録する
func (al *AuditLogger) Log(event AuditEvent) {
	if !al.enabled {
		return
	}

	event.Timestamp = time.Now().UTC()

	switch al.output {
	case "stdout":
		al.logToStdout(event)
	case "stderr":
		al.logToStderr(event)
	case "file":
		al.logToFile(event)
	case "webhook":
		go al.logToWebhook(event) // 非同期
	}
}

func (al *AuditLogger) logToStdout(event AuditEvent) {
	data, _ := json.Marshal(event)
	fmt.Println(string(data))
}

func (al *AuditLogger) logToStderr(event AuditEvent) {
	data, _ := json.Marshal(event)
	fmt.Fprintln(os.Stderr, string(data))
}

func (al *AuditLogger) logToFile(event AuditEvent) {
	if al.file == nil {
		return
	}

	al.mu.Lock()
	defer al.mu.Unlock()

	data, _ := json.Marshal(event)
	al.file.Write(data)
	al.file.WriteString("\n")
}

func (al *AuditLogger) logToWebhook(event AuditEvent) {
	if al.webhookURL == "" {
		return
	}

	// Slack形式のペイロード
	payload := al.buildSlackPayload(event)
	data, _ := json.Marshal(payload)

	resp, err := al.client.Post(al.webhookURL, "application/json", bytes.NewReader(data))
	if err != nil {
		slog.Error("failed to send audit log to webhook", "error", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		slog.Error("webhook returned error", "status", resp.StatusCode, "body", string(body))
	}
}

func (al *AuditLogger) buildSlackPayload(event AuditEvent) map[string]interface{} {
	color := "good"
	if event.Result == "error" {
		color = "danger"
	}

	title := fmt.Sprintf("Backlog CLI: %s", event.Action)

	fields := []map[string]interface{}{
		{"title": "Space", "value": event.Space + "." + event.Domain, "short": true},
		{"title": "Result", "value": event.Result, "short": true},
	}

	if event.SessionID != "" {
		fields = append(fields, map[string]interface{}{
			"title": "Session",
			"value": event.SessionID,
			"short": true,
		})
	}

	if event.UserName != "" {
		fields = append(fields, map[string]interface{}{
			"title": "User",
			"value": event.UserName,
			"short": true,
		})
	}

	if event.Project != "" {
		fields = append(fields, map[string]interface{}{
			"title": "Project",
			"value": event.Project,
			"short": true,
		})
	}

	if event.ClientIP != "" {
		fields = append(fields, map[string]interface{}{
			"title": "IP",
			"value": event.ClientIP,
			"short": true,
		})
	}

	if event.Error != "" {
		fields = append(fields, map[string]interface{}{
			"title": "Error",
			"value": event.Error,
			"short": false,
		})
	}

	return map[string]interface{}{
		"text": title,
		"attachments": []map[string]interface{}{
			{
				"color":  color,
				"fields": fields,
				"ts":     event.Timestamp.Unix(),
			},
		},
	}
}

// Close はリソースを解放する
func (al *AuditLogger) Close() error {
	if al.file != nil {
		return al.file.Close()
	}
	return nil
}
