# OAuth 2.0 中継サーバー設計書

## 1. 概要

### 1.1 目的

CLIツールからBacklog APIにOAuth 2.0 Authorization Code Grantでアクセスする際、CLIツール自体にClient IDやClient Secretを持たせずに認証を実現する。

### 1.2 背景

- CLIツールにシークレットを埋め込むとリバースエンジニアリングで漏洩するリスクがある
- Authorization Code Grantでは認可コード→トークン交換時にClient Secretが必要
- リフレッシュトークンによるトークン更新時にもClient Secretが必要

### 1.3 解決策

中継サーバーを設置し、Client ID/Secretを中継サーバー側で管理する。CLIは中継サーバー経由でトークンの取得・更新を行う。

---

## 2. 設計方針

### 2.1 採用した方式

| 要素 | 採用方式 | 理由 |
|------|----------|------|
| ポート情報の保持 | Cookie | stateの役割を本来のCSRF保護に限定できる |
| 認可コードの受け渡し | HTTPリダイレクト（302） | JavaScript不要で信頼性が高い |
| トークン取得・更新 | 統一エンドポイント | OAuth標準に沿った設計、実装がシンプル |
| state検証 | 中継サーバー側 | セキュリティロジックの集約 |
| 中継サーバーの状態 | ステートレス | ストレージ・DB不要 |

### 2.2 不採用とした方式

| 方式 | 不採用理由 |
|------|-----------|
| stateにポート情報埋め込み | stateの役割が混在、署名ロジックが必要 |
| トークンをURLでリダイレクト | ブラウザ履歴・アドレスバーへの露出リスク |
| 自動POSTでトークン送信 | JavaScript依存、ブロックされる可能性 |
| 固定ポート | ポート競合の問題 |

---

## 3. アーキテクチャ

### 3.1 コンポーネント構成

```
┌─────────────────┐
│   CLIツール      │  - トークンの保存・管理
│   (ユーザー環境)  │  - ローカルHTTPサーバー起動
└────────┬────────┘
         │
         │ HTTPS
         ↓
┌─────────────────┐
│   中継サーバー    │  - Client ID/Secret保持
│   (クラウド)     │  - トークン交換・更新の代行
└────────┬────────┘
         │
         │ HTTPS
         ↓
┌─────────────────┐
│ Backlog認可サーバー│
│   (外部)        │
└─────────────────┘
```

### 3.2 登録情報

| 項目 | 値 |
|------|-----|
| リダイレクトURI（Backlogに登録） | `https://relay.example.com/auth/callback` |
| Client ID | 中継サーバーの環境変数で管理 |
| Client Secret | 中継サーバーの環境変数で管理 |

---

## 4. 認証フロー

### 4.1 初回認証フロー

```
┌─────────┐      ┌──────────┐      ┌─────────────┐      ┌─────────────────┐
│   CLI   │      │ ブラウザ   │      │  中継サーバー  │      │ Backlog認可サーバー │
└────┬────┘      └────┬─────┘      └──────┬──────┘      └────────┬────────┘
     │                │                   │                     │
     │ 1. 空きポート確保                    │                     │
     │    (例: 52847)  │                   │                     │
     │                │                   │                     │
     │ 2. localhostで │                   │                     │
     │    HTTPサーバー起動                  │                     │
     │                │                   │                     │
     │ 3. ブラウザで認可開始URLを開く        │                     │
     │───────────────>│                   │                     │
     │                │                   │                     │
     │                │ 4. GET /auth/start?port=52847           │
     │                │──────────────────>│                     │
     │                │                   │                     │
     │                │                   │ 5. state生成        │
     │                │                   │                     │
     │                │ 6. Set-Cookie:    │                     │
     │                │    oauth_port=52847                     │
     │                │    oauth_state=xxx │                     │
     │                │    + 302 Redirect to Backlog            │
     │                │<──────────────────│                     │
     │                │                   │                     │
     │                │ 7. Backlog認可画面にアクセス              │
     │                │─────────────────────────────────────────>│
     │                │                   │                     │
     │                │                   │     8. ユーザーが認可 │
     │                │                   │                     │
     │                │ 9. 302 Redirect   │                     │
     │                │    /auth/callback?code=xxx&state=yyy    │
     │                │<─────────────────────────────────────────│
     │                │                   │                     │
     │                │ 10. GET /auth/callback?code=xxx&state=yyy
     │                │     + Cookie送信   │                     │
     │                │──────────────────>│                     │
     │                │                   │                     │
     │                │                   │ 11. state検証       │
     │                │                   │     (Cookie vs URL) │
     │                │                   │                     │
     │                │                   │ 12. Cookieからport取得
     │                │                   │                     │
     │                │ 13. 302 Redirect  │                     │
     │                │     http://localhost:52847/callback?code=xxx
     │                │     + Cookie削除   │                     │
     │                │<──────────────────│                     │
     │                │                   │                     │
     │ 14. GET /callback?code=xxx         │                     │
     │<───────────────│                   │                     │
     │                │                   │                     │
     │ 15. POST /auth/token               │                     │
     │     { grant_type: "authorization_code",                  │
     │       code: "xxx" }                │                     │
     │───────────────────────────────────>│                     │
     │                │                   │                     │
     │                │                   │ 16. POST /oauth2/token
     │                │                   │     + Client Secret │
     │                │                   │────────────────────>│
     │                │                   │                     │
     │                │                   │ 17. トークン応答     │
     │                │                   │<────────────────────│
     │                │                   │                     │
     │ 18. トークン応答 │                   │                     │
     │     { access_token, refresh_token, expires_in }          │
     │<───────────────────────────────────│                     │
     │                │                   │                     │
     │ 19. トークンをローカルに保存         │                     │
```

