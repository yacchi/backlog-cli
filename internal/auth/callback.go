package auth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/coder/websocket"
	"github.com/yacchi/backlog-cli/internal/config"
	"github.com/yacchi/backlog-cli/internal/debug"
	"github.com/yacchi/backlog-cli/internal/ui"
)

// セッションCookie名
const sessionCookieName = "backlog_auth_session"

// CallbackResult はコールバックの結果
type CallbackResult struct {
	Code  string
	Error error
}

// CallbackServerOptions はコールバックサーバーのオプション
type CallbackServerOptions struct {
	Port        int
	State       string          // CLI が生成した state
	ConfigStore *config.Store   // 設定の読み書き用
	Reuse       bool            // true の場合、確認画面をスキップして即座にリダイレクト
	Ctx         context.Context // コマンドのContext（シグナル処理用）
}

// WSMessage はWebSocketで送信するメッセージ
type WSMessage struct {
	Status string `json:"status"` // "pending", "success", "error"
	Error  string `json:"error,omitempty"`
}

// Session は認証セッションを表す
type Session struct {
	ID             string          // セッションID
	CreatedAt      time.Time       // 作成時刻
	LastActivityAt time.Time       // 最終アクティビティ時刻
	Status         string          // pending/success/error
	ErrorMessage   string          // エラーメッセージ
	WSConn         *websocket.Conn // WebSocket接続（nil=未接続）
	DisconnectedAt *time.Time      // WebSocket切断時刻（nil=接続中または未接続）
}

// CallbackServer はCLIのローカルコールバックサーバー
type CallbackServer struct {
	port        int
	server      *http.Server
	result      chan CallbackResult
	listener    net.Listener
	once        sync.Once
	state       string
	configStore *config.Store
	reuse       bool
	ctx         context.Context // コマンドのContext

	// セッション管理
	session            *Session      // 現在のセッション（1サーバー1セッション）
	sessionMu          sync.RWMutex  // セッションアクセス用ミューテックス
	sessionEstablished bool          // セッションが確立されたことがあるか
	cancelCheck        chan struct{} // チェッカーを停止するためのチャネル
	cancelOnce         sync.Once     // cancelCheck を一度だけ閉じるため
}

// authConfig は認証設定を取得する
func (cs *CallbackServer) authConfig() *config.ResolvedAuth {
	return &cs.configStore.Resolved().Auth
}

// wsConfig はWebSocket設定を取得する
func (cs *CallbackServer) wsConfig() *config.ResolvedAuthWebSocket {
	return &cs.authConfig().WebSocket
}

// sessionConfig はセッション設定を取得する
func (cs *CallbackServer) sessionConfig() *config.ResolvedAuthSession {
	return &cs.authConfig().Session
}

// NewCallbackServer は新しいコールバックサーバーを作成する
func NewCallbackServer(opts CallbackServerOptions) (*CallbackServer, error) {
	// ポートが0の場合は空きポートを探す
	addr := fmt.Sprintf("127.0.0.1:%d", opts.Port)
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("failed to listen on %s: %w", addr, err)
	}

	// 実際のポートを取得
	actualPort := listener.Addr().(*net.TCPAddr).Port

	// Contextが指定されていない場合はBackgroundを使用
	ctx := opts.Ctx
	if ctx == nil {
		ctx = context.Background()
	}

	cs := &CallbackServer{
		port:        actualPort,
		result:      make(chan CallbackResult, 1),
		listener:    listener,
		state:       opts.State,
		configStore: opts.ConfigStore,
		reuse:       opts.Reuse,
		ctx:         ctx,
		cancelCheck: make(chan struct{}),
	}

	debug.Log("callback server created", "port", actualPort, "address", addr)

	cs.server = &http.Server{
		Handler: cs.setupRoutes(),
	}

	return cs, nil
}

