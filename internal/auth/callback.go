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

	// Accept-Languageヘッダーから言語を判定
	isJapanese := isJapanesePreferred(r.Header.Get("Accept-Language"))

	// 成功ページを表示
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if isJapanese {
		fmt.Fprint(w, successPageJa)
	} else {
		fmt.Fprint(w, successPageEn)
	}
}

// isJapanesePreferred はAccept-Languageヘッダーから日本語が優先されているかを判定する
func isJapanesePreferred(acceptLanguage string) bool {
	if acceptLanguage == "" {
		return false
	}
	// 簡易的な判定: "ja" が含まれていて、それが先頭近くにあるかをチェック
	// Accept-Language例: "ja,en-US;q=0.9,en;q=0.8"
	for i, part := range splitAcceptLanguage(acceptLanguage) {
		lang := extractLanguageTag(part)
		if lang == "ja" || lang == "ja-jp" {
			return true
		}
		// 最初の言語が日本語でない場合は英語優先
		if i == 0 {
			return false
		}
	}
	return false
}

// splitAcceptLanguage はAccept-Languageヘッダーを分割する
func splitAcceptLanguage(header string) []string {
	var result []string
	for _, part := range splitByComma(header) {
		part = trimSpace(part)
		if part != "" {
			result = append(result, part)
		}
	}
	return result
}

// splitByComma はカンマで文字列を分割する（strings.Splitの代わり）
func splitByComma(s string) []string {
	var result []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == ',' {
			result = append(result, s[start:i])
			start = i + 1
		}
	}
	result = append(result, s[start:])
	return result
}

// trimSpace は文字列の前後の空白を除去する
func trimSpace(s string) string {
	start := 0
	end := len(s)
	for start < end && (s[start] == ' ' || s[start] == '\t') {
		start++
	}
	for end > start && (s[end-1] == ' ' || s[end-1] == '\t') {
		end--
	}
	return s[start:end]
}

// extractLanguageTag は言語タグを抽出する（例: "en-US;q=0.9" -> "en-us"）
func extractLanguageTag(part string) string {
	// セミコロンより前の部分を取得
	for i := 0; i < len(part); i++ {
		if part[i] == ';' {
			part = part[:i]
			break
		}
	}
	// 小文字に変換
	result := make([]byte, len(part))
	for i := 0; i < len(part); i++ {
		c := part[i]
		if c >= 'A' && c <= 'Z' {
			c = c + 32
		}
		result[i] = c
	}
	return string(result)
}

// ユーザースクリプト（Tampermonkey等）による自動クローズのサポート:
//
// ユーザースクリプトで window.forceCloseTab 関数を公開すると、
// ページ側でカウントダウン表示後に自動でタブを閉じます。
// ユーザースクリプトが未導入の場合は、手動でタブを閉じるよう案内します。
//
// インストール方法:
//   Tampermonkey等をインストール後、以下のURLを開いてください:
//   https://github.com/yacchi/backlog-cli/raw/master/scripts/backlog-cli-auto-close.user.js

const successPageJa = `<!DOCTYPE html>
<html lang="ja">
<head>
<meta charset="utf-8">
<title>認証成功</title>
<style>
body {
  font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, "Hiragino Sans", "Noto Sans CJK JP", sans-serif;
  display: flex;
  justify-content: center;
  align-items: center;
  min-height: 100vh;
  margin: 0;
  background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
}
.container {
  text-align: center;
  background: white;
  padding: 3rem;
  border-radius: 1rem;
  box-shadow: 0 10px 40px rgba(0,0,0,0.2);
  max-width: 400px;
}
.icon {
  font-size: 4rem;
  margin-bottom: 1rem;
}
h1 {
  color: #333;
  margin: 0 0 1rem 0;
  font-size: 1.5rem;
}
p {
  color: #666;
  margin: 0 0 1.5rem 0;
  line-height: 1.6;
}
.note {
  color: #999;
  font-size: 0.9rem;
}
</style>
</head>
<body data-auth-callback="success">
<div class="container">
  <div class="icon">✓</div>
  <h1>認証が完了しました</h1>
  <p>ターミナルに戻って操作を続けてください。</p>
  <p class="note" id="note">このタブは閉じて構いません。</p>
</div>
<script>
// ユーザースクリプトで window.forceCloseTab が公開されている場合、
// カウントダウン後に自動でタブを閉じる
(function() {
  var noteEl = document.getElementById('note');
  if (typeof window.forceCloseTab === 'function') {
    var seconds = 3;
    noteEl.textContent = seconds + ' 秒後にこのタブを閉じます...';
    var interval = setInterval(function() {
      seconds--;
      if (seconds > 0) {
        noteEl.textContent = seconds + ' 秒後にこのタブを閉じます...';
      } else {
        clearInterval(interval);
        window.forceCloseTab();
      }
    }, 1000);
  }
})();
</script>
</body>
</html>`

const successPageEn = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<title>Authentication Successful</title>
<style>
body {
  font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif;
  display: flex;
  justify-content: center;
  align-items: center;
  min-height: 100vh;
  margin: 0;
  background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
}
.container {
  text-align: center;
  background: white;
  padding: 3rem;
  border-radius: 1rem;
  box-shadow: 0 10px 40px rgba(0,0,0,0.2);
  max-width: 400px;
}
.icon {
  font-size: 4rem;
  margin-bottom: 1rem;
}
h1 {
  color: #333;
  margin: 0 0 1rem 0;
  font-size: 1.5rem;
}
p {
  color: #666;
  margin: 0 0 1.5rem 0;
  line-height: 1.6;
}
.note {
  color: #999;
  font-size: 0.9rem;
}
</style>
</head>
<body data-auth-callback="success">
<div class="container">
  <div class="icon">✓</div>
  <h1>Authentication Successful</h1>
  <p>You can return to the terminal to continue.</p>
  <p class="note" id="note">You may close this tab.</p>
</div>
<script>
// If userscript exposes window.forceCloseTab, auto-close after countdown
(function() {
  var noteEl = document.getElementById('note');
  if (typeof window.forceCloseTab === 'function') {
    var seconds = 3;
    noteEl.textContent = 'Closing this tab in ' + seconds + ' seconds...';
    var interval = setInterval(function() {
      seconds--;
      if (seconds > 0) {
        noteEl.textContent = 'Closing this tab in ' + seconds + ' seconds...';
      } else {
        clearInterval(interval);
        window.forceCloseTab();
      }
    }, 1000);
  }
})();
</script>
</body>
</html>`

// FindFreePort は空いているポートを探す
func FindFreePort() (int, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer listener.Close()
	return listener.Addr().(*net.TCPAddr).Port, nil
}
