package auth

import (
	"fmt"
	"html/template"
	"net/http"
)

// SetupPageData は設定ページのテンプレートデータ
type SetupPageData struct {
	// 現在の設定（プリフィル用）
	SpaceHost   string // 例: yourspace.backlog.jp
	SpaceURL    string // 例: https://yourspace.backlog.jp（hidden field用）
	RelayServer string

	// 状態
	IsConfigured bool
	ErrorMessage string

	// 確認画面用
	Space  string
	Domain string
}

// 共通のスタイル
const pageStyle = `
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
  padding: 2rem 3rem;
  border-radius: 1rem;
  box-shadow: 0 10px 40px rgba(0,0,0,0.2);
  max-width: 750px;
  width: 90%;
}
h1 {
  color: #333;
  margin: 0 0 1.5rem 0;
  font-size: 1.5rem;
}
.form-group {
  margin-bottom: 1.5rem;
  text-align: left;
}
label {
  display: block;
  color: #555;
  margin-bottom: 0.5rem;
  font-weight: 500;
}
input[type="url"], input[type="text"] {
  width: 100%;
  padding: 0.75rem;
  border: 2px solid #ddd;
  border-radius: 0.5rem;
  font-size: 1rem;
  box-sizing: border-box;
  transition: border-color 0.2s;
}
input[type="url"]:focus, input[type="text"]:focus {
  outline: none;
  border-color: #667eea;
}
button {
  background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
  color: white;
  border: none;
  padding: 0.875rem 2rem;
  font-size: 1rem;
  border-radius: 0.5rem;
  cursor: pointer;
  transition: transform 0.2s, box-shadow 0.2s;
}
button:hover {
  transform: translateY(-2px);
  box-shadow: 0 4px 12px rgba(102, 126, 234, 0.4);
}
button:disabled {
  opacity: 0.6;
  cursor: not-allowed;
  transform: none;
}
.error {
  background: #fee;
  color: #c00;
  padding: 1rem;
  border-radius: 0.5rem;
  margin-bottom: 1.5rem;
  text-align: left;
}
.success {
  background: #e8f5e9;
  color: #2e7d32;
  padding: 1rem;
  border-radius: 0.5rem;
  margin-bottom: 1.5rem;
  text-align: center;
}
.info-box {
  background: #f8f9fa;
  padding: 1rem;
  border-radius: 0.5rem;
  margin-bottom: 1.5rem;
  text-align: left;
}
.info-box .label {
  color: #666;
  font-size: 0.875rem;
  margin-bottom: 0.25rem;
}
.info-box .value {
  color: #333;
  font-weight: 500;
  word-break: break-all;
}
.info-box + .info-box {
  margin-top: 0.75rem;
}
.link-button {
  color: #667eea;
  text-decoration: none;
  font-size: 0.875rem;
  display: inline-block;
  margin-top: 1rem;
}
.link-button:hover {
  text-decoration: underline;
}
.countdown {
  color: #666;
  font-size: 0.875rem;
  margin-top: 1rem;
}
.helper {
  color: #888;
  font-size: 0.75rem;
  margin-top: 0.25rem;
}
.status {
  color: #666;
  font-size: 0.875rem;
  margin-top: 1rem;
}
.status.waiting {
  color: #1976d2;
}
.spinner {
  display: inline-block;
  width: 1rem;
  height: 1rem;
  border: 2px solid #667eea;
  border-radius: 50%;
  border-top-color: transparent;
  animation: spin 1s linear infinite;
  margin-right: 0.5rem;
  vertical-align: middle;
}
@keyframes spin {
  to { transform: rotate(360deg); }
}
.icon {
  font-size: 3rem;
  margin-bottom: 1rem;
}
.icon.success { color: #4caf50; }
.icon.error { color: #f44336; }
.hidden { display: none; }
.warning-box {
  background: #fff8e1;
  border: 1px solid #ffcc02;
  color: #856404;
  padding: 0.875rem 1rem;
  border-radius: 0.5rem;
  margin-bottom: 1.5rem;
  text-align: left;
  font-size: 0.875rem;
  line-height: 1.5;
}
.warning-box .warning-title {
  font-weight: 600;
  margin-bottom: 0.25rem;
}
.button-group {
  display: flex;
  gap: 1rem;
  justify-content: center;
  flex-wrap: wrap;
  margin-top: 1.5rem;
}
.button-secondary {
  background: #f5f5f5;
  color: #333;
  border: 1px solid #ddd;
}
.button-secondary:hover {
  background: #e8e8e8;
  box-shadow: none;
  transform: none;
}
.context-info {
  color: #666;
  font-size: 0.875rem;
  margin-bottom: 1.5rem;
  text-align: left;
  padding: 0.75rem 1rem;
  background: #f0f4ff;
  border-radius: 0.5rem;
  border-left: 3px solid #667eea;
}
`