func (cs *CallbackServer) setupRoutes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/auth/config", cs.handleConfig)
	mux.HandleFunc("/auth/configure", cs.handleConfigure)
	mux.HandleFunc("/auth/ws", cs.handleWebSocket)
	mux.HandleFunc("/auth/popup", cs.handlePopup)
	mux.HandleFunc("/callback", cs.handleCallback)

	assets, err := ui.Assets()
	if err != nil {
		debug.Log("ui assets unavailable", "error", err)
		http.Error(nil, "SPA assets not available", http.StatusInternalServerError)
		return mux
	}

	spaHandler := ui.SPAHandler(assets)
	mux.Handle("/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Ensure a session cookie exists before the SPA boots and opens WebSocket.
		_ = cs.ensureSession(w, r)
		spaHandler.ServeHTTP(w, r)
	}))
	return mux
}

// generateSessionID はセッションIDを生成する
func generateSessionID() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(b), nil
}

// createSession は新しいセッションを作成する
func (cs *CallbackServer) createSession() (*Session, error) {
	id, err := generateSessionID()
	if err != nil {
		return nil, err
	}

	now := time.Now()
	session := &Session{
		ID:             id,
		CreatedAt:      now,
		LastActivityAt: now,
		Status:         "pending",
	}

	cs.sessionMu.Lock()
	cs.session = session
	cs.sessionEstablished = true // セッションが確立されたことを記録
	cs.sessionMu.Unlock()

	debug.Log("session created", "id", id[:16]+"...", "established", true)
	return session, nil
}

// getSessionFromCookie はCookieからセッションを取得・検証する
func (cs *CallbackServer) getSessionFromCookie(r *http.Request) *Session {
	cookie, err := r.Cookie(sessionCookieName)
	if err != nil {
		return nil
	}

	cs.sessionMu.RLock()
	defer cs.sessionMu.RUnlock()

	if cs.session != nil && cs.session.ID == cookie.Value {
		return cs.session
	}
	return nil
}

// setSessionCookie はセッションCookieを設定する
func (cs *CallbackServer) setSessionCookie(w http.ResponseWriter, sessionID string) {
	timeout := cs.sessionConfig().TimeoutDuration()
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    sessionID,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(timeout.Seconds()),
	})
}

// updateSessionActivity はセッションの最終アクティビティ時刻を更新する
func (cs *CallbackServer) updateSessionActivity() {
	cs.sessionMu.Lock()
	defer cs.sessionMu.Unlock()
	if cs.session != nil {
		cs.session.LastActivityAt = time.Now()
	}
}

// GenerateState は認証用の state を生成する
func GenerateState() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(b), nil
}

// Port は実際のポート番号を返す
func (cs *CallbackServer) Port() int {
	return cs.port
}

// Start はサーバーを起動する
func (cs *CallbackServer) Start() error {
	debug.Log("callback server starting", "port", cs.port)

	// セッションチェッカーを起動
	go cs.checkSessionTimeout()

	return cs.server.Serve(cs.listener)
}

