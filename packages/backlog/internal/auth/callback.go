package auth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/yacchi/backlog-cli/packages/backlog/internal/config"
	"github.com/yacchi/backlog-cli/packages/backlog/internal/debug"
	"github.com/yacchi/backlog-cli/packages/backlog/internal/gen/auth/v1/authv1connect"
	"github.com/yacchi/backlog-cli/packages/backlog/internal/ui"
	"github.com/yacchi/backlog-cli/packages/web"
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
	Port              int
	State             string          // CLI が生成した state
	ConfigStore       *config.Store   // 設定の読み書き用
	Reuse             bool            // true の場合、確認画面をスキップして即座にリダイレクト
	ForceBundleUpdate bool            // バンドル更新チェックを強制する（デバッグ用）
	Ctx               context.Context // コマンドのContext（シグナル処理用）
}

// Session は認証セッションを表す。
// ストリーム接続状態は CallbackServer 側で別に管理しており、
// 複数のセッション/タブが並行しても他方を壊さないようにしている。
type Session struct {
	ID             string    // セッションID
	CreatedAt      time.Time // 作成時刻
	LastActivityAt time.Time // 最終アクティビティ時刻
	Status         string    // pending/success/error
	ErrorMessage   string    // エラーメッセージ
}

// CallbackServer はCLIのローカルコールバックサーバー
type CallbackServer struct {
	port              int
	server            *http.Server
	result            chan CallbackResult
	listener          net.Listener
	once              sync.Once
	state             string
	configStore       *config.Store
	reuse             bool
	forceBundleUpdate bool
	ctx               context.Context // コマンドのContext

	// セッション管理。cs.session は Cookie 突き合わせ用の代表セッションで
	// ステータス（pending/success/error）を保持する。同一ブラウザの再読込や
	// 別タブからの接続によって cs.session が差し替わっても、ストリーム接続の
	// カウントは下記の stream* フィールドが独立に持つため壊れない。
	session            *Session      // 現在の代表セッション
	sessionMu          sync.RWMutex  // セッションアクセス用ミューテックス
	sessionEstablished bool          // セッションが確立されたことがあるか
	cancelCheck        chan struct{} // チェッカーを停止するためのチャネル
	cancelOnce         sync.Once     // cancelCheck を一度だけ閉じるため
	statusNotify       chan struct{} // ステータス変更通知用チャネル

	// ストリーム接続状態。セッション ID とは独立にカウントする。
	streamMu            sync.Mutex
	activeStreams       int        // 現在接続中のストリーム数
	streamEverConnected bool       // 一度でもストリームが接続されたか
	disconnectedAt      *time.Time // すべてのストリームが切れた時刻（nil = 接続中もしくは未接続）
}

// authConfig は認証設定を取得する
func (cs *CallbackServer) authConfig() *config.ResolvedAuth {
	return &cs.configStore.Resolved().Auth
}

// keepaliveConfig はKeepalive設定を取得する
func (cs *CallbackServer) keepaliveConfig() *config.ResolvedAuthKeepalive {
	return &cs.authConfig().Keepalive
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
		port:              actualPort,
		result:            make(chan CallbackResult, 1),
		listener:          listener,
		state:             opts.State,
		configStore:       opts.ConfigStore,
		reuse:             opts.Reuse,
		forceBundleUpdate: opts.ForceBundleUpdate,
		ctx:               ctx,
		cancelCheck:       make(chan struct{}),
		statusNotify:      make(chan struct{}, 1), // バッファ付き（ノンブロッキング通知用）
	}

	debug.Log("callback server created",
		"port", actualPort,
		"address", addr,
		"pid", os.Getpid(),
		"state", stateFingerprint(opts.State),
	)

	cs.server = &http.Server{
		Handler: cs.setupRoutes(),
	}

	return cs, nil
}

