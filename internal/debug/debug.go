package debug

import (
	"log/slog"
	"os"
	"sync"
)

var (
	enabled bool
	mu      sync.RWMutex
	logger  *slog.Logger
)

func init() {
	// デフォルトは無効
	logger = slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))
}

// Enable はデバッグモードを有効化する
func Enable() {
	mu.Lock()
	defer mu.Unlock()
	enabled = true
}

// Disable はデバッグモードを無効化する
func Disable() {
	mu.Lock()
	defer mu.Unlock()
	enabled = false
}

// IsEnabled はデバッグモードが有効かどうかを返す
func IsEnabled() bool {
	mu.RLock()
	defer mu.RUnlock()
	return enabled
}

// Log はデバッグモード時にログを出力する
func Log(msg string, args ...any) {
	if !IsEnabled() {
		return
	}
	logger.Debug(msg, args...)
}

// Logf はデバッグモード時にフォーマットされたログを出力する
func Logf(format string, args ...any) {
	if !IsEnabled() {
		return
	}
	logger.Debug(format, args...)
}