// checkSessionTimeout はセッションのタイムアウトをチェックする
func (cs *CallbackServer) checkSessionTimeout() {
	debug.Log("session timeout checker started")
	sessConf := cs.sessionConfig()
	ticker := time.NewTicker(sessConf.CheckIntervalDuration())
	defer ticker.Stop()

	checkCount := 0
	var noConnectionSince *time.Time // WebSocket接続がない状態が始まった時刻

	for {
		select {
		case <-cs.cancelCheck:
			debug.Log("session timeout checker stopped")
			return
		case <-ticker.C:
			checkCount++

			cs.sessionMu.RLock()
			session := cs.session
			established := cs.sessionEstablished
			var status string
			var createdAt time.Time
			var wsConn *websocket.Conn
			if session != nil {
				status = session.Status
				createdAt = session.CreatedAt
				wsConn = session.WSConn
			}
			cs.sessionMu.RUnlock()

			// セッションがまだ確立されていない場合
			if !established {
				// 初期状態：ブラウザがまだアクセスしていない
				continue
			}

			// 10回に1回状態をログ出力
			if checkCount%10 == 0 {
				hasWS := wsConn != nil
				noConnDuration := ""
				if noConnectionSince != nil {
					noConnDuration = time.Since(*noConnectionSince).String()
				}
				debug.Log("session checker tick",
					"count", checkCount,
					"status", status,
					"hasWS", hasWS,
					"noConnDuration", noConnDuration,
					"age", time.Since(createdAt).String())
			}

			// 認証が完了している場合はチェックしない
			if status != "pending" {
				debug.Log("auth completed, stopping checker", "status", status)
				return
			}

			// セッション全体のタイムアウトチェック
			if time.Since(createdAt) > sessConf.TimeoutDuration() {
				debug.Log("session timeout detected", "age", time.Since(createdAt).String())
				cs.once.Do(func() {
					cs.result <- CallbackResult{
						Error: fmt.Errorf("authentication timeout (session expired)"),
					}
				})
				return
			}

			// WebSocket接続状態のチェック
			if wsConn != nil {
				// WebSocket接続中：タイマーをリセット
				noConnectionSince = nil
				continue
			}

			// WebSocket接続がない状態
			now := time.Now()
			if noConnectionSince == nil {
				// 接続がない状態が始まった
				noConnectionSince = &now
				debug.Log("no WebSocket connection detected, starting grace period timer")
			}

			// 接続がない状態が一定時間続いたらタイムアウト
			gracePeriod := cs.wsConfig().DisconnectGracePeriodDuration()
			elapsed := time.Since(*noConnectionSince)
			if elapsed > gracePeriod {
				debug.Log("no connection grace period expired",
					"elapsed", elapsed.String(),
					"grace", gracePeriod.String())
				cs.once.Do(func() {
					cs.result <- CallbackResult{
						Error: fmt.Errorf("authentication cancelled (browser closed or navigated away)"),
					}
				})
				return
			}
		}
	}
}

// Wait はコールバックを待機する
func (cs *CallbackServer) Wait() CallbackResult {
	return <-cs.result
}

// Shutdown はサーバーを停止する
func (cs *CallbackServer) Shutdown(ctx context.Context) error {
	// セッションチェッカーを停止
	cs.cancelOnce.Do(func() {
		close(cs.cancelCheck)
	})
	return cs.server.Shutdown(ctx)
}

// ensureSession はセッションが存在することを確認し、なければ作成してCookieを設定する
func (cs *CallbackServer) ensureSession(w http.ResponseWriter, r *http.Request) *Session {
	// 既存のセッションをCookieから取得
	session := cs.getSessionFromCookie(r)
	if session != nil {
		cs.updateSessionActivity()
		return session
	}

	// セッションがなければ新規作成
	session, err := cs.createSession()
	if err != nil {
		debug.Log("failed to create session", "error", err)
		return nil
	}

	// Cookieを設定
	cs.setSessionCookie(w, session.ID)
	return session
}


// AuthConfigResponse は設定取得のJSONレスポンス
type AuthConfigResponse struct {
	Space       string `json:"space"`
	Domain      string `json:"domain"`
	RelayServer string `json:"relayServer"`
	SpaceHost   string `json:"spaceHost"`
	Configured  bool   `json:"configured"`
}

func (cs *CallbackServer) handleConfig(w http.ResponseWriter, r *http.Request) {
	debug.Log("auth/config accessed", "method", r.Method)

	if r.Method != http.MethodGet {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusMethodNotAllowed)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "Method not allowed"})
		return
	}

	session := cs.ensureSession(w, r)
	if session == nil {
		http.Error(w, "Failed to create session", http.StatusInternalServerError)
		return
	}

	profile := cs.configStore.CurrentProfile()
	spaceHost := ""
	if profile.Space != "" && profile.Domain != "" {
		spaceHost = fmt.Sprintf("%s.%s", profile.Space, profile.Domain)
	}

	payload := AuthConfigResponse{
		Space:       profile.Space,
		Domain:      profile.Domain,
		RelayServer: profile.RelayServer,
		SpaceHost:   spaceHost,
		Configured:  profile.Space != "" && profile.Domain != "" && profile.RelayServer != "",
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(payload)
}

// ConfigureResponse は設定保存の JSON レスポンス
type ConfigureResponse struct {
	Success bool   `json:"success"`
	Error   string `json:"error,omitempty"`
}