// openPopupCentered は親ウィンドウの上にポップアップを開く JavaScript 関数
const openPopupScript = `
function openPopupCentered(url, name, width, height) {
  var left = window.screenX + (window.outerWidth - width) / 2;
  var top = window.screenY + (window.outerHeight - height) / 2;
  return window.open(url, name,
    'width=' + width + ',height=' + height +
    ',left=' + left + ',top=' + top +
    ',menubar=no,toolbar=no,location=no,status=no');
}
`

// 共通のWebSocket接続スクリプト
const wsScript = `
var wsConnection = null;
var wsActive = true;

function startWebSocket(onSuccess, onError, onServerClosed) {
  console.log('[Backlog CLI] WebSocket connecting...');

  var protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
  var wsUrl = protocol + '//' + window.location.host + '/auth/ws';

  try {
    wsConnection = new WebSocket(wsUrl);
  } catch (err) {
    console.error('[Backlog CLI] WebSocket creation failed:', err);
    if (onServerClosed) onServerClosed();
    return;
  }

  wsConnection.onopen = function() {
    console.log('[Backlog CLI] WebSocket connected');
  };

  wsConnection.onmessage = function(event) {
    if (!wsActive) return;

    try {
      var data = JSON.parse(event.data);
      console.log('[Backlog CLI] WebSocket message:', data.status);

      if (data.status === 'success') {
        wsActive = false;
        if (onSuccess) onSuccess();
      } else if (data.status === 'error') {
        wsActive = false;
        if (onError) onError(data.error || '認証に失敗しました');
      }
      // pending の場合は何もしない（次のメッセージを待つ）
    } catch (err) {
      console.error('[Backlog CLI] WebSocket message parse error:', err);
    }
  };

  wsConnection.onerror = function(err) {
    console.error('[Backlog CLI] WebSocket error:', err);
  };

  wsConnection.onclose = function(event) {
    console.log('[Backlog CLI] WebSocket closed:', event.code, event.reason);
    if (wsActive) {
      wsActive = false;
      if (onServerClosed) onServerClosed();
    }
  };
}

function stopWebSocket() {
  wsActive = false;
  if (wsConnection) {
    wsConnection.close();
    wsConnection = null;
  }
}

// 自動クローズ機能（ユーザースクリプト対応）
function tryAutoClose(onAutoCloseAvailable, onAutoCloseUnavailable) {
  var checkCount = 0;
  var maxChecks = 10;

  function check() {
    if (typeof forceCloseTab === 'function') {
      if (onAutoCloseAvailable) onAutoCloseAvailable();
      return;
    }
    checkCount++;
    if (checkCount < maxChecks) {
      setTimeout(check, 100);
    } else {
      if (onAutoCloseUnavailable) onAutoCloseUnavailable();
    }
  }
  check();
}

function startAutoCloseCountdown(seconds, countdownEl, onComplete) {
  var remaining = seconds;

  function tick() {
    countdownEl.textContent = remaining + ' 秒後に閉じます...';
    if (remaining <= 0) {
      if (onComplete) onComplete();
    } else {
      remaining--;
      setTimeout(tick, 1000);
    }
  }
  tick();
}
`

