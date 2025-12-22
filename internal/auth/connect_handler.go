package auth

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"connectrpc.com/connect"
	authv1 "github.com/yacchi/backlog-cli/gen/go/auth/v1"
	"github.com/yacchi/backlog-cli/gen/go/auth/v1/authv1connect"
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

	debug.Log("GetConfig called", "configured", resp.Msg.Configured)
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