// handleConfigure はフォーム送信を処理する（JSON APIのみ）
func (cs *CallbackServer) handleConfigure(w http.ResponseWriter, r *http.Request) {
	debug.Log("auth/configure accessed", "method", r.Method)

	w.Header().Set("Content-Type", "application/json")

	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		_ = json.NewEncoder(w).Encode(ConfigureResponse{Success: false, Error: "Method not allowed"})
		return
	}

	// FormData (multipart/form-data) と通常のフォーム (application/x-www-form-urlencoded) の両方に対応
	contentType := r.Header.Get("Content-Type")
	if strings.HasPrefix(contentType, "multipart/form-data") {
		if err := r.ParseMultipartForm(32 << 10); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(ConfigureResponse{Success: false, Error: "Failed to parse form"})
			return
		}
	} else {
		if err := r.ParseForm(); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(ConfigureResponse{Success: false, Error: "Failed to parse form"})
			return
		}
	}

	spaceHost := r.FormValue("space_host")
	relayServer := r.FormValue("relay_server")

	debug.Log("form values", "space_host", spaceHost, "relay_server", relayServer)

	// スペースホストをパース
	space, domain, err := parseSpaceHost(spaceHost)
	if err != nil {
		debug.Log("invalid space host", "error", err)
		_ = json.NewEncoder(w).Encode(ConfigureResponse{Success: false, Error: "無効なスペース形式です: " + err.Error()})
		return
	}

	// well-known チェック
	client := NewClient(relayServer)
	wellKnown, err := client.FetchWellKnown()
	if err != nil {
		debug.Log("failed to fetch well-known", "error", err)
		_ = json.NewEncoder(w).Encode(ConfigureResponse{Success: false, Error: fmt.Sprintf("リレーサーバーに接続できません: %v", err)})
		return
	}

	// ドメインがサポートされているかチェック
	if !slices.Contains(wellKnown.SupportedDomains, domain) {
		debug.Log("domain not supported", "domain", domain, "supported", wellKnown.SupportedDomains)
		_ = json.NewEncoder(w).Encode(ConfigureResponse{Success: false, Error: fmt.Sprintf("このリレーサーバーは %s をサポートしていません（サポート: %s）", domain, strings.Join(wellKnown.SupportedDomains, ", "))})
		return
	}

	// 設定保存
	profileName := cs.configStore.GetActiveProfile()
	if err := cs.configStore.SetProfileValue(config.LayerUser, profileName, "relay_server", relayServer); err != nil {
		debug.Log("failed to save relay_server", "error", err)
		_ = json.NewEncoder(w).Encode(ConfigureResponse{Success: false, Error: fmt.Sprintf("設定の保存に失敗しました: %v", err)})
		return
	}
	if err := cs.configStore.SetProfileValue(config.LayerUser, profileName, "space", space); err != nil {
		debug.Log("failed to save space", "error", err)
	}
	if err := cs.configStore.SetProfileValue(config.LayerUser, profileName, "domain", domain); err != nil {
		debug.Log("failed to save domain", "error", err)
	}

	if err := cs.configStore.Save(r.Context()); err != nil {
		debug.Log("failed to save config", "error", err)
		_ = json.NewEncoder(w).Encode(ConfigureResponse{Success: false, Error: fmt.Sprintf("設定の保存に失敗しました: %v", err)})
		return
	}

	debug.Log("config saved")
	_ = json.NewEncoder(w).Encode(ConfigureResponse{Success: true})
}

// redirectToRelay は中継サーバーへリダイレクトする
func (cs *CallbackServer) redirectToRelay(w http.ResponseWriter, r *http.Request, relayServer, space, domain string) {
	// プロジェクト名を取得（設定されている場合）
	project := cs.configStore.CurrentProfile().Project

	redirectURL := fmt.Sprintf(
		"%s/auth/start?port=%d&state=%s&space=%s&domain=%s",
		strings.TrimRight(relayServer, "/"),
		cs.port,
		url.QueryEscape(cs.state),
		url.QueryEscape(space),
		url.QueryEscape(domain),
	)
	if project != "" {
		redirectURL += "&project=" + url.QueryEscape(project)
	}

	debug.Log("redirecting to relay server", "url", redirectURL)
	http.Redirect(w, r, redirectURL, http.StatusFound)
}

