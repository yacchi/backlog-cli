# OAuth 2.0 中継サーバー設計書（Backlog OAuth Relay）

## 1. 概要

### 1.1 目的

CLIツールからBacklog APIにOAuth 2.0 Authorization Code Grantでアクセスする際、CLIツール自体にClient IDやClient
Secretを持たせずに認証を実現する。

### 1.2 背景

- CLIツールにシークレットを埋め込むとリバースエンジニアリングで漏洩するリスクがある
- Authorization Code Grantでは認可コード→トークン交換時にClient Secretが必要
- リフレッシュトークンによるトークン更新時にもClient Secretが必要

### 1.3 解決策

中継サーバーを設置し、Client ID/Secretを中継サーバー側で管理する。CLIは中継サーバー経由でトークンの取得・更新を行う。

### 1.4 実装の所在（このリポジトリ）

- Relay の中核ロジック: `packages/relay-core/`
- AWS デプロイ（CDK 等）: `packages/relay-aws/`
- CLI 側の利用（well-known / token / start）: `packages/backlog/internal/auth/*`

---

## 2. 設計方針

### 2.1 採用した方式

| 要素         | 採用方式            | 理由                       |
|------------|-----------------|--------------------------|
| 認証フロー開始    | CLI ローカルサーバー起点  | state管理がCLI側で完結、Cookie不要 |
| ポート情報の保持   | stateに情報をエンコード（非署名） | 中継サーバーがステートレスになる（Cookie不要） |
| 認可コードの受け渡し | HTTPリダイレクト（302） | JavaScript不要で信頼性が高い      |
| トークン取得・更新  | 統一エンドポイント       | OAuth標準に沿った設計、実装がシンプル    |
| state検証    | CLI側（ローカルサーバー）  | セキュリティ責務の明確化             |
| 中継サーバーの状態  | 完全ステートレス        | Cookie不要、スケーラビリティ向上      |

### 2.2 不採用とした方式

| 方式                | 不採用理由                       |
|-------------------|-----------------------------|
| 中継サーバーでCookie使用   | ブラウザのサードパーティCookie制限の影響を受ける |
| 中継サーバーでstate生成・検証 | 状態管理が必要になりスケーラビリティが低下       |
| トークンをURLでリダイレクト   | ブラウザ履歴・アドレスバーへの露出リスク        |
| 自動POSTでトークン送信     | JavaScript依存、ブロックされる可能性     |
| 固定ポート             | ポート競合の問題                    |

### 2.3 Cookie不要方式のメリット

| メリット               | 説明                        |
|--------------------|---------------------------|
| サードパーティCookie制限の回避 | Safari/Firefox等の制限に影響されない |
| SameSite属性の考慮不要    | Cookie属性の複雑な設定が不要         |
| 中継サーバーの完全ステートレス化   | 水平スケーリングが容易               |
| PKCE実装の容易さ         | CLI側でcode_verifierを管理しやすい |

---

## 3. アーキテクチャ

### 3.1 コンポーネント構成

```
┌─────────────────────┐
│   CLIツール          │  - トークンの保存・管理
│   (ユーザー環境)      │  - ローカルHTTPサーバー起動
│                     │  - state生成・検証
└──────────┬──────────┘
           │
           │ HTTP (localhost)
           ↓
┌─────────────────────┐
│ ローカルHTTPサーバー   │  - 認証フロー開始点
│ (localhost:動的ポート) │  - コールバック受信
└──────────┬──────────┘
           │
           │ HTTPS
           ↓
┌─────────────────────┐
│   中継サーバー        │  - Client ID/Secret保持
│   (クラウド)         │  - トークン交換・更新の代行
│                     │  - ステートレス（Cookie不使用）
└──────────┬──────────┘
           │
           │ HTTPS
           ↓
┌─────────────────────┐
│ Backlog認可サーバー   │
│   (外部)            │
└─────────────────────┘
```

### 3.2 登録情報