// setupFormPageTemplate は入力フォームのテンプレート
var setupFormPageTemplate = template.Must(template.New("setup").Parse(`<!DOCTYPE html>
<html lang="ja">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Backlog CLI - ログイン設定</title>
<style>` + pageStyle + `
.server-closed {
  background: #fff3e0;
  border: 1px solid #ff9800;
  color: #e65100;
  padding: 1.5rem;
  border-radius: 0.5rem;
  text-align: center;
}
.server-closed h2 {
  margin: 0 0 0.5rem 0;
  font-size: 1.25rem;
}
.server-closed p {
  margin: 0;
}
</style>
</head>
<body>
<div class="container">
  <div id="form-view">
    <h1>Backlog CLI ログイン設定</h1>
    {{if .ErrorMessage}}
    <div class="error" id="error-message">{{.ErrorMessage}}</div>
    {{else}}
    <div class="error hidden" id="error-message"></div>
    {{end}}
    <form id="setup-form">
      <div class="form-group">
        <label for="space_host">スペース</label>
        <input type="text" id="space_host" name="space_host"
               value="{{.SpaceHost}}"
               placeholder="yourspace.backlog.jp"
               required>
        <div class="helper">例: yourspace.backlog.jp</div>
      </div>
      <div class="form-group">
        <label for="relay_server">リレーサーバーURL</label>
        <input type="url" id="relay_server" name="relay_server"
               value="{{.RelayServer}}"
               placeholder="https://relay.example.com"
               required>
        <div class="helper">OAuth認証を中継するサーバーのURL</div>
      </div>
      <div class="warning-box">
        <div class="warning-title">セキュリティに関する注意</div>
        リレーサーバーは OAuth 認証を中継し、アクセストークンを取り扱います。
        信頼できるサーバーのみを指定してください。
        不明な場合は、組織の管理者にご確認ください。
      </div>
      <div class="button-group">
        <button type="button" class="button-secondary" onclick="location.href='/auth/start'">キャンセル</button>
        <button type="submit" id="submit-btn">登録して続行</button>
      </div>
    </form>
    <p class="status hidden" id="status"><span class="spinner"></span><span id="status-text">認証中...</span></p>
  </div>
  <div id="success-view" class="hidden">
    <div class="icon success">✓</div>
    <h1>認証が完了しました</h1>
    <p style="color: #666;">ターミナルに戻って操作を続けてください。</p>
    <p id="success-auto-close-msg" class="hidden" style="color: #666;">このタブは自動的に閉じられます。</p>
    <p id="success-countdown" class="hidden" style="color: #999; font-size: 0.875rem;"></p>
    <p id="success-manual-close-msg" style="color: #999; font-size: 0.875rem;">このタブは閉じて構いません。</p>
  </div>
  <div id="error-view" class="hidden">
    <div class="icon error">✗</div>
    <h1>認証に失敗しました</h1>
    <p style="color: #666;" id="final-error-message"></p>
    <p style="color: #999; font-size: 0.875rem;">このタブは閉じて構いません。</p>
  </div>
  <div id="server-closed-view" class="hidden">
    <div class="server-closed">
      <h2>CLIが終了しました</h2>
      <p>ターミナルで再度 <code>backlog auth login</code> を実行してください。</p>
      <p style="margin-top: 1rem;"><button onclick="window.close()">このタブを閉じる</button></p>
    </div>
  </div>
</div>
<script>
` + openPopupScript + wsScript + `
(function() {
  var form = document.getElementById('setup-form');
  var submitBtn = document.getElementById('submit-btn');
  var statusEl = document.getElementById('status');
  var statusText = document.getElementById('status-text');
  var errorMessage = document.getElementById('error-message');
  var formView = document.getElementById('form-view');
  var successView = document.getElementById('success-view');
  var errorView = document.getElementById('error-view');
  var serverClosedView = document.getElementById('server-closed-view');
  var finalErrorMessage = document.getElementById('final-error-message');
  var successAutoCloseMsg = document.getElementById('success-auto-close-msg');
  var successCountdown = document.getElementById('success-countdown');
  var successManualCloseMsg = document.getElementById('success-manual-close-msg');

  function showError(msg) {
    errorMessage.textContent = msg;
    errorMessage.classList.remove('hidden');
    submitBtn.disabled = false;
    statusEl.classList.add('hidden');
  }

  function showFinalError(msg) {
    formView.classList.add('hidden');
    finalErrorMessage.textContent = msg;
    errorView.classList.remove('hidden');
  }

  function showSuccess() {
    formView.classList.add('hidden');
    successView.classList.remove('hidden');

    // 自動クローズが可能かチェック
    tryAutoClose(
      function() {
        // 自動クローズ可能：カウントダウン開始
        successManualCloseMsg.classList.add('hidden');
        successAutoCloseMsg.classList.remove('hidden');
        successCountdown.classList.remove('hidden');
        startAutoCloseCountdown(5, successCountdown, function() {
          forceCloseTab();
        });
      },
      null // 自動クローズ不可の場合は何もしない（初期状態のまま）
    );
  }

  function showServerClosed() {
    formView.classList.add('hidden');
    serverClosedView.classList.remove('hidden');
  }

  // ページ読み込み時からWebSocket接続開始（死活監視）
  startWebSocket(showSuccess, showFinalError, showServerClosed);

  form.addEventListener('submit', function(e) {
    e.preventDefault();

    submitBtn.disabled = true;
    errorMessage.classList.add('hidden');
    statusEl.classList.remove('hidden');
    statusText.textContent = '設定を保存中...';

    var formData = new FormData(form);

    fetch('/auth/configure', {
      method: 'POST',
      headers: { 'Accept': 'application/json' },
      body: formData
    })
    .then(function(resp) { return resp.json(); })
    .then(function(data) {
      if (data.error) {
        showError(data.error);
        return;
      }

      // 設定保存成功、確認画面へ遷移
      statusText.textContent = '設定を保存しました。確認画面に戻ります...';
      location.href = '/auth/start';
    })
    .catch(function(err) {
      showError('エラーが発生しました: ' + err.message);
    });
  });
})();
</script>
</body>
</html>`))

