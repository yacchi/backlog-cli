# 認証フロー（CLIローカルサーバー + SPA）

このプロジェクトの認証は、CLIがローカルHTTPサーバーを立ててブラウザUI（SPA）を提供し、SPA↔CLI間を Connect RPC で通信します。
OAuth の Client Secret は CLI に持たせず、外部の Relay（Backlog OAuth Relay）経由でトークン交換を行います。

## コンポーネント

- **CLI**: `packages/backlog/internal/cmd/auth/*`
- **ローカルHTTPサーバー**（CLI内）: `packages/backlog/internal/auth/callback.go`
- **SPA**: `packages/web/`（`go:embed`、開発時は `embed_dev.go`）
- **Relay**: `/.well-known/backlog-oauth-relay`, `/auth/start`, `/auth/token` など（仕様は `docs/design/oauth-relay-server.md`）

## ローカルサーバーの公開エンドポイント

実装: `packages/backlog/internal/auth/callback.go`

- `GET /` : SPA（`packages/web`）を配信
  - SPA起動前に `backlog_auth_session` Cookie を作成して、ストリーミング接続の紐付けに使用
- `GET /auth/popup` : OAuth用ポップアップ（リレーへのリダイレクト起点）
- `GET /callback` : OAuth コールバック受信（リレー→ローカルへのリダイレクト）
- Connect RPC（`AuthService`）:
  - `GetConfig` / `Configure` / `SubscribeAuthEvents` / `AuthenticateWithApiKey`

IDL: `proto/auth/v1/auth.proto`

## セッション（Cookieベース）

SPA の Connect RPC ストリームと、OAuth コールバック処理を同一の「認証セッション」として扱うため、ローカルサーバー側でセッションを管理します。

- Cookie 名: `backlog_auth_session`
- 状態: `pending` / `success` / `error`
- 断線検知: `SubscribeAuthEvents` のストリーム切断で検知し、設定された猶予期間（grace period）内の再接続を待つ

実装: `packages/backlog/internal/auth/callback.go`（`Session`, `SubscribeAuthEvents`）

## OAuth 認証（概要）

1. CLI がローカルサーバーを起動（ポートは `127.0.0.1:0` の空きポートを使用）
2. ブラウザでローカルサーバー（SPA）を開く
3. SPA が `Configure` で `space_host` と `relay_server` を保存（必要に応じて信頼バンドル検証も実施）
4. SPA が `/auth/popup` を開き、ローカルサーバーが Relay の `/auth/start?...` へ 302 リダイレクト
5. Relay が Backlog の認可フローを進め、最終的にローカルの `GET /callback` へリダイレクト
6. CLI が認可コードを受け取り、Relay の `/auth/token` へ交換し、結果を `credentials.yaml` に保存
7. `SubscribeAuthEvents` に `success` が流れ、SPA は完了画面へ遷移

## API Key 認証（概要）

SPA から `AuthenticateWithApiKey` を呼び出し、Backlog API へ疎通して有効な API Key であることを確認します。
成功した API Key は `credentials.yaml` に保存します。

IDL/ハンドラ:

- `proto/auth/v1/auth.proto`
- `packages/backlog/internal/auth/connect_handler.go`（`AuthenticateWithApiKey`）

## Relay の信頼（Relay Config Bundle）

設定保存時に、対象の `allowed_domain`（例: `space.backlog.jp`）に対応する信頼バンドルがインポート済みなら、
Relay 側の `info` / `certs` を使って Relay の正当性を検証し、必要ならバンドル自動更新も行います。

- 仕様: `docs/design/relay-config-bundle.md`
- 実装: `packages/backlog/internal/config/relay_info.go`, `packages/backlog/internal/config/relay_bundle.go`