| 項目                    | 値                                         |
|-----------------------|-------------------------------------------|
| リダイレクトURI（Backlogに登録） | `https://relay.example.com/auth/callback` |
| Client ID             | 中継サーバーの環境変数で管理                            |
| Client Secret         | 中継サーバーの環境変数で管理                            |

---

## 4. 認証フロー

### 4.1 初回認証フロー

```
┌─────────┐      ┌────────────────┐      ┌─────────────┐      ┌─────────────────┐
│   CLI   │      │ ローカルサーバー  │      │  中継サーバー  │      │ Backlog認可サーバー │
│         │      │ (localhost)    │      │  (クラウド)   │      │                 │
└────┬────┘      └───────┬────────┘      └──────┬───────┘      └────────┬────────┘
     │                   │                      │                      │
     │ 1. 空きポート確保    │                      │                      │
     │    (例: 52847)     │                      │                      │
     │                   │                      │                      │
     │ 2. state生成       │                      │                      │
     │    セッション保存    │                      │                      │
     │                   │                      │                      │
     │ 3. ローカルサーバー起動                      │                      │
     │    (/auth/start, /callback を待ち受け)     │                      │
     │                   │                      │                      │
     │ 4. ブラウザで http://localhost:52847/auth/start を開く            │
     │──────────────────>│                      │                      │
     │                   │                      │                      │
     │                   │ 5. 302 Redirect      │                      │
     │                   │    → 中継サーバー      │                      │
     │                   │    /auth/start?port=52847&state=xxx&space=yyy&domain=zzz
     │                   │─────────────────────>│                      │
	     │                   │                      │                      │
	     │                   │                      │ 6. パラメータ検証     │
	     │                   │                      │    stateをエンコード  │
	     │                   │                      │                      │
	     │                   │                      │ 7. 302 Redirect      │
	     │                   │                      │    → Backlog認可URL   │
	     │                   │                      │    state=encoded_state
	     │                   │                      │─────────────────────>│
     │                   │                      │                      │
     │                   │                      │      8. ユーザー認可   │
     │                   │                      │                      │
	     │                   │                      │ 9. 302 Redirect      │
	     │                   │                      │    → /auth/callback  │
	     │                   │                      │    ?code=xxx&state=encoded_state
	     │                   │                      │<─────────────────────│
	     │                   │                      │                      │
	     │                   │                      │ 10. stateをデコード   │
	     │                   │                      │     port, cli_state抽出
     │                   │                      │                      │
     │                   │ 11. 302 Redirect     │                      │
     │                   │     → localhost:52847/callback?code=xxx&state=cli_state
     │                   │<─────────────────────│                      │
     │                   │                      │                      │
     │ 12. state検証      │                      │                      │
     │     (保存したstateと比較)                  │                      │
     │<──────────────────│                      │                      │
     │                   │                      │                      │
     │ 13. POST /auth/token                     │                      │
     │     { grant_type: "authorization_code",  │                      │
     │       code: "xxx", space: "yyy", domain: "zzz" }                │
     │─────────────────────────────────────────>│                      │
     │                   │                      │                      │
     │                   │                      │ 14. POST /oauth2/token
     │                   │                      │     + Client Secret  │
     │                   │                      │─────────────────────>│
     │                   │                      │                      │
     │                   │                      │ 15. トークン応答      │
     │                   │                      │<─────────────────────│
     │                   │                      │                      │
     │ 16. トークン応答    │                      │                      │
     │     { access_token, refresh_token, expires_in }                 │
     │<─────────────────────────────────────────│                      │
     │                   │                      │                      │
     │ 17. トークンをローカルに保存               │                      │
     │                   │                      │                      │
     │ 18. ローカルサーバー停止                   │                      │
```

### 4.2 トークン更新フロー