### 4.2 トークン更新フロー

```
┌─────────┐                          ┌─────────────┐      ┌─────────────────┐
│   CLI   │                          │  中継サーバー  │      │ Backlog認可サーバー │
└────┬────┘                          └──────┬──────┘      └────────┬────────┘
     │                                      │                     │
     │ 1. POST /auth/token                  │                     │
     │    { grant_type: "refresh_token",    │                     │
     │      refresh_token: "xxx" }          │                     │
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

| メソッド | パス | 説明 |
|---------|------|------|
| GET | `/auth/start` | 認可開始（Backlogへリダイレクト） |
| GET | `/auth/callback` | Backlogからのコールバック受信 |
| POST | `/auth/token` | トークン取得・更新 |

### 5.2 GET /auth/start

認可フローを開始する。Cookieにポート情報とstateを保存し、Backlog認可画面へリダイレクトする。

#### リクエスト

```
GET /auth/start?port=52847
```

| パラメータ | 型 | 必須 | 説明 |
|-----------|-----|------|------|
| port | integer | Yes | CLIのローカルサーバーのポート番号（1024-65535） |

#### レスポンス

```
HTTP/1.1 302 Found
Location: https://{space}.backlog.com/OAuth2AccessRequest.action?response_type=code&client_id=xxx&redirect_uri=xxx&state=xxx
Set-Cookie: oauth_port=52847; Max-Age=300; HttpOnly; Secure; SameSite=Lax
Set-Cookie: oauth_state=xxx; Max-Age=300; HttpOnly; Secure; SameSite=Lax
```

### 5.3 GET /auth/callback

Backlogからの認可コールバックを受信し、CLIのローカルサーバーへリダイレクトする。

#### リクエスト

```
GET /auth/callback?code=xxx&state=xxx
Cookie: oauth_port=52847; oauth_state=xxx
```

| パラメータ | 型 | 必須 | 説明 |
|-----------|-----|------|------|
| code | string | Yes | 認可コード |
| state | string | Yes | CSRF保護用トークン |

#### レスポンス（成功時）

```
HTTP/1.1 302 Found
Location: http://localhost:52847/callback?code=xxx
Set-Cookie: oauth_port=; Max-Age=0
Set-Cookie: oauth_state=; Max-Age=0
```

#### レスポンス（エラー時）

```
HTTP/1.1 400 Bad Request
Content-Type: text/html

<html><body><h1>エラー</h1><p>不正なリクエストです。</p></body></html>
```

### 5.4 POST /auth/token

認可コードまたはリフレッシュトークンをアクセストークンに交換する。

#### リクエスト（認可コード交換）

```
POST /auth/token
Content-Type: application/json

{
  "grant_type": "authorization_code",
  "code": "認可コード"
}
```

#### リクエスト（トークン更新）

```
POST /auth/token
Content-Type: application/json