// handleWebSocket はWebSocket接続を処理する
func (cs *CallbackServer) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	debug.Log("WebSocket connection request received")

	// セッションを検証
	session := cs.getSessionFromCookie(r)
	if session == nil {
		debug.Log("WebSocket: no valid session")
		http.Error(w, "No valid session", http.StatusUnauthorized)
		return
	}

	// WebSocket接続を受け入れる
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		// localhostからの接続のみ許可（Origin チェックをスキップ）
		InsecureSkipVerify: true,
	})
	if err != nil {
		debug.Log("WebSocket accept failed", "error", err)
		http.Error(w, "WebSocket upgrade failed", http.StatusBadRequest)
		return
	}

	debug.Log("WebSocket connection established", "sessionID", session.ID[:16]+"...")

	// セッションにWebSocket接続を紐付け
	cs.sessionMu.Lock()
	session.WSConn = conn
	session.DisconnectedAt = nil // 切断時刻をクリア
	cs.sessionMu.Unlock()

	// ユーザーに通知
	fmt.Fprintf(os.Stderr, "Browser connected.\n")

	// WebSocket用のcontextを作成（コマンドのcontextを親として使用）
	ctx, cancel := context.WithCancel(cs.ctx)
	defer cancel()

	// 接続終了時の処理
	defer func() {
		cs.handleWSDisconnect()
		_ = conn.Close(websocket.StatusNormalClosure, "connection closed")
	}()

	// 現在の状態を即座に送信
	cs.sessionMu.RLock()
	status := session.Status
	errorMsg := session.ErrorMessage
	cs.sessionMu.RUnlock()

	msg := WSMessage{Status: status}
	if errorMsg != "" {
		msg.Error = errorMsg
	}
	if err := cs.sendWSMessage(ctx, conn, msg); err != nil {
		debug.Log("failed to send initial status", "error", err)
		return
	}

	// 認証が既に完了している場合は終了
	if status != "pending" {
		debug.Log("auth already completed, closing WebSocket", "status", status)
		return
	}

	// 接続切断を検知するための読み取りgoroutine
	readDone := make(chan struct{})
	go func() {
		defer close(readDone)
		for {
			// クライアントからのメッセージを読み取る（切断検知用）
			_, _, err := conn.Read(ctx)
			if err != nil {
				debug.Log("WebSocket read error (connection closed)", "error", err)
				return
			}
		}
	}()

	// 状態変更を監視するループ
	wsConf := cs.wsConfig()
	checkTicker := time.NewTicker(wsConf.PingIntervalDuration())
	defer checkTicker.Stop()

	for {
		select {
		case <-readDone:
			// クライアントが切断した
			debug.Log("WebSocket client disconnected")
			return
		case <-checkTicker.C:
			// 状態が変わっていないかチェック
			cs.sessionMu.RLock()
			currentStatus := session.Status
			currentError := session.ErrorMessage
			cs.sessionMu.RUnlock()

			if currentStatus != "pending" {
				// 状態が変わった（別のgoroutineで更新された）
				msg := WSMessage{Status: currentStatus}
				if currentError != "" {
					msg.Error = currentError
				}
				_ = cs.sendWSMessage(ctx, conn, msg)
				debug.Log("auth status changed, closing WebSocket", "status", currentStatus)
				return
			}
		}
	}
}

// sendWSMessage はWebSocketでメッセージを送信する
func (cs *CallbackServer) sendWSMessage(ctx context.Context, conn *websocket.Conn, msg WSMessage) error {
	data, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	return conn.Write(ctx, websocket.MessageText, data)
}

// handleWSDisconnect はWebSocket切断時の処理を行う
func (cs *CallbackServer) handleWSDisconnect() {
	cs.sessionMu.Lock()
	defer cs.sessionMu.Unlock()

	if cs.session == nil {
		return
	}

	// 切断時刻を記録
	now := time.Now()
	cs.session.WSConn = nil
	cs.session.DisconnectedAt = &now

	gracePeriod := cs.wsConfig().DisconnectGracePeriodDuration()

	// ユーザーに通知
	fmt.Fprintf(os.Stderr, "Browser disconnected. Waiting %s for reconnection...\n", gracePeriod)

	debug.Log("session temporarily disconnected",
		"state", "disconnected",
		"gracePeriod", gracePeriod.String(),
		"willTimeoutAt", now.Add(gracePeriod).Format("15:04:05"))
}