```
┌─────────┐                          ┌─────────────┐      ┌─────────────────┐
│   CLI   │                          │  中継サーバー  │      │ Backlog認可サーバー │
└────┬────┘                          └──────┬──────┘      └────────┬────────┘
     │                                      │                     │
     │ 1. POST /auth/token                  │                     │
     │    { grant_type: "refresh_token",    │                     │
     │      refresh_token: "xxx",           │                     │
     │      space: "yyy", domain: "zzz" }   │                     │
     │─────────────────────────────────────>│                     │
     │                                      │                     │
     │                                      │ 2. POST /oauth2/token
     │                                      │    + Client Secret  │
     │                                      │────────────────────>│
     │                                      │                     │
     │                                      │ 3. 新トークン応答    │
     │                                      │<────────────────────│
     │                                      │                     │
     │ 4. 新トークン応答                     │                     │
     │    { access_token, refresh_token, expires_in }             │
     │<─────────────────────────────────────│                     │
     │                                      │                     │
     │ 5. トークンを上書き保存               │                     │
```

---

## 5. 中継サーバー API仕様

### 5.1 エンドポイント一覧

| メソッド | パス                      | 説明                   |
|------|-------------------------|----------------------|
| GET  | `/.well-known/backlog-oauth-relay` | サーバー情報・対応ドメイン |
| GET  | `/auth/start`           | 認可開始（Backlogへリダイレクト） |
| GET  | `/auth/callback`        | Backlogからのコールバック受信   |
| POST | `/auth/token`           | トークン取得・更新            |
| GET  | `/health`               | ヘルスチェック              |

### 5.2 GET /.well-known/backlog-oauth-relay

中継サーバーの情報を返す。CLIが中継サーバーの対応ドメインを確認するために使用。

#### レスポンス

```json
{
  "version": "1.0",
  "capabilities": ["oauth2", "token-exchange", "token-refresh"],
  "supported_domains": ["backlog.jp", "backlog.com"]
}
```

### 5.3 GET /auth/start

認可フローを開始する。CLIから受け取ったパラメータを検証し、stateをエンコードしてBacklog認可画面へリダイレクトする。

#### リクエスト

```
GET /auth/start?port=52847&state=xxx&space=myspace&domain=backlog.jp
```

| パラメータ  | 型       | 必須  | 説明                                      |
|--------|---------|-----|-----------------------------------------|
| port   | integer | Yes | CLIのローカルサーバーのポート番号（1024-65535）          |
| state  | string  | Yes | CLIが生成したCSRF保護用トークン                     |
| space  | string  | Yes | Backlogスペース名                            |
| domain | string  | Yes | Backlogドメイン（backlog.jp または backlog.com） |

#### レスポンス

```
HTTP/1.1 302 Found
Location: https://{space}.{domain}/OAuth2AccessRequest.action?response_type=code&client_id=xxx&redirect_uri=xxx&state=encoded_state

※ 現行実装では state は署名しません（`packages/relay-core/src/utils/state.ts`）。
```

#### state（encoded_state）の構造

```
encoded_state = base64url(json({
  "port": 52847,
  "cli_state": "original_state_from_cli",
  "space": "myspace",
  "domain": "backlog.jp",
  "project": "PROJ" // optional
}))
```

注: CSRF保護は CLI が生成した `cli_state` を、ローカルサーバーの `/callback` で **完全一致** で検証することで担保します
（`packages/backlog/internal/auth/callback.go`）。

### 5.4 GET /auth/callback

Backlogからの認可コールバックを受信し、state をデコード後、CLIのローカルサーバーへリダイレクトする。

#### リクエスト

```
GET /auth/callback?code=xxx&state=encoded_state
```

| パラメータ | 型      | 必須  | 説明        |
|-------|--------|-----|-----------|
| code  | string | Yes | 認可コード     |
| state | string | Yes | encoded_state |

#### レスポンス（成功時）

```
HTTP/1.1 302 Found
Location: http://localhost:{port}/callback?code=xxx&state={cli_state}
```

#### レスポンス（エラー時）

