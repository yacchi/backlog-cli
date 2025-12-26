package auth

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"connectrpc.com/connect"
	authv1 "github.com/yacchi/backlog-cli/gen/go/auth/v1"
	"github.com/yacchi/backlog-cli/gen/go/auth/v1/authv1connect"
	"github.com/yacchi/backlog-cli/internal/api"
	"github.com/yacchi/backlog-cli/internal/config"
	"github.com/yacchi/backlog-cli/internal/debug"
)

// CallbackServerがAuthServiceHandlerを実装していることをコンパイル時に検証
var _ authv1connect.AuthServiceHandler = (*CallbackServer)(nil)

// GetConfig は設定を取得する
func (cs *CallbackServer) GetConfig(
	ctx context.Context,
	req *connect.Request[authv1.GetConfigRequest],
) (*connect.Response[authv1.GetConfigResponse], error) {
	resp := connect.NewResponse(&authv1.GetConfigResponse{})

	// セッションを確保（Cookieベース）
	session := cs.ensureSessionFromHeader(req.Header(), resp.Header())
	if session == nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to create session"))
	}

	profile := cs.configStore.CurrentProfile()
	spaceHost := ""
	if profile.Space != "" && profile.Domain != "" {
		spaceHost = fmt.Sprintf("%s.%s", profile.Space, profile.Domain)
	}

	resp.Msg.Space = profile.Space
	resp.Msg.Domain = profile.Domain
	resp.Msg.RelayServer = profile.RelayServer
	resp.Msg.SpaceHost = spaceHost
	resp.Msg.Configured = profile.Space != "" && profile.Domain != "" && profile.RelayServer != ""

	// 現在の認証タイプを取得
	resolved := cs.configStore.Resolved()
	cred := resolved.GetActiveCredential()
	if cred != nil {
		switch cred.GetAuthType() {
		case config.AuthTypeOAuth:
			resp.Msg.CurrentAuthType = "oauth"
		case config.AuthTypeAPIKey:
			resp.Msg.CurrentAuthType = "apikey"
		}
	}

	debug.Log("GetConfig called", "configured", resp.Msg.Configured, "current_auth_type", resp.Msg.CurrentAuthType)
	return resp, nil
}

// Configure は設定を保存する
func (cs *CallbackServer) Configure(
	ctx context.Context,
	req *connect.Request[authv1.ConfigureRequest],
) (*connect.Response[authv1.ConfigureResponse], error) {
	resp := connect.NewResponse(&authv1.ConfigureResponse{})

	// セッションを確保（Cookieベース）
	session := cs.ensureSessionFromHeader(req.Header(), resp.Header())
	if session == nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to create session"))
	}

	spaceHost := req.Msg.SpaceHost
	relayServer := req.Msg.RelayServer

	debug.Log("Configure called", "space_host", spaceHost, "relay_server", relayServer)

	// スペースホストをパース
	space, domain, err := parseSpaceHost(spaceHost)
	if err != nil {
		debug.Log("invalid space host", "error", err)
		resp.Msg.Success = false
		resp.Msg.Error = stringPtr("無効なスペース形式です: " + err.Error())
		return resp, nil
	}

	// well-known チェック
	client := NewClient(relayServer)
	wellKnown, err := client.FetchWellKnown()
	if err != nil {
		debug.Log("failed to fetch well-known", "error", err)
		resp.Msg.Success = false
		resp.Msg.Error = stringPtr(fmt.Sprintf("リレーサーバーに接続できません: %v", err))
		return resp, nil
	}

	// ドメインがサポートされているかチェック
	if !slices.Contains(wellKnown.SupportedDomains, domain) {
		debug.Log("domain not supported", "domain", domain, "supported", wellKnown.SupportedDomains)
		resp.Msg.Success = false
		resp.Msg.Error = stringPtr(fmt.Sprintf("このリレーサーバーは %s をサポートしていません（サポート: %s）",
			domain, strings.Join(wellKnown.SupportedDomains, ", ")))
		return resp, nil
	}

	allowedDomain := space + "." + domain
	if bundle := config.FindTrustedBundle(cs.configStore, allowedDomain); bundle != nil {
		cacheDir, cacheErr := cs.configStore.GetCacheDir()
		if cacheErr != nil {
			debug.Log("failed to resolve cache dir", "error", cacheErr)
		}
		info, err := config.VerifyRelayInfo(ctx, relayServer, allowedDomain, bundle.BundleToken, bundle.RelayKeys, config.RelayInfoOptions{
			CacheDir:      cacheDir,
			CertsCacheTTL: bundle.CertsCacheTTL,
		})
		if err != nil {
			debug.Log("failed to verify relay info", "error", err)
			resp.Msg.Success = false
			resp.Msg.Error = stringPtr(fmt.Sprintf("リレーサーバーの検証に失敗しました: %v", err))
			return resp, nil
		}

		if err := config.CheckBundleUpdate(info, bundle, time.Now().UTC(), cs.forceBundleUpdate); err != nil {
			var updateErr *config.BundleUpdateRequiredError
			if errors.As(err, &updateErr) {
				bundleURL, urlErr := config.BuildRelayBundleURL(relayServer, allowedDomain)
				if urlErr != nil {
					resp.Msg.Success = false
					resp.Msg.Error = stringPtr(fmt.Sprintf("バンドルURLの生成に失敗しました: %v", urlErr))
					return resp, nil
				}
				debug.Log("fetching relay bundle", "url", bundleURL, "allowed_domain", allowedDomain)
				_, updateErr := config.FetchAndImportRelayBundle(ctx, cs.configStore, relayServer, allowedDomain, bundle.BundleToken, config.BundleFetchOptions{
					CacheDir:          cacheDir,
					AllowNameMismatch: false,
					NoDefaults:        true,
				})
				if updateErr != nil {
					debug.Log("bundle update failed", "error", updateErr)
					resp.Msg.Success = false
					resp.Msg.Error = stringPtr(fmt.Sprintf("バンドル更新に失敗しました: %v", updateErr))
					return resp, nil
				}
				debug.Log("bundle update completed", "allowed_domain", allowedDomain)
			} else {
				debug.Log("failed to check bundle update", "error", err)
				resp.Msg.Success = false
				resp.Msg.Error = stringPtr(fmt.Sprintf("バンドル更新の確認に失敗しました: %v", err))
				return resp, nil
			}
		}

		debug.Log("relay info verified", "allowed_domain", allowedDomain)
	} else {
		debug.Log("trusted bundle not found; skipping relay info check", "allowed_domain", allowedDomain)
	}

	// 設定保存
	profileName := cs.configStore.GetActiveProfile()
	if err := cs.configStore.SetProfileValue(config.LayerUser, profileName, "relay_server", relayServer); err != nil {
		debug.Log("failed to save relay_server", "error", err)
		resp.Msg.Success = false
		resp.Msg.Error = stringPtr(fmt.Sprintf("設定の保存に失敗しました: %v", err))
		return resp, nil
	}
	if err := cs.configStore.SetProfileValue(config.LayerUser, profileName, "space", space); err != nil {
		debug.Log("failed to save space", "error", err)
	}
	if err := cs.configStore.SetProfileValue(config.LayerUser, profileName, "domain", domain); err != nil {
		debug.Log("failed to save domain", "error", err)
	}

	if err := cs.configStore.Save(ctx); err != nil {
		debug.Log("failed to save config", "error", err)
		resp.Msg.Success = false
		resp.Msg.Error = stringPtr(fmt.Sprintf("設定の保存に失敗しました: %v", err))
		return resp, nil
	}

	debug.Log("config saved")
	resp.Msg.Success = true
	return resp, nil
}

