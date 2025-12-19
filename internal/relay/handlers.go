package relay

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"

	"github.com/yacchi/backlog-cli/internal/api"
	"github.com/yacchi/backlog-cli/internal/config"
)

// ErrorResponse はエラーレスポンス
type ErrorResponse struct {
	Error       string `json:"error"`
	Description string `json:"error_description,omitempty"`
}

// TokenRequest はトークンリクエスト
type TokenRequest struct {
	GrantType    string `json:"grant_type"`
	Code         string `json:"code,omitempty"`
	RefreshToken string `json:"refresh_token,omitempty"`
	Domain       string `json:"domain"`
	Space        string `json:"space"`
	State        string `json:"state,omitempty"` // セッション追跡用（auth_startで取得した値）
}

// TokenResponse はトークンレスポンス
type TokenResponse struct {
	AccessToken  string `json:"access_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
	RefreshToken string `json:"refresh_token"`
}

// AuthStartResponse は認証開始レスポンス
type AuthStartResponse struct {
	AuthURL string `json:"auth_url"`
	State   string `json:"state"` // セッション追跡用（CLIが/auth/tokenに送信）
}

func (s *Server) writeError(w http.ResponseWriter, status int, err, desc string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(ErrorResponse{
		Error:       err,
		Description: desc,
	})
}

func (s *Server) handleAuthStart(w http.ResponseWriter, r *http.Request) {
	// パラメータ取得
	domain := r.URL.Query().Get("domain")
	space := r.URL.Query().Get("space")
	portStr := r.URL.Query().Get("port")
	project := r.URL.Query().Get("project")

	clientIP := ""
	if ip := getClientIP(r); ip != nil {
		clientIP = ip.String()
	}

	// バリデーション
	if domain == "" || space == "" || portStr == "" {
		s.writeError(w, http.StatusBadRequest, "invalid_request", "domain, space, and port are required")
		return
	}

	port, err := strconv.Atoi(portStr)
	if err != nil || port < 1024 || port > 65535 {
		s.writeError(w, http.StatusBadRequest, "invalid_request", "port must be between 1024 and 65535")
		return
	}

	// スペース制限チェック
	if err := s.accessControl.CheckSpace(space); err != nil {
		s.auditLogger.Log(AuditEvent{
			Action:    AuditActionAccessDenied,
			Space:     space,
			Domain:    domain,
			Project:   project,
			ClientIP:  clientIP,
			UserAgent: r.UserAgent(),
			Result:    "error",
			Error:     err.Error(),
		})
		s.writeError(w, http.StatusForbidden, "access_denied", err.Error())
		return
	}

	// プロジェクト制限チェック
	if err := s.accessControl.CheckProject(project); err != nil {
		s.auditLogger.Log(AuditEvent{
			Action:    AuditActionAccessDenied,
			Space:     space,
			Domain:    domain,
			Project:   project,
			ClientIP:  clientIP,
			UserAgent: r.UserAgent(),
			Result:    "error",
			Error:     err.Error(),
		})
		s.writeError(w, http.StatusForbidden, "access_denied", err.Error())
		return
	}

	// Backlog設定を取得
	backlogCfg := s.findBacklogConfig(domain)
	if backlogCfg == nil {
		s.writeError(w, http.StatusBadRequest, "invalid_request", fmt.Sprintf("domain '%s' is not supported", domain))
		return
	}

	// state生成
	state, err := generateState()
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "server_error", "failed to generate state")
		return
	}

	// JWTトークン作成（/auth/callbackでブラウザに設定するCookie用）
	token, err := s.createSessionToken(port, state, domain, space, project)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "server_error", "failed to create session")
		return
	}

	// 監査ログ
	s.auditLogger.Log(AuditEvent{
		SessionID: ExtractSessionID(state),
		Action:    AuditActionAuthStart,
		Space:     space,
		Domain:    domain,
		Project:   project,
		ClientIP:  clientIP,
		UserAgent: r.UserAgent(),
		Result:    "success",
	})

	// Backlog認可URLを構築（stateにJWTトークンを含める）
	redirectURI := s.buildCallbackURL()
	authURL := fmt.Sprintf("https://%s.%s/OAuth2AccessRequest.action?response_type=code&client_id=%s&redirect_uri=%s&state=%s",
		space,
		domain,
		url.QueryEscape(backlogCfg.ClientID()),
		url.QueryEscape(redirectURI),
		url.QueryEscape(token), // stateとしてJWTトークンを使用
	)

	// JSON APIとしてauth_urlとstateを返す
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(AuthStartResponse{
		AuthURL: authURL,
		State:   state, // CLIが/auth/tokenに送信するセッションID用
	})
}

func (s *Server) handleAuthCallback(w http.ResponseWriter, r *http.Request) {
	// パラメータ取得
	code := r.URL.Query().Get("code")
	stateToken := r.URL.Query().Get("state") // JWTトークンが入っている
	errorParam := r.URL.Query().Get("error")

	clientIP := ""
	if ip := getClientIP(r); ip != nil {
		clientIP = ip.String()
	}

	// Backlogからのエラー
	if errorParam != "" {
		errorDesc := r.URL.Query().Get("error_description")
		s.auditLogger.Log(AuditEvent{
			Action:    AuditActionAuthCallback,
			ClientIP:  clientIP,
			UserAgent: r.UserAgent(),
			Result:    "error",
			Error:     fmt.Sprintf("%s: %s", errorParam, errorDesc),
		})
		s.renderErrorPage(w, "Authorization Failed", errorDesc)
		return
	}

	if code == "" || stateToken == "" {
		s.auditLogger.Log(AuditEvent{
			Action:    AuditActionAuthCallback,
			ClientIP:  clientIP,
			UserAgent: r.UserAgent(),
			Result:    "error",
			Error:     "missing code or state parameter",
		})
		s.renderErrorPage(w, "Invalid Request", "Missing code or state parameter")
		return
	}

	// stateパラメータはJWTトークンなのでパースする
	claims, err := s.parseSessionToken(stateToken)
	if err != nil {
		s.auditLogger.Log(AuditEvent{
			Action:    AuditActionAuthCallback,
			ClientIP:  clientIP,
			UserAgent: r.UserAgent(),
			Result:    "error",
			Error:     "invalid state token: " + err.Error(),
		})
		s.renderErrorPage(w, "Session Invalid", "Please try logging in again")
		return
	}

	// 監査ログ（成功）
	s.auditLogger.Log(AuditEvent{
		SessionID: ExtractSessionID(claims.State),
		Action:    AuditActionAuthCallback,
		Space:     claims.Space,
		Domain:    claims.Domain,
		Project:   claims.Project,
		ClientIP:  clientIP,
		UserAgent: r.UserAgent(),
		Result:    "success",
	})

	// CLIローカルサーバーへリダイレクト
	localURL := fmt.Sprintf("http://localhost:%d/callback?code=%s", claims.Port, url.QueryEscape(code))
	http.Redirect(w, r, localURL, http.StatusFound)
}

func (s *Server) handleAuthToken(w http.ResponseWriter, r *http.Request) {
	clientIP := ""
	if ip := getClientIP(r); ip != nil {
		clientIP = ip.String()
	}

	// リクエストボディをパース
	var req TokenRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeError(w, http.StatusBadRequest, "invalid_request", "Invalid JSON body")
		return
	}

	// バリデーション
	if req.Domain == "" || req.Space == "" {
		s.writeError(w, http.StatusBadRequest, "invalid_request", "domain and space are required")
		return
	}

	backlogCfg := s.findBacklogConfig(req.Domain)
	if backlogCfg == nil {
		s.writeError(w, http.StatusBadRequest, "invalid_request", fmt.Sprintf("domain '%s' is not supported", req.Domain))
		return
	}

	var tokenResp *TokenResponse
	var err error
	var auditAction string

	switch req.GrantType {
	case "authorization_code":
		auditAction = AuditActionTokenExchange
		if req.Code == "" {
			s.writeError(w, http.StatusBadRequest, "invalid_request", "code is required for authorization_code grant")
			return
		}
		tokenResp, err = s.exchangeCode(backlogCfg, req.Space, req.Code)

	case "refresh_token":
		auditAction = AuditActionTokenRefresh
		if req.RefreshToken == "" {
			s.writeError(w, http.StatusBadRequest, "invalid_request", "refresh_token is required for refresh_token grant")
			return
		}
		tokenResp, err = s.refreshToken(backlogCfg, req.Space, req.RefreshToken)

	default:
		s.writeError(w, http.StatusBadRequest, "unsupported_grant_type", "Supported: authorization_code, refresh_token")
		return
	}

	// リクエストのStateからセッションIDを取得
	sessionID := ExtractSessionID(req.State)

	if err != nil {
		s.auditLogger.Log(AuditEvent{
			SessionID: sessionID,
			Action:    auditAction,
			Space:     req.Space,
			Domain:    req.Domain,
			ClientIP:  clientIP,
			UserAgent: r.UserAgent(),
			Result:    "error",
			Error:     err.Error(),
		})
		s.writeError(w, http.StatusBadGateway, "upstream_error", err.Error())
		return
	}

	// 認証ユーザー情報を取得
	var userID, userName, userEmail string
	if user, err := api.FetchCurrentUser(req.Domain, req.Space, tokenResp.AccessToken); err == nil {
		userID = user.UserId.Value
		userName = user.Name.Value
		userEmail = user.MailAddress.Value
	}
	// ユーザー情報取得に失敗しても、トークン発行は成功しているので続行

	// 監査ログ（成功）
	s.auditLogger.Log(AuditEvent{
		SessionID: sessionID,
		Action:    auditAction,
		UserID:    userID,
		UserName:  userName,
		UserEmail: userEmail,
		Space:     req.Space,
		Domain:    req.Domain,
		ClientIP:  clientIP,
		UserAgent: r.UserAgent(),
		Result:    "success",
	})

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(tokenResp)
}

func (s *Server) findBacklogConfig(domain string) *config.ResolvedBacklogApp {
	return s.cfg.BacklogApp(domain)
}

func (s *Server) buildCallbackURL() string {
	server := s.cfg.Server()
	if server.BaseURL != "" {
		return server.BaseURL + "/auth/callback"
	}
	// デフォルト（開発用）
	return fmt.Sprintf("http://localhost:%d/auth/callback", server.Port)
}

func generateState() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(b), nil
}

func (s *Server) renderErrorPage(w http.ResponseWriter, title, message string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusBadRequest)
	fmt.Fprintf(w, `<!DOCTYPE html>
<html>
<head><title>%s</title></head>
<body>
<h1>%s</h1>
<p>%s</p>
<p>You can close this window.</p>
</body>
</html>`, title, title, message)
}

func (s *Server) exchangeCode(cfg *config.ResolvedBacklogApp, space, code string) (*TokenResponse, error) {
	return s.requestToken(cfg, space, url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"redirect_uri":  {s.buildCallbackURL()},
		"client_id":     {cfg.ClientID()},
		"client_secret": {cfg.ClientSecret()},
	})
}

func (s *Server) refreshToken(cfg *config.ResolvedBacklogApp, space, refreshToken string) (*TokenResponse, error) {
	return s.requestToken(cfg, space, url.Values{
		"grant_type":    {"refresh_token"},
		"refresh_token": {refreshToken},
		"client_id":     {cfg.ClientID()},
		"client_secret": {cfg.ClientSecret()},
	})
}

func (s *Server) requestToken(cfg *config.ResolvedBacklogApp, space string, params url.Values) (*TokenResponse, error) {
	tokenURL := fmt.Sprintf("https://%s.%s/api/v2/oauth2/token", space, cfg.Domain())

	resp, err := http.PostForm(tokenURL, params)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("token request failed: %s", string(body))
	}

	var tokenResp TokenResponse
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &tokenResp, nil
}