```
HTTP/1.1 400 Bad Request
Content-Type: text/html

<html><body><h1>エラー</h1><p>不正なリクエストです。</p></body></html>
```

### 5.5 POST /auth/token

認可コードまたはリフレッシュトークンをアクセストークンに交換する。

#### リクエスト（認可コード交換）

```
POST /auth/token
Content-Type: application/json

{
  "grant_type": "authorization_code",
  "code": "認可コード",
  "space": "myspace",
  "domain": "backlog.jp"
}
```

#### リクエスト（トークン更新）

```
POST /auth/token
Content-Type: application/json

{
  "grant_type": "refresh_token",
  "refresh_token": "リフレッシュトークン",
  "space": "myspace",
  "domain": "backlog.jp"
}
```

#### レスポンス（成功時）

```json
{
  "access_token": "アクセストークン",
  "token_type": "Bearer",
  "expires_in": 3600,
  "refresh_token": "リフレッシュトークン"
}
```

#### レスポンス（エラー時）

```json
{
  "error": "invalid_grant",
  "error_description": "エラーの詳細"
}
```

---

## 6. CLI実装要件

### 6.1 認証フロー

1. `find_free_port()` で空きポートを動的に取得
2. `state` を生成してメモリに保持
3. ローカルでHTTPサーバーを起動（`/auth/start` と `/callback` を待ち受け）
4. ブラウザで `http://localhost:{port}/auth/start` を開く
5. `/auth/start` へのリクエストで中継サーバーへリダイレクト
6. `/callback` でコールバックを受信、stateを検証
7. 認可コードを受信したら `/auth/token` にPOSTしてトークン取得
8. トークンをローカルに保存
9. ローカルサーバーを停止

### 6.2 ローカルサーバーのエンドポイント

| パス            | 処理                            |
|---------------|-------------------------------|
| `/auth/start` | 中継サーバーの `/auth/start` へリダイレクト |
| `/callback`   | state検証、認可コード受信               |

### 6.3 トークン更新フロー

1. 保存済みトークンを読み込み
2. アクセストークンの有効期限を確認
3. 期限切れまたは期限間近の場合、`/auth/token` にリフレッシュトークンをPOST
4. 新しいトークンで上書き保存

### 6.4 トークン保存

| 項目      | 推奨                                                      |
|---------|---------------------------------------------------------|
| 保存場所    | `~/.config/{app_name}/config.yaml` 内の credentials セクション |
| 暗号化     | 可能であればOS提供のキーチェーン/資格情報マネージャーを使用                         |
| パーミッション | 600（所有者のみ読み書き可）                                         |

---

## 7. セキュリティ考慮事項

### 7.1 中継サーバー

| 項目            | 対策                      |
|---------------|-------------------------|
| 通信の暗号化        | HTTPS必須                 |
| state（relay）  | base64url(JSON) でエンコード（非署名） |
| CORS          | 不要（ブラウザからの直接アクセスはない）    |
| Rate Limiting | 認可開始エンドポイントに適用推奨        |

### 7.2 CLI（ローカルサーバー）

| 項目       | 対策                             |
|----------|--------------------------------|
| リッスンアドレス | `127.0.0.1` のみ（外部からのアクセスを防止）   |
| state検証  | 自身が生成したstateと厳密に比較             |
| タイムアウト   | コールバック待機に適切なタイムアウトを設定（60-120秒） |
| サーバー停止   | 認証完了後は速やかにサーバーを停止              |

### 7.3 トークン管理

| 項目          | 対策                |
|-------------|-------------------|
| ファイルパーミッション | 600（所有者のみ読み書き可）   |
| リフレッシュトークン  | 漏洩時の影響が大きいため厳重に管理 |
| トークン更新      | 有効期限の5分前を目安に自動更新  |

### 7.4 認可コードの特性（URLに露出しても安全な理由）