// SubscribeAuthEvents は認証イベントをストリーミングで送信する
func (cs *CallbackServer) SubscribeAuthEvents(
	ctx context.Context,
	req *connect.Request[authv1.SubscribeAuthEventsRequest],
	stream *connect.ServerStream[authv1.AuthEvent],
) error {
	debug.Log("SubscribeAuthEvents called")

	// セッションを確保（Cookieベース）
	session := cs.ensureSessionFromHeader(req.Header(), stream.ResponseHeader())
	if session == nil {
		return connect.NewError(connect.CodeInternal, fmt.Errorf("failed to create session"))
	}

	// ストリーム接続を記録
	cs.handleStreamConnect()
	defer cs.handleStreamDisconnect()

	// 現在の状態を即座に送信
	status, errorMsg := cs.sessionStatus()
	if err := stream.Send(buildAuthEvent(status, errorMsg)); err != nil {
		debug.Log("failed to send initial status", "error", err)
		return err
	}

	// 認証が既に完了している場合は終了
	if status != "pending" {
		debug.Log("auth already completed, closing stream", "status", status)
		return nil
	}

	// keepalive設定を取得
	keepalive := cs.keepaliveConfig()
	interval := keepalive.IntervalDuration()
	if interval <= 0 {
		interval = time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	// 状態変更を監視するループ
	// statusNotifyチャネルで即時通知、tickerで定期keepalive送信
	for {
		select {
		case <-ctx.Done():
			// クライアントが切断した
			debug.Log("stream client disconnected")
			return ctx.Err()
		case <-cs.statusNotify:
			// 状態変更が通知された（即時通知）
			currentStatus, currentError := cs.sessionStatus()
			if err := stream.Send(buildAuthEvent(currentStatus, currentError)); err != nil {
				debug.Log("failed to send status update", "error", err)
				return err
			}
			if currentStatus != "pending" {
				debug.Log("auth status changed (notified), closing stream", "status", currentStatus)
				return nil
			}
		case <-ticker.C:
			// 定期的にイベント送信（keepalive兼フォールバック）
			// PENDING中も送信することで、中継・プロキシによる切断を検知可能にする
			currentStatus, currentError := cs.sessionStatus()
			if err := stream.Send(buildAuthEvent(currentStatus, currentError)); err != nil {
				debug.Log("failed to send keepalive", "error", err)
				return err
			}
			if currentStatus != "pending" {
				debug.Log("auth status changed (ticker), closing stream", "status", currentStatus)
				return nil
			}
		}
	}
}

// buildAuthEvent はAuthEventを構築する
func buildAuthEvent(status, errorMsg string) *authv1.AuthEvent {
	event := &authv1.AuthEvent{Status: statusToProto(status)}
	if errorMsg != "" {
		event.Error = stringPtr(errorMsg)
	}
	return event
}

// statusToProto は文字列のステータスをprotoのenumに変換する
func statusToProto(status string) authv1.AuthStatus {
	switch status {
	case "pending":
		return authv1.AuthStatus_AUTH_STATUS_PENDING
	case "success":
		return authv1.AuthStatus_AUTH_STATUS_SUCCESS
	case "error":
		return authv1.AuthStatus_AUTH_STATUS_ERROR
	default:
		return authv1.AuthStatus_AUTH_STATUS_UNSPECIFIED
	}
}

// stringPtr は文字列のポインタを返す
func stringPtr(s string) *string {
	return &s
}

// AuthenticateWithApiKey はAPI Keyによる認証を実行する
func (cs *CallbackServer) AuthenticateWithApiKey(
	ctx context.Context,
	req *connect.Request[authv1.AuthenticateWithApiKeyRequest],
) (*connect.Response[authv1.AuthenticateWithApiKeyResponse], error) {
	resp := connect.NewResponse(&authv1.AuthenticateWithApiKeyResponse{})

	// セッションを確保（Cookieベース）
	session := cs.ensureSessionFromHeader(req.Header(), resp.Header())
	if session == nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to create session"))
	}

	spaceHost := req.Msg.SpaceHost
	apiKey := strings.TrimSpace(req.Msg.ApiKey)

	debug.Log("AuthenticateWithApiKey called", "space_host", spaceHost)

	// バリデーション
	if apiKey == "" {
		resp.Msg.Success = false
		resp.Msg.Error = stringPtr("API Key を入力してください")
		return resp, nil
	}

	// 既存の設定を取得
	profile := cs.configStore.CurrentProfile()
	existingSpace := profile.Space
	existingDomain := profile.Domain

	var space, domain string
	var spaceChanged bool

	// spaceHostが指定されている場合はパースして使用
	if spaceHost != "" {
		var err error
		space, domain, err = parseSpaceHost(spaceHost)
		if err != nil {
			debug.Log("invalid space host", "error", err)
			resp.Msg.Success = false
			resp.Msg.Error = stringPtr("無効なスペース形式です: " + err.Error())
			return resp, nil
		}
		// 既存の値と異なる場合のみ変更フラグを立てる
		spaceChanged = space != existingSpace || domain != existingDomain
	} else {
		// spaceHostが空の場合は既存の設定を使用
		if existingSpace == "" || existingDomain == "" {
			resp.Msg.Success = false
			resp.Msg.Error = stringPtr("スペースを入力してください")
			return resp, nil
		}
		space = existingSpace
		domain = existingDomain
	}

	// API Keyの検証（ユーザー情報取得）
	debug.Log("verifying API Key", "space", space, "domain", domain)
	apiClient := api.NewClient(space, domain, "", api.WithAPIKey(apiKey))
	user, err := apiClient.GetCurrentUser(ctx)
	if err != nil {
		debug.Log("API Key verification failed", "error", err)
		resp.Msg.Success = false
		resp.Msg.Error = stringPtr("API Key の検証に失敗しました: " + err.Error())
		return resp, nil
	}

	debug.Log("API Key verified", "user", user.Name.Value)

	// 設定を保存（変更があった場合のみ）
	profileName := cs.configStore.GetActiveProfile()
	if spaceChanged {
		if err := cs.configStore.SetProfileValue(config.LayerUser, profileName, "space", space); err != nil {
			debug.Log("failed to save space", "error", err)
		}
		if err := cs.configStore.SetProfileValue(config.LayerUser, profileName, "domain", domain); err != nil {
			debug.Log("failed to save domain", "error", err)
		}
	}

	// 認証情報を保存（auth_typeと必要な情報のみ更新）
	cred := &config.Credential{
		AuthType: config.AuthTypeAPIKey,
		APIKey:   apiKey,
		UserID:   user.UserId.Value,
		UserName: user.Name.Value,
	}
	_ = cs.configStore.SetCredential(profileName, cred)

	if err := cs.configStore.Save(ctx); err != nil {
		debug.Log("failed to save config", "error", err)
		resp.Msg.Success = false
		resp.Msg.Error = stringPtr("設定の保存に失敗しました: " + err.Error())
		return resp, nil
	}

	// セッションステータスを更新
	cs.sessionMu.Lock()
	if cs.session != nil {
		cs.session.Status = "success"
	}
	cs.sessionMu.Unlock()
	cs.notifyStatus("success")

	// 認証完了を通知
	cs.once.Do(func() {
		cs.result <- CallbackResult{Code: "api_key_auth"}
	})

	resp.Msg.Success = true
	resp.Msg.UserName = stringPtr(user.Name.Value)
	debug.Log("API Key authentication successful", "user", user.Name.Value)
	return resp, nil
}