// broadcastAuthStatus はセッションのWebSocket接続に認証状態を配信する
func (cs *CallbackServer) broadcastAuthStatus(status, errorMsg string) {
	cs.sessionMu.RLock()
	session := cs.session
	cs.sessionMu.RUnlock()

	if session == nil || session.WSConn == nil {
		debug.Log("no WebSocket connection to broadcast")
		return
	}

	msg := WSMessage{Status: status}
	if errorMsg != "" {
		msg.Error = errorMsg
	}

	ctx := context.Background()
	if err := cs.sendWSMessage(ctx, session.WSConn, msg); err != nil {
		debug.Log("failed to broadcast to WebSocket", "error", err)
	}
	debug.Log("broadcast auth status", "status", status)
}

// handlePopup はポップアップウィンドウ用のページを表示する
func (cs *CallbackServer) handlePopup(w http.ResponseWriter, r *http.Request) {
	debug.Log("popup request received", "method", r.Method)

	profile := cs.configStore.CurrentProfile()
	relayServer := profile.RelayServer
	space := profile.Space
	domain := profile.Domain

	if relayServer == "" || space == "" || domain == "" {
		debug.Log("popup: configuration incomplete")
		renderPopupError(w, "設定が不完全です。ページを更新してください。")
		return
	}

	// 中継サーバーへリダイレクト
	cs.redirectToRelay(w, r, relayServer, space, domain)
}

// parseSpaceHost はスペースホストをパースする
// 入力例: myspace.backlog.jp
// 出力: space="myspace", domain="backlog.jp"
func parseSpaceHost(spaceHost string) (space, domain string, err error) {
	// 前後の空白を除去
	spaceHost = strings.TrimSpace(spaceHost)

	// https:// や http:// が含まれている場合は除去
	spaceHost = strings.TrimPrefix(spaceHost, "https://")
	spaceHost = strings.TrimPrefix(spaceHost, "http://")

	// 末尾のスラッシュやパスを除去
	if idx := strings.Index(spaceHost, "/"); idx != -1 {
		spaceHost = spaceHost[:idx]
	}

	parts := strings.SplitN(spaceHost, ".", 2)
	if len(parts) != 2 {
		return "", "", fmt.Errorf("形式が正しくありません（例: yourspace.backlog.jp）")
	}

	space = parts[0]
	domain = parts[1]

	if space == "" {
		return "", "", fmt.Errorf("スペース名が空です")
	}

	// サポートされているドメインかチェック
	if domain != "backlog.jp" && domain != "backlog.com" && domain != "backlogtool.com" {
		return "", "", fmt.Errorf("サポートされていないドメインです: %s", domain)
	}

	return space, domain, nil
}