// confirmPageTemplate は確認画面のテンプレート
var confirmPageTemplate = template.Must(template.New("confirm").Parse(`<!DOCTYPE html>
<html lang="ja">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Backlog CLI - ログイン確認</title>
<style>` + pageStyle + `
.server-closed {
  background: #fff3e0;
  border: 1px solid #ff9800;
  color: #e65100;
  padding: 1.5rem;
  border-radius: 0.5rem;
  text-align: center;
}
.server-closed h2 {
  margin: 0 0 0.5rem 0;
  font-size: 1.25rem;
}
.server-closed p {
  margin: 0;
}
</style>
</head>
<body>
<div class="container">
  <div id="form-view">
    <h1>Backlog CLI ログイン</h1>
    <div class="context-info">
      Backlog CLI がターミナルからの操作で Backlog API にアクセスするための認証を行います。
    </div>
    <div class="info-box">
      <div class="label">スペース</div>
      <div class="value">{{.Space}}.{{.Domain}}</div>
    </div>
    <div class="info-box">
      <div class="label">リレーサーバー</div>
      <div class="value">{{.RelayServer}}</div>
    </div>
    <div class="button-group">
      <button type="button" class="button-secondary" onclick="stopWebSocket(); location.href='/auth/setup'">設定を変更</button>
      <button type="button" id="login-btn">ログインする</button>
    </div>
    <p class="status hidden" id="status"><span class="spinner"></span><span id="status-text">認証中...</span></p>
  </div>
  <div id="success-view" class="hidden">
    <div class="icon success">✓</div>
    <h1>認証が完了しました</h1>
    <p style="color: #666;">ターミナルに戻って操作を続けてください。</p>
    <p id="success-auto-close-msg" class="hidden" style="color: #666;">このタブは自動的に閉じられます。</p>
    <p id="success-countdown" class="hidden" style="color: #999; font-size: 0.875rem;"></p>
    <p id="success-manual-close-msg" style="color: #999; font-size: 0.875rem;">このタブは閉じて構いません。</p>
  </div>
  <div id="error-view" class="hidden">
    <div class="icon error">✗</div>
    <h1>認証に失敗しました</h1>
    <p style="color: #666;" id="final-error-message"></p>
    <p style="color: #999; font-size: 0.875rem;">このタブは閉じて構いません。</p>
  </div>
  <div id="server-closed-view" class="hidden">
    <div class="server-closed">
      <h2>CLIが終了しました</h2>
      <p>ターミナルで再度 <code>backlog auth login</code> を実行してください。</p>
      <p style="margin-top: 1rem;"><button onclick="window.close()">このタブを閉じる</button></p>
    </div>
  </div>
</div>
<script>
` + openPopupScript + wsScript + `
(function() {
  var loginBtn = document.getElementById('login-btn');
  var statusEl = document.getElementById('status');
  var statusText = document.getElementById('status-text');
  var formView = document.getElementById('form-view');
  var successView = document.getElementById('success-view');
  var errorView = document.getElementById('error-view');
  var serverClosedView = document.getElementById('server-closed-view');
  var finalErrorMessage = document.getElementById('final-error-message');
  var successAutoCloseMsg = document.getElementById('success-auto-close-msg');
  var successCountdown = document.getElementById('success-countdown');
  var successManualCloseMsg = document.getElementById('success-manual-close-msg');

  function showFinalError(msg) {
    formView.classList.add('hidden');
    finalErrorMessage.textContent = msg;
    errorView.classList.remove('hidden');
  }

  function showSuccess() {
    formView.classList.add('hidden');
    successView.classList.remove('hidden');

    // 自動クローズが可能かチェック
    tryAutoClose(
      function() {
        // 自動クローズ可能：カウントダウン開始
        successManualCloseMsg.classList.add('hidden');
        successAutoCloseMsg.classList.remove('hidden');
        successCountdown.classList.remove('hidden');
        startAutoCloseCountdown(5, successCountdown, function() {
          forceCloseTab();
        });
      },
      null // 自動クローズ不可の場合は何もしない（初期状態のまま）
    );
  }

  function showServerClosed() {
    formView.classList.add('hidden');
    serverClosedView.classList.remove('hidden');
  }

  // ページ読み込み時からWebSocket接続開始（死活監視）
  startWebSocket(showSuccess, showFinalError, showServerClosed);

  loginBtn.addEventListener('click', function() {
    loginBtn.disabled = true;
    statusEl.classList.remove('hidden');
    statusText.textContent = 'ログイン画面を開いています...';

    // ポップアップを開く（親ウィンドウの中央に配置）
    var popup = openPopupCentered('/auth/popup', 'backlog_auth', 600, 700);

    if (!popup || popup.closed || typeof popup.closed === 'undefined') {
      statusText.textContent = 'ポップアップがブロックされました。ポップアップを許可してください。';
      loginBtn.disabled = false;
      return;
    }

    statusText.textContent = 'ポップアップで認証を進めてください...';

    // ポップアップの状態を監視
    var popupCheckInterval = setInterval(function() {
      // 認証が完了している場合は監視を停止
      if (!wsActive) {
        clearInterval(popupCheckInterval);
        return;
      }

      // ポップアップが閉じられた場合
      if (popup.closed) {
        clearInterval(popupCheckInterval);
        // WebSocket接続を閉じてCLIにタイムアウトを発生させる
        stopWebSocket();
        showFinalError('認証がキャンセルされました。ポップアップが閉じられました。');
      }
    }, 1000);
  });
})();
</script>
</body>
</html>`))

// renderSetupForm は入力フォームを描画する
func renderSetupForm(w http.ResponseWriter, data SetupPageData) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := setupFormPageTemplate.Execute(w, data); err != nil {
		http.Error(w, fmt.Sprintf("template error: %v", err), http.StatusInternalServerError)
	}
}

// renderConfirmPage は確認画面を描画する
func renderConfirmPage(w http.ResponseWriter, data SetupPageData) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := confirmPageTemplate.Execute(w, data); err != nil {
		http.Error(w, fmt.Sprintf("template error: %v", err), http.StatusInternalServerError)
	}
}
