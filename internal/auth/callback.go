package auth

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"sync"

	"github.com/yacchi/backlog-cli/internal/debug"
)

// CallbackResult はコールバックの結果
type CallbackResult struct {
	Code  string
	Error error
}

// CallbackServer はCLIのローカルコールバックサーバー
type CallbackServer struct {
	port     int
	server   *http.Server
	result   chan CallbackResult
	listener net.Listener
	once     sync.Once
}

// NewCallbackServer は新しいコールバックサーバーを作成する
func NewCallbackServer(port int) (*CallbackServer, error) {
	// ポートが0の場合は空きポートを探す
	addr := fmt.Sprintf("127.0.0.1:%d", port)
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("failed to listen on %s: %w", addr, err)
	}

	// 実際のポートを取得
	actualPort := listener.Addr().(*net.TCPAddr).Port

	cs := &CallbackServer{
		port:     actualPort,
		result:   make(chan CallbackResult, 1),
		listener: listener,
	}

	debug.Log("callback server created", "port", actualPort, "address", addr)

	mux := http.NewServeMux()
	mux.HandleFunc("/callback", cs.handleCallback)

	cs.server = &http.Server{
		Handler: mux,
	}

	return cs, nil
}

// Port は実際のポート番号を返す
func (cs *CallbackServer) Port() int {
	return cs.port
}

// Start はサーバーを起動する
func (cs *CallbackServer) Start() error {
	debug.Log("callback server starting", "port", cs.port)
	return cs.server.Serve(cs.listener)
}

// Wait はコールバックを待機する
func (cs *CallbackServer) Wait() CallbackResult {
	return <-cs.result
}

// Shutdown はサーバーを停止する
func (cs *CallbackServer) Shutdown(ctx context.Context) error {
	return cs.server.Shutdown(ctx)
}

func (cs *CallbackServer) handleCallback(w http.ResponseWriter, r *http.Request) {
	debug.Log("callback received", "method", r.Method, "path", r.URL.Path, "query", r.URL.RawQuery)

	cs.once.Do(func() {
		code := r.URL.Query().Get("code")
		errorParam := r.URL.Query().Get("error")

		if errorParam != "" {
			errorDesc := r.URL.Query().Get("error_description")
			debug.Log("callback error received", "error", errorParam, "description", errorDesc)
			cs.result <- CallbackResult{
				Error: fmt.Errorf("%s: %s", errorParam, errorDesc),
			}
		} else if code == "" {
			debug.Log("callback received without code")
			cs.result <- CallbackResult{
				Error: fmt.Errorf("no code received"),
			}
		} else {
			debug.Log("callback code received", "code_length", len(code))
			cs.result <- CallbackResult{Code: code}
		}
	})

	// 成功ページを表示
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprint(w, `<!DOCTYPE html>
<html>
<head><title>Authentication Successful</title></head>
<body>
<h1>Authentication Successful</h1>
<p>You can close this window and return to the terminal.</p>
<script>window.close();</script>
</body>
</html>`)
}

// FindFreePort は空いているポートを探す
func FindFreePort() (int, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer listener.Close()
	return listener.Addr().(*net.TCPAddr).Port, nil
}