{
  "grant_type": "refresh_token",
  "refresh_token": "リフレッシュトークン"
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
2. ローカルでHTTPサーバーを起動（1リクエストのみ受け付け）
3. ブラウザで `https://relay.example.com/auth/start?port={port}` を開く
4. コールバック待機（タイムアウト設定推奨：60-120秒）
5. 認可コードを受信したら `/auth/token` にPOSTしてトークン取得
6. トークンをローカルに保存

### 6.2 トークン更新フロー

1. 保存済みトークンを読み込み
2. アクセストークンの有効期限を確認
3. 期限切れまたは期限間近の場合、`/auth/token` にリフレッシュトークンをPOST
4. 新しいトークンで上書き保存

### 6.3 トークン保存

| 項目 | 推奨 |
|------|------|
| 保存場所 | `~/.config/{app_name}/tokens.json` または OS標準の資格情報ストア |
| 暗号化 | 可能であればOS提供のキーチェーン/資格情報マネージャーを使用 |
| パーミッション | 600（所有者のみ読み書き可） |

---

## 7. セキュリティ考慮事項

### 7.1 中継サーバー

| 項目 | 対策 |
|------|------|
| 通信の暗号化 | HTTPS必須 |
| state検証 | Cookieに保存したstateとURL上のstateを比較 |
| Cookieの有効期限 | 短め（5分程度）に設定 |
| Cookieの属性 | `HttpOnly`, `Secure`, `SameSite=Lax` |
| CORS | 必要に応じて設定（基本的に不要） |
| Rate Limiting | 認可開始エンドポイントに適用推奨 |

### 7.2 CLI

| 項目 | 対策 |
|------|------|
| ローカルサーバー | localhostのみでリッスン |
| タイムアウト | コールバック待機に適切なタイムアウトを設定 |
| トークン保存 | 適切なパーミッション設定、可能なら暗号化 |
| リフレッシュトークン | 漏洩時の影響が大きいため厳重に管理 |

### 7.3 認可コードの特性（URLに露出しても安全な理由）

| 特性 | 説明 |
|------|------|
| 有効期限 | 数分（通常1-10分） |
| 使用回数 | 1回限り（使用後は無効化） |
| 単体での価値 | なし（Client Secretがないとトークンに交換不可） |

---

## 8. エラーハンドリング

### 8.1 中継サーバーが返すエラー

| ケース | HTTPステータス | エラーコード |
|--------|---------------|-------------|
| 不正なport | 400 | invalid_request |
| state不一致 | 400 | invalid_state |
| Cookie不在 | 400 | missing_cookie |
| 認可コード無効 | 400 | invalid_grant |
| リフレッシュトークン無効 | 400 | invalid_grant |
| Backlog API障害 | 502 | upstream_error |

### 8.2 CLI側のエラーハンドリング

| ケース | 対応 |
|--------|------|
| タイムアウト | 再認証を促す |
| トークン交換失敗 | エラーメッセージ表示、再認証を促す |
| リフレッシュ失敗 | 再認証フローを開始 |
| ネットワークエラー | リトライ（exponential backoff） |

---

## 9. 設計の決定根拠

### 9.1 Cookie vs state埋め込み

| 観点 | Cookie方式 | state埋め込み方式 |
|------|-----------|-----------------|
| stateの役割 | CSRF保護のみ（本来の用途） | 情報伝達 + CSRF保護（兼用） |
| 署名ロジック | 不要 | 必要（改ざん防止） |
| 実装複雑度 | 低 | 中 |
| 責務分離 | 明確 | 混在 |

**採用: Cookie方式**

### 9.2 HTTPリダイレクト vs 自動POST

| 観点 | HTTPリダイレクト | 自動POST |
|------|----------------|----------|
| JavaScript依存 | なし | あり |
| 信頼性 | 高（ブラウザネイティブ） | 中（ブロックされる可能性） |
| 処理速度 | 高速 | HTML解析+JS実行のオーバーヘッド |
| 実装複雑度 | 低 | 中 |

**採用: HTTPリダイレクト**

### 9.3 エンドポイント統一 vs 分離

| 観点 | 統一 | 分離 |
|------|------|------|
| OAuth標準との整合性 | 高 | 低 |
| CLI実装の複雑度 | 低 | やや高 |
| 保守性 | 高（ロジック集約） | やや低 |

**採用: 統一エンドポイント（`/auth/token`）**

---

## 10. 付録

### 10.1 環境変数（中継サーバー）

| 変数名 | 説明 | 例 |
|--------|------|-----|
| BACKLOG_CLIENT_ID | BacklogアプリのClient ID | `abcd1234...` |
| BACKLOG_CLIENT_SECRET | BacklogアプリのClient Secret | `efgh5678...` |
| BACKLOG_SPACE | Backlogスペース名 | `your-space` |
| STATE_SECRET | state署名用シークレット（Cookie方式では不要） | - |

### 10.2 Backlog OAuth設定

| 項目 | 値 |
|------|-----|
| リダイレクトURI | `https://relay.example.com/auth/callback` |
| スコープ | 必要に応じて設定 |

### 10.3 参考資料

- [RFC 6749 - The OAuth 2.0 Authorization Framework](https://tools.ietf.org/html/rfc6749)
- [RFC 7636 - Proof Key for Code Exchange (PKCE)](https://tools.ietf.org/html/rfc7636)
- [Backlog API ドキュメント](https://developer.nulab.com/ja/docs/backlog/)