func (cs *CallbackServer) handleCallback(w http.ResponseWriter, r *http.Request) {
	debug.Log("callback received", "method", r.Method, "path", r.URL.Path, "query", r.URL.RawQuery)

	var callbackError error
	var newStatus string
	var newErrorMsg string

	cs.once.Do(func() {
		code := r.URL.Query().Get("code")
		returnedState := r.URL.Query().Get("state")
		errorParam := r.URL.Query().Get("error")

		// セッションの状態を更新するヘルパー関数
		updateSessionStatus := func(status, errMsg string) {
			cs.sessionMu.Lock()
			if cs.session != nil {
				cs.session.Status = status
				cs.session.ErrorMessage = errMsg
			}
			cs.sessionMu.Unlock()
		}

		if errorParam != "" {
			errorDesc := r.URL.Query().Get("error_description")
			debug.Log("callback error received", "error", errorParam, "description", errorDesc)
			callbackError = fmt.Errorf("%s: %s", errorParam, errorDesc)
			cs.result <- CallbackResult{Error: callbackError}
			updateSessionStatus("error", callbackError.Error())
			newStatus = "error"
			newErrorMsg = callbackError.Error()
		} else if returnedState != cs.state {
			// state 検証
			debug.Log("state mismatch", "expected", cs.state, "got", returnedState)
			callbackError = fmt.Errorf("state mismatch: possible CSRF attack")
			cs.result <- CallbackResult{Error: callbackError}
			updateSessionStatus("error", callbackError.Error())
			newStatus = "error"
			newErrorMsg = callbackError.Error()
		} else if code == "" {
			debug.Log("callback received without code")
			callbackError = fmt.Errorf("no code received")
			cs.result <- CallbackResult{Error: callbackError}
			updateSessionStatus("error", callbackError.Error())
			newStatus = "error"
			newErrorMsg = callbackError.Error()
		} else {
			debug.Log("callback code received", "code_length", len(code))
			cs.result <- CallbackResult{Code: code}
			updateSessionStatus("success", "")
			newStatus = "success"
		}
	})

	// WebSocketクライアントに状態変更を通知
	if newStatus != "" {
		cs.broadcastAuthStatus(newStatus, newErrorMsg)
	}

	// Accept-Languageヘッダーから言語を判定
	isJapanese := isJapanesePreferred(r.Header.Get("Accept-Language"))

	// ポップアップ用の成功/エラーページを表示（自動クローズ付き）
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if callbackError != nil {
		if isJapanese {
			_, _ = fmt.Fprint(w, popupErrorPageJa)
		} else {
			_, _ = fmt.Fprint(w, popupErrorPageEn)
		}
	} else {
		if isJapanese {
			_, _ = fmt.Fprint(w, popupSuccessPageJa)
		} else {
			_, _ = fmt.Fprint(w, popupSuccessPageEn)
		}
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

// ポップアップ用成功ページ（即時クローズ）
const popupSuccessPageJa = `<!DOCTYPE html>
<html lang="ja">
<head><meta charset="utf-8"><title>認証成功</title></head>
<body><script>window.close();</script></body>
</html>`

const popupSuccessPageEn = `<!DOCTYPE html>
<html lang="en">
<head><meta charset="utf-8"><title>Authentication Successful</title></head>
<body><script>window.close();</script></body>
</html>`

// ポップアップ用エラーページ（即時クローズ）
const popupErrorPageJa = `<!DOCTYPE html>
<html lang="ja">
<head><meta charset="utf-8"><title>認証エラー</title></head>
<body><script>window.close();</script></body>
</html>`

const popupErrorPageEn = `<!DOCTYPE html>
<html lang="en">
<head><meta charset="utf-8"><title>Authentication Error</title></head>
<body><script>window.close();</script></body>
</html>`

// renderPopupError はポップアップ用のエラーページを表示する
func renderPopupError(w http.ResponseWriter, message string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = fmt.Fprintf(w, `<!DOCTYPE html>
<html lang="ja">
<head>
<meta charset="utf-8">
<title>エラー</title>
<style>
body {
  font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, "Hiragino Sans", sans-serif;
  display: flex;
  justify-content: center;
  align-items: center;
  min-height: 100vh;
  margin: 0;
  background: linear-gradient(135deg, #667eea 0%%, #764ba2 100%%);
}
.container {
  text-align: center;
  background: white;
  padding: 3rem;
  border-radius: 1rem;
  box-shadow: 0 10px 40px rgba(0,0,0,0.2);
  max-width: 400px;
}
.icon { font-size: 4rem; margin-bottom: 1rem; color: #f44336; }
h1 { color: #333; margin: 0 0 1rem 0; font-size: 1.5rem; }
p { color: #666; margin: 0; line-height: 1.6; }
</style>
</head>
<body>
<div class="container">
  <div class="icon">✗</div>
  <h1>エラー</h1>
  <p>%s</p>
</div>
</body>
</html>`, message)
}

// FindFreePort は空いているポートを探す
func FindFreePort() (int, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer func() { _ = listener.Close() }()
	return listener.Addr().(*net.TCPAddr).Port, nil
}