| 特性     | 説明                             |
|--------|--------------------------------|
| 有効期限   | 数分（通常1-10分）                    |
| 使用回数   | 1回限り（使用後は無効化）                  |
| 単体での価値 | なし（Client Secretがないとトークンに交換不可） |

---

## 8. エラーハンドリング

### 8.1 中継サーバーが返すエラー

| ケース           | HTTPステータス | エラーコード          |
|---------------|-----------|-----------------|
| 不正なport       | 400       | invalid_request |
| 不正なdomain     | 400       | invalid_request |
| state不正（デコード失敗） | 400       | invalid_request |
| 認可コード無効       | 400       | invalid_grant   |
| リフレッシュトークン無効  | 400       | invalid_grant   |
| Backlog API障害 | 502       | upstream_error  |

### 8.2 CLI側のエラーハンドリング

| ケース       | 対応                        |
|-----------|---------------------------|
| タイムアウト    | 再認証を促す                    |
| state不一致  | エラーメッセージ表示、再認証を促す         |
| トークン交換失敗  | エラーメッセージ表示、再認証を促す         |
| リフレッシュ失敗  | 再認証フローを開始                 |
| ネットワークエラー | リトライ（exponential backoff） |

---

## 9. 設計の決定根拠

### 9.1 CLI起点 vs 中継サーバー起点

| 観点        | CLI起点（採用）    | 中継サーバー起点            |
|-----------|--------------|---------------------|
| Cookie依存  | なし           | あり（サードパーティCookie問題） |
| state管理   | CLI側で完結      | 中継サーバーで管理が必要        |
| 中継サーバーの状態 | 完全ステートレス     | Cookieによる状態保持       |
| PKCE実装    | CLI側で自然に実装可能 | 中継サーバーでの管理が必要       |
| スケーラビリティ  | 高            | 中                   |

**採用: CLI起点方式**

### 9.2 stateエンコード（非署名） vs Cookie

| 観点     | stateエンコード（採用） | Cookie             |
|--------|---------------|--------------------|
| ブラウザ制限 | 影響なし          | サードパーティCookie制限の影響 |
| 実装複雑度  | 中（エンコード/デコード） | 低                  |
| ステートレス | Yes           | No                 |
| 将来性    | 高             | Cookie制限強化の傾向      |

**採用: stateエンコード方式（非署名）**

### 9.3 HTTPリダイレクト vs 自動POST

| 観点           | HTTPリダイレクト   | 自動POST              |
|--------------|--------------|---------------------|
| JavaScript依存 | なし           | あり                  |
| 信頼性          | 高（ブラウザネイティブ） | 中（ブロックされる可能性）       |
| 処理速度         | 高速           | HTML解析+JS実行のオーバーヘッド |
| 実装複雑度        | 低            | 中                   |

**採用: HTTPリダイレクト**

---

## 10. 付録

### 10.1 環境変数（中継サーバー）

| 変数名                       | 説明                        | 例                 |
|---------------------------|---------------------------|-------------------|
| BACKLOG_JP_CLIENT_ID      | backlog.jp用Client ID      | `abcd1234...`     |
| BACKLOG_JP_CLIENT_SECRET  | backlog.jp用Client Secret  | `efgh5678...`     |
| BACKLOG_COM_CLIENT_ID     | backlog.com用Client ID     | `ijkl9012...`     |
| BACKLOG_COM_CLIENT_SECRET | backlog.com用Client Secret | `mnop3456...`     |

### 10.2 Backlog OAuth設定

| 項目        | 値                                         |
|-----------|-------------------------------------------|
| リダイレクトURI | `https://relay.example.com/auth/callback` |
| スコープ      | 必要に応じて設定                                  |

### 10.3 参考資料

- [RFC 6749 - The OAuth 2.0 Authorization Framework](https://tools.ietf.org/html/rfc6749)
- [RFC 7636 - Proof Key for Code Exchange (PKCE)](https://tools.ietf.org/html/rfc7636)
- [Backlog API ドキュメント](https://developer.nulab.com/ja/docs/backlog/)