func (cs *CallbackServer) setupRoutes() http.Handler {
	mux := http.NewServeMux()

	// Connect RPCハンドラーを登録
	path, handler := authv1connect.NewAuthServiceHandler(cs)
	mux.Handle(path, handler)

	// HTTPエンドポイント（HTML/リダイレクト用）
	mux.HandleFunc("/auth/popup", cs.handlePopup)
	mux.HandleFunc("/callback", cs.handleCallback)

	assets, err := web.Assets()
	if err != nil {
		debug.Log("ui assets unavailable", "error", err)
		mux.Handle("/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "SPA assets not available", http.StatusInternalServerError)
		}))
		return mux
	}

	spaHandler := ui.SPAHandler(assets)
	mux.Handle("/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Ensure a session cookie exists before the SPA boots and opens streaming.
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

// createSession は新しいセッションを作成する。
// 既存セッションのステータス（pending/success/error）は引き継ぎ、
// ID と作成時刻のみを更新する。ストリーム接続状態は cs.streamMu 配下で
// 別管理しており、ここでは触らない。
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
	// 既存セッションのステータス継承（複数タブ・リロード対応）
	if cs.session != nil {
		session.Status = cs.session.Status
		session.ErrorMessage = cs.session.ErrorMessage
		// 既にセッションが確立されている場合は作成時刻も引き継ぐ
		// （session.timeout の起点をリセットしないため）
		if cs.sessionEstablished {
			session.CreatedAt = cs.session.CreatedAt
		}
	}
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

// sessionCookie はセッションCookieを生成する
func (cs *CallbackServer) sessionCookie(sessionID string) *http.Cookie {
	timeout := cs.sessionConfig().TimeoutDuration()
	return &http.Cookie{
		Name:     sessionCookieName,
		Value:    sessionID,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(timeout.Seconds()),
	}
}

// setSessionCookie はセッションCookieを設定する
func (cs *CallbackServer) setSessionCookie(w http.ResponseWriter, sessionID string) {
	http.SetCookie(w, cs.sessionCookie(sessionID))
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
	var noConnectionSince *time.Time // 接続がない状態が始まった時刻

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
			if session != nil {
				status = session.Status
				createdAt = session.CreatedAt
			}
			cs.sessionMu.RUnlock()

			cs.streamMu.Lock()
			hasConnection := cs.activeStreams > 0
			everConnected := cs.streamEverConnected
			cs.streamMu.Unlock()

			// セッションがまだ確立されていない場合
			if !established {
				// 初期状態：ブラウザがまだアクセスしていない
				continue
			}

			// 10回に1回状態をログ出力
			if checkCount%10 == 0 {
				noConnDuration := ""
				if noConnectionSince != nil {
					noConnDuration = time.Since(*noConnectionSince).String()
				}
				debug.Log("session checker tick",
					"count", checkCount,
					"status", status,
					"hasConnection", hasConnection,
					"everConnected", everConnected,
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
				// セッションステータスを更新してストリームに通知
				cs.setSessionError("authentication timeout (session expired)")
				cs.once.Do(func() {
					cs.result <- CallbackResult{
						Error: fmt.Errorf("authentication timeout (session expired)"),
					}
				})
				return
			}

			// 接続状態のチェック
			if hasConnection {
				// 接続中：タイマーをリセット
				noConnectionSince = nil
				continue
			}

			// 接続がない状態
			now := time.Now()
			if noConnectionSince == nil {
				// 接続がない状態が始まった
				noConnectionSince = &now
				debug.Log("no connection detected, starting grace period timer")
			}

			// 接続がない状態が一定時間続いたらタイムアウト
			elapsed := time.Since(*noConnectionSince)
			if !everConnected {
				connectTimeout := cs.keepaliveConfig().ConnectTimeoutDuration()
				if elapsed > connectTimeout {
					debug.Log("browser connect timeout expired",
						"elapsed", elapsed.String(),
						"connectTimeout", connectTimeout.String())
					cs.setSessionError("authentication cancelled (browser failed to connect)")
					cs.once.Do(func() {
						cs.result <- CallbackResult{
							Error: fmt.Errorf("authentication cancelled (browser failed to connect)"),
						}
					})
					return
				}
				continue
			}

			gracePeriod := cs.keepaliveConfig().GracePeriodDuration()
			if elapsed > gracePeriod {
				debug.Log("no connection grace period expired",
					"elapsed", elapsed.String(),
					"grace", gracePeriod.String())
				// セッションステータスを更新してストリームに通知
				cs.setSessionError("authentication cancelled (browser closed or navigated away)")
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
	debug.Log("callback server shutdown", "port", cs.port)
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

// ensureSessionFromHeader はConnect RPC用にHTTPヘッダーからセッションを確保する
// セキュリティ: Cookie検証を厳格に行い、セッション乗っ取りを防止
func (cs *CallbackServer) ensureSessionFromHeader(reqHeader http.Header, respHeader http.Header) *Session {
	// CookieヘッダーからセッションIDを取得
	cookieHeader := reqHeader.Get("Cookie")
	sessionID := extractSessionIDFromCookie(cookieHeader)

	cs.sessionMu.RLock()
	session := cs.session
	cs.sessionMu.RUnlock()

	// 既存セッションがあり、IDが一致する場合のみ再利用
	if session != nil && sessionID != "" && session.ID == sessionID {
		cs.updateSessionActivity()
		return session
	}

	// Cookie が無い、または不一致の場合は新規セッション作成
	// これにより、別クライアントが既存セッションに紐付くことを防止
	newSession, err := cs.createSession()
	if err != nil {
		debug.Log("failed to create session", "error", err)
		return nil
	}

	// レスポンスヘッダーにSet-Cookieを設定
	respHeader.Add("Set-Cookie", cs.sessionCookie(newSession.ID).String())
	return newSession
}

// extractSessionIDFromCookie はCookieヘッダー文字列からセッションIDを抽出する
func extractSessionIDFromCookie(cookieHeader string) string {
	if cookieHeader == "" {
		return ""
	}
	header := http.Header{"Cookie": []string{cookieHeader}}
	req := &http.Request{Header: header}
	cookie, err := req.Cookie(sessionCookieName)
	if err != nil {
		return ""
	}
	return cookie.Value
}

// sessionStatus はセッションのステータスとエラーメッセージを返す
func (cs *CallbackServer) sessionStatus() (status string, errorMsg string) {
	cs.sessionMu.RLock()
	defer cs.sessionMu.RUnlock()
	if cs.session == nil {
		return "pending", ""
	}
	return cs.session.Status, cs.session.ErrorMessage
}

// handleStreamConnect はストリーム接続時の処理を行う。
// 同時に複数のストリームが接続される可能性があるため、カウント形式で管理する。
// 初回接続時のみユーザーに通知する。
func (cs *CallbackServer) handleStreamConnect() {
	cs.streamMu.Lock()
	cs.activeStreams++
	wasFirst := !cs.streamEverConnected
	cs.streamEverConnected = true
	cs.disconnectedAt = nil
	active := cs.activeStreams
	cs.streamMu.Unlock()

	debug.Log("stream connected", "active_streams", active, "was_first", wasFirst)

	if wasFirst {
		// ユーザーに通知
		fmt.Fprintf(os.Stderr, "Browser connected.\n")
	}
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

	debug.Log("redirecting to relay server",
		"url", redirectURL,
		"port", cs.port,
		"state", stateFingerprint(cs.state),
	)
	http.Redirect(w, r, redirectURL, http.StatusFound)
}

// stateFingerprint は state の短縮表記をログ用に返す。
// 完全な state はセキュリティ上ログに残さないが、突き合わせには使いたいため
// 先頭8文字 + 末尾4文字だけを残す。
func stateFingerprint(state string) string {
	if len(state) <= 12 {
		return state
	}
	return state[:8] + "..." + state[len(state)-4:]
}

// notifyStatus は認証状態変更をストリーミングリスナーに通知する
func (cs *CallbackServer) notifyStatus(status string) {
	select {
	case cs.statusNotify <- struct{}{}:
		debug.Log("notified streaming listeners", "status", status)
	default:
		// チャネルがいっぱいの場合はスキップ（既に通知済み）
	}
}

// setSessionError はセッションのエラーステータスを設定し、ストリームに通知する
func (cs *CallbackServer) setSessionError(errorMsg string) {
	cs.sessionMu.Lock()
	if cs.session != nil {
		cs.session.Status = "error"
		cs.session.ErrorMessage = errorMsg
	}
	cs.sessionMu.Unlock()
	cs.notifyStatus("error")
}

// handleStreamDisconnect はストリーム切断時の処理を行う。
// アクティブストリームのカウントを1減らし、すべてのストリームが切れた場合のみ
// 切断時刻を記録する。途中で別のストリームに置き換わっただけ（同時に複数接続中の
// うち1本が切れた場合）にはタイムアウトを起動しない。
func (cs *CallbackServer) handleStreamDisconnect() {
	// 認証完了済みなら通知不要（接続カウントだけ減らす）
	status, _ := cs.sessionStatus()

	cs.streamMu.Lock()
	cs.activeStreams--
	if cs.activeStreams < 0 {
		cs.activeStreams = 0
	}
	remaining := cs.activeStreams
	allDisconnected := remaining == 0
	if allDisconnected && status == "pending" {
		now := time.Now()
		cs.disconnectedAt = &now
	}
	cs.streamMu.Unlock()

	debug.Log("stream disconnected", "remaining_streams", remaining, "status", status)

	if !allDisconnected || status != "pending" {
		// まだ別のストリームが生きている、または認証完了済みのため通知不要。
		return
	}

	gracePeriod := cs.keepaliveConfig().GracePeriodDuration()
	// ユーザーに通知
	fmt.Fprintf(os.Stderr, "Browser disconnected. Waiting %s for reconnection...\n", gracePeriod)
	debug.Log("waiting for stream reconnect",
		"gracePeriod", gracePeriod.String(),
		"willTimeoutAt", time.Now().Add(gracePeriod).Format("15:04:05"))
}

// handlePopup はポップアップウィンドウ用のページを表示する
func (cs *CallbackServer) handlePopup(w http.ResponseWriter, r *http.Request) {
	debug.Log("popup request received", "method", r.Method)

	// この経路で発行される 302 とそのレスポンスはブラウザにキャッシュさせない。
	// 残しておくと bfcache 等で旧 callback URL が再 GET される原因になる。
	w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate, max-age=0")
	w.Header().Set("Pragma", "no-cache")

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
		} else if returnedState != cs.state {
			// state 検証
			debug.Log("state mismatch", "expected", cs.state, "got", returnedState)
			callbackError = fmt.Errorf("state mismatch: possible CSRF attack")
			cs.result <- CallbackResult{Error: callbackError}
			updateSessionStatus("error", callbackError.Error())
			newStatus = "error"
		} else if code == "" {
			debug.Log("callback received without code")
			callbackError = fmt.Errorf("no code received")
			cs.result <- CallbackResult{Error: callbackError}
			updateSessionStatus("error", callbackError.Error())
			newStatus = "error"
		} else {
			debug.Log("callback code received", "code_length", len(code))
			cs.result <- CallbackResult{Code: code}
			updateSessionStatus("success", "")
			newStatus = "success"
		}
	})

	// ストリーミングリスナーに状態変更を通知
	if newStatus != "" {
		cs.notifyStatus(newStatus)
	}

	// Accept-Languageヘッダーから言語を判定
	isJapanese := isJapanesePreferred(r.Header.Get("Accept-Language"))

	// ポップアップ用の成功/エラーページを表示（自動クローズ付き）。
	// bfcache 等で再表示されたときに同じ /callback?code=...&state=... を再度
	// GET しに行かないよう、Cache-Control を厳密に no-store にし、
	// HTML 側でも history.replaceState によりクエリを落としてから close する。
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate, max-age=0")
	w.Header().Set("Pragma", "no-cache")
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

// popupCloseScript はポップアップを安全にクローズするスクリプト。
//
//  1. history.replaceState で /callback?code=...&state=... のクエリを落とす。
//     これで F5 / 戻る等で /callback?code=... を再 GET されない。
//  2. window.close() を試みる。Chrome は scripted popup でも cross-origin を経由した
//     window の close を拒否することがある（Backlog 認証画面を経由するためここに該当）。
//  3. 短時間待っても window が閉じていない場合は about:blank に置換する。
//     こうすると、ローカルサーバーが既に停止していても popup が
//     ERR_CONNECTION_REFUSED の表示で残るのを防げる。
const popupCloseScript = `<script>
(function(){
  try {
    if (window.history && window.history.replaceState) {
      window.history.replaceState({}, document.title, window.location.pathname + "#done");
    }
  } catch (e) {}
  try { window.close(); } catch (e) {}
  // close が拒否された場合に備えて、ローカルサーバーに依存しない URL に置換する。
  setTimeout(function(){
    if (!window.closed) {
      try { window.location.replace("about:blank"); } catch (e) {}
    }
  }, 250);
})();
</script>`

// ポップアップ用成功ページ（即時クローズ）
const popupSuccessPageJa = `<!DOCTYPE html>
<html lang="ja">
<head><meta charset="utf-8"><title>認証成功</title></head>
<body>` + popupCloseScript + `</body>
</html>`

const popupSuccessPageEn = `<!DOCTYPE html>
<html lang="en">
<head><meta charset="utf-8"><title>Authentication Successful</title></head>
<body>` + popupCloseScript + `</body>
</html>`

// ポップアップ用エラーページ（即時クローズ）
const popupErrorPageJa = `<!DOCTYPE html>
<html lang="ja">
<head><meta charset="utf-8"><title>認証エラー</title></head>
<body>` + popupCloseScript + `</body>
</html>`

const popupErrorPageEn = `<!DOCTYPE html>
<html lang="en">
<head><meta charset="utf-8"><title>Authentication Error</title></head>
<body>` + popupCloseScript + `</body>
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
