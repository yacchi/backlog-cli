# Remote MCP Server 設計書

## 概要

Backlog CLI の Remote MCP Server は、Claude Desktop / claude.ai / Claude Code から Backlog API にアクセスするためのリモート MCP サーバーです。

**特徴:**
- **ステートレス認証** — ユーザートークンは JWE 暗号化してクライアント側に保持（サーバーに DB 不要）
- **OAuth ログインのみ** — ユーザーは URL を追加してブラウザでログインするだけ
- **組織のツール制御** — テナント設定で CLI コマンドパターンを allow/deny で制御
- **2 ツール + Skill** — `backlog` (CLI 実行) + `run_script` (Python sandbox) + CLI リファレンス Skill

## アーキテクチャ

```
┌──────────────────┐                       ┌──────────────────────────────────┐
│ Claude Desktop   │                       │  MCP Server (独立サービス)        │
│ claude.ai        │  Streamable HTTP      │                                  │
│ Claude Code      │◄─────────────────────►│  MCP OAuth AS + Transport        │
│                  │  Bearer: JWE token     │  backlog CLI + Deno sandbox     │
└──────┬───────────┘                       └────────┬──────────────┬─────────┘
       │  ブラウザでログイン                          │              │
       └──────────────────────────────────┐         │              │
                                          ▼         ▼              ▼
                                     ┌──────────┐  ┌───────────────┐
                                     │  Relay   │  │ Backlog API   │
                                     │  Server  │  └───────────────┘
                                     └──────────┘
```

## MCP プロトコル対応（dual-era）

サーバーは modern 版（`2026-07-28` 以降。バージョン・クライアント識別・capability を
リクエストごとの `_meta` で運ぶ）と legacy 版（`initialize` ハンドシェイクでセッションを
確立する `2025-11-25` 以前）を同一エンドポイントで両対応する。

| 対応バージョン | 位置づけ |
|---|---|
| `2026-07-28` | modern。`server/discover` / `_meta` / `resultType` を使用 |
| `2025-11-25`, `2025-06-18`, `2025-03-26` | legacy。`initialize` でネゴシエート |

### バージョン決定

1. リクエストボディの `_meta["io.modelcontextprotocol/protocolVersion"]`
2. なければ `MCP-Protocol-Version` ヘッダ
3. どちらも無ければ `2025-03-26` とみなす（ヘッダ導入前のクライアント向け。仕様の MAY）

`_meta` とヘッダの両方があって値が異なる場合は `400` + JSON-RPC `-32020`
(`HeaderMismatch`)。未サポートのバージョンは `400` + `-32022`
(`UnsupportedProtocolVersion`, `data.supported` にサポート一覧)。

`initialize`（legacy）はクライアントが要求したバージョンがサポート範囲なら**そのまま返す**。
範囲外なら最新の legacy 版（`2025-11-25`）を返す — legacy クライアントには fall-forward の
手段がないため、返答が唯一の診断情報になる。

### ミラーヘッダの検証

`Mcp-Method` / `Mcp-Name` はボディの `method` / `params.name`（または `params.uri`）を
ミラーしたもの。ロードバランサ等がヘッダで振り分け、サーバーがボディで実行すると
判断根拠がズレるため、**値の不一致は `-32020` で拒否**する。`Mcp-Name` の
`=?base64?<b64>?=` 形式はデコードして比較する。

ヘッダの**欠落は拒否しない**（仕様上は MUST だが、移行途上のクライアントを壊さないため。
不一致だけを拒否すればセキュリティ上の目的は満たせる）。

### `server/discover`

modern では実装が MUST。`supportedVersions` / `capabilities` / `instructions` を返し、
サーバー識別は `_meta["io.modelcontextprotocol/serverInfo"]` に入れる。
バージョン宣言のないリクエストでも応答する（クライアントが era 判定のプローブに使えるように）。

### 応答エンベロープ

modern と判定したリクエストの結果にのみ以下を付与する（legacy 応答は変更しない）:

| フィールド | 値 |
|---|---|
| `resultType` | 常に `"complete"`（MRTR の `input_required` は未使用） |
| `_meta["io.modelcontextprotocol/serverInfo"]` | サーバー名/バージョン |
| `ttlMs` / `cacheScope` | `tools/list` / `prompts/list` は 5 分 / `private`（呼び出し元のトークンに依存するため）、`server/discover` は 1 時間 / `public` |

未実装メソッドは modern では `404` + `-32601`（stray な HTTP 404 と区別できるよう本文に
JSON-RPC エラーを入れる）、legacy では従来どおり `200` + `-32601`。

### 未対応（意図的）

MRTR / tasks 拡張 / `subscriptions/listen` / Roots / Sampling / MCP Logging は未実装。
いずれも 2026-07-28 で新設または deprecated となった機能で、このサーバーが提供していない
ため対応不要。セッション（`Mcp-Session-Id`）と SSE 再開（`Last-Event-ID`）は元から未使用で、
2026-07-28 の削除とすでに整合している。

## クライアント登録

| 方式 | 状態 |
|---|---|
| Client ID Metadata Documents (CIMD) | 推奨。`client_id_metadata_document_supported: true` で広告 |
| Dynamic Client Registration (DCR) | 後方互換用に維持（`/mcp/register`、署名 JWS を `client_id` として発行） |

### CIMD

`client_id` が https URL の場合、その URL からクライアントメタデータ文書を取得して検証する
（`src/oauth/cimd.ts`）。文書の `client_id` は URL と完全一致していなければならず、
`redirect_uris` は認可リクエストの `redirect_uri` 照合に使う。

クライアント指定の URL に対してサーバーが外向き通信を行うため、以下で囲い込む:

- https のみ / パス必須 / fragment・credentials 禁止
- IP リテラル・`localhost`・`.local`・`.localhost`・`.internal`・`.home.arpa`・ドット無しホストを拒否
  （リンクローカルのメタデータエンドポイント等への SSRF 防止）
- リダイレクト追跡なし（`redirect: "error"`）、5 秒タイムアウト、32KB 上限、`application/json` 以外を拒否
- `Cache-Control: max-age` を尊重したインメモリキャッシュ（60 秒〜24 時間、既定 5 分、最大 256 エントリ）
- 任意の許可ホストリスト（`cimd.allowed_hosts`、完全一致の正規表現。既定は空＝公開ホストなら許可）

設定例:

```json
{
    "cimd": {
        "enabled": true,
        "allowed_hosts": ["claude\\.ai", ".*\\.anthropic\\.com"]
    }
}
```

`enabled: false` で CIMD を無効化でき、その場合 URL 形式の `client_id` は `invalid_client`
として拒否される（DCR のみ受け付ける）。

### 認可レスポンスの `iss`

認可コードを返すリダイレクト（単一スペースの `/mcp/authorize/callback`、
複数スペースの `/mcp/authorize/complete`）には RFC 9207 の `iss` パラメーターを付与する。
クライアントはコードを交換する前に、記録した issuer と一致するか検証できる（mix-up 攻撃対策）。
AS メタデータでは `authorization_response_iss_parameter_supported: true` として広告する。

### PKCE と認可コードのバインディング（下流：MCP クライアント ↔ 本サーバー）

本サーバーは MCP クライアントに対する認可サーバー (AS) として PKCE (S256) を必須とする。
`/mcp/authorize` は `code_challenge` が無ければ `invalid_request` を返し、`S256` 以外の
`code_challenge_method` を拒否する。

発行する認可コード（JWS）には以下を埋め込み、`/mcp/token` で全て照合する
（`verifyCodeBinding()`）。単一スペースの `/mcp/authorize/callback` と複数スペースの
`/mcp/authorize/complete` の双方で同じクレームを載せる。

| クレーム | 照合内容 |
|---|---|
| `code_challenge` / `code_challenge_method` | `base64url(SHA-256(code_verifier))` と定数時間比較 |
| `client_id` | リクエストの `client_id` と一致すること |
| `redirect_uri` | リクエストの `redirect_uri` と一致すること |

`code_challenge` を持たないコードは**ダウングレードさせず拒否する**（`invalid_grant`）。
認可コードの寿命は 300 秒なので、デプロイ跨ぎで旧形式のコードを受理する必要はない。

未対応: 認可コードの単回利用強制。ステートレス設計のため `jti` と使用済みストアが必要で、
現状は有効期間（300 秒）内の再利用を防げない。

### 上流（本サーバー ↔ Backlog 認可サーバー）で PKCE を使わない理由

Backlog の OAuth 2.0 は PKCE に対応していないため、上流では `code_challenge` を送らず、
`client_secret` を用いた認可コード交換を中継サーバー経由で行う（`docs/design/oauth-relay-server.md`）。
この制約は下流の PKCE とは独立しており、下流の PKCE 必須化に影響しない。

#### 非対応の実測根拠（2026-09-02 時点）

Backlog は AS メタデータを公開していないため（`/.well-known/oauth-authorization-server`
等はいずれも 404）、実際にエンドポイントを叩いて確認した。authorize エンドポイントは
ログイン前に一切パラメーター検証をせず（不正な `code_challenge_method` でも同じ 303 を返す）
判定材料にならないため、トークンエンドポイント `POST /api/v2/oauth2/token` で測定した。

| プローブ | 結果 | 判定 |
|---|---|---|
| `client_secret` なし / `code_verifier` なし | `401 invalid_client` | — |
| `client_secret` なし / `code_verifier` あり | `401 invalid_client`（上と完全同一） | `code_verifier` は `client_secret` を代替しない＝public client 非対応 |
| `client_secret` あり / `code_verifier` なし | `400 invalid_grant` | 基準 |
| `client_secret` あり / 正常形式の `code_verifier`（48 文字） | `400 invalid_grant`（同一） | — |
| `client_secret` あり / 5 文字の `code_verifier` | `400 invalid_grant`（同一） | RFC 7636 の 43〜128 文字制約を検証していない |
| `client_secret` あり / 不正文字を含む `code_verifier` | `400 invalid_grant`（同一） | unreserved 文字の制約も検証していない |

`error_description` はいずれも `"Authorized information is not found by the code"` で、
コード検索の段階で停止しており `code_verifier` に到達していない。形式違反を弾かない
ことから、RFC 準拠の検証ロジックは存在しないと判断した。

再確認する場合は bogus な `code` で上記 6 パターンを投げ直せばよい（承認操作は不要。
`invalid_client` と `invalid_grant` の切り替わりで client 認証の要否が分かる）。
`code_verifier` の有無・形式でレスポンスが変わるようになっていれば、対応した可能性がある。

Backlog が将来 PKCE に対応した場合のトレードオフ:

- 利点: 中継サーバーが `client_secret` を保持・行使する必要がなくなり、共有中継サーバーを
  立てる際の「中継側でトークンを使われる」リスクが消える
- 欠点: 監査機能（アクセストークンでユーザー情報を取得して監査ログに残す、
  `audit.collect_user_info`）はトークンが中継を通らなくなるため成立しない

## セットアップ手順

### 前提条件

- Node.js 22+、Go 1.23+、pnpm がインストール済み
- AWS アカウント（Lambda デプロイの場合）

### 1. 設定ファイルの作成

#### Relay 統合デプロイ（推奨）

```bash
cd packages/relay-aws
cp config.example.ts config.ts
```

`config.ts` を編集。テナント設定は `tenants` ディクトに統合されています:

```typescript
export const config: RelayConfig = {
    parameterName: "/backlog-relay/config",
    parameterValue: {
        server: {},
        backlog_apps: [
            {
                domain: "backlog.jp",
                client_id: "your-client-id",
                client_secret: "your-client-secret",
            },
        ],
        tenants: {
            "your-space.backlog.jp": {
                // Relay バンドル署名設定（オプション）
                relay: {
                    jwks: { keys: [{ kty: "OKP", crv: "Ed25519", kid: "2025-01", x: "...", d: "..." }] },
                    active_keys: "2025-01",
                    passphrase: "your-passphrase",  // デプロイ時に自動 bcrypt ハッシュ化
                },
                // MCP アクセス制御設定（オプション）
                mcp: {
                    cli_access: {
                        allow: ["issue list *", "issue view *", "project list *", "project view *",
                                "wiki list *", "wiki view *", "notification list *",
                                "api /api/v2/* -X GET"],
                        deny: [],
                    },
                    script: { enabled: false, max_cli_calls: 20, timeout_ms: 30000 },
                },
            },
        },
    },
    // MCP 有効化（いずれかのテナントに mcp 設定がある場合に自動検出）
    mcp: {
        tokenKeyRotationDays: 30,  // MCP token key の自動ローテーション間隔
    },
};
```

**シークレットの自動管理:**
- `client_secret`、JWKS 秘密鍵、passphrase ハッシュ → Secrets Manager に自動分離
- MCP token key → Secrets Manager で自動生成・ローテーション
- SSM Parameter Store には非秘匿情報のみ保存

#### Docker デプロイ

環境変数 `MCP_CONFIG` に JSON 文字列で設定を渡します。Docker の場合は `token_key` を JSON に含めます:

```bash
docker run -p 8080:8080 \
  -e MCP_CONFIG='{"base_url":"https://...","relay_url":"https://...","token_key":"<base64url-key>","backlog_apps":[...],"tenants":{...}}' \
  backlog-mcp-server
```

### 2. デプロイ

#### Lambda (CDK)

```bash
cd packages/relay-aws
pnpm cdk deploy
```

デプロイ後に出力される `FunctionUrl` を `config.ts` の `server.base_url` に設定し、再度デプロイします。
初回デプロイで Secrets Manager に以下が自動作成されます:
- Relay secrets（client_secret, JWKS, passphrase_hash）
- MCP token key（ランダム生成、自動ローテーション）

#### Docker

```bash
docker build -f packages/mcp-server/Dockerfile -t backlog-mcp-server .
docker run -p 8080:8080 -e MCP_CONFIG='...' backlog-mcp-server
```

### 4. Deno + Pyodide のセットアップ (run_script 有効化時)

`run_script` ツールを使用する場合、Deno バイナリと Pyodide WASM キャッシュが必要です。

```bash
# Deno バイナリを vendor/ に配置
mkdir -p packages/mcp-server/vendor
curl -fsSL https://github.com/denoland/deno/releases/latest/download/deno-aarch64-unknown-linux-gnu.zip -o /tmp/deno.zip
unzip -o /tmp/deno.zip -d packages/mcp-server/vendor/

# Pyodide WASM をキャッシュ
cd packages/mcp-server
DENO_DIR=.deno-cache deno cache src/sandbox/sandbox-worker.mjs
```

### 5. ユーザー側の設定

#### Claude Desktop

`claude_desktop_config.json` に追加:

```json
{
  "mcpServers": {
    "backlog": {
      "url": "https://your-function-url.lambda-url.ap-northeast-1.on.aws/mcp"
    }
  }
}
```

初回アクセス時にブラウザが開き、Backlog にログインすれば完了です。

## テナント設定

### アクセス制御パターン

`cli_access` の `allow` / `deny` はグロブパターンで CLI コマンドを制御します。`deny` は `allow` より優先されます。

**読み取り専用:**
```json
{
    "allow": ["issue list *", "issue view *", "pr list *", "pr view *",
              "wiki list *", "wiki view *", "project list *", "project view *",
              "notification list *", "api /api/v2/* -X GET"],
    "deny": []
}
```

**フル機能:**
```json
{
    "allow": ["*"],
    "deny": ["config *", "auth *"]
}
```

### run_script 設定

```json
{
    "script": {
        "enabled": true,
        "max_cli_calls": 30,
        "timeout_ms": 30000
    }
}
```

- `max_cli_calls`: 1 回のスクリプト実行で呼べる `backlog()` の回数上限
- `timeout_ms`: スクリプト実行のタイムアウト

## シークレット管理

### Secrets Manager 構成

CDK スタックが以下の 2 つの Secrets Manager シークレットを自動作成します:

| シークレット | 名前 | 内容 |
|------------|------|------|
| Relay secrets | `{parameterName}-secrets` | apps の client_secret + tenants の JWKS/passphrase_hash |
| MCP token key | 設定可能 (default: `/backlog-mcp/token-key`) | base64url 32 バイト AES-256 鍵 |

SSM Parameter Store には非秘匿情報のみ保存されます。Lambda ハンドラがコールドスタート時に
SM から秘匿情報を読み込み、SSM の設定とマージして使用します。

### MCP token key ローテーション

CDK スタックがローテーション Lambda を自動作成し、設定間隔（デフォルト 30 日）で鍵をローテーションします。

- **`AWSCURRENT`** → 暗号化 + 復号に使用（`token_key`）
- **`AWSPREVIOUS`** → 復号のみに使用（`token_key_prev`）

`mcp.tokenKeyRotationDays` で間隔を設定。`0` でローテーション無効。

手動ローテーション:
```bash
aws secretsmanager rotate-secret --secret-id /backlog-mcp/token-key
```

### Docker / スタンドアロン

`MCP_CONFIG` JSON 内の `token_key` と `token_key_prev` を手動で管理します:

1. 新しい鍵を生成
2. `token_key_prev` に現在の `token_key` の値を設定
3. `token_key` に新しい鍵を設定
4. サーバーを再起動

## セキュリティモデル

| 脅威 | 対策 |
|------|------|
| トークン窃取 | JWE 暗号化 + HTTPS 必須 |
| トークンリプレイ | `exp` クレームで有効期限を強制 |
| 鍵漏洩 | SM 自動ローテーション + `AWSPREVIOUS` で旧鍵復号 |
| シークレット漏洩 | client_secret/JWKS/passphrase を SM に分離、SSM には非秘匿情報のみ |
| 不正な redirect_uri | DCR 時に登録された URI を client_id JWS に内包し、認可時に検証。CIMD ではメタデータ文書の `redirect_uris` と照合 |
| CIMD 解決による SSRF | https/パス必須、IP・ローカルホスト系を拒否、リダイレクト追跡なし、タイムアウト・サイズ上限、任意の許可ホストリスト |
| 認可サーバー mix-up | 認可レスポンスに `iss` を付与（RFC 9207） |
| ヘッダとボディの不整合 | ミラーヘッダ (`Mcp-Method` / `Mcp-Name` / `MCP-Protocol-Version`) の不一致を `-32020` で拒否 |
| コマンドインジェクション | `execFile` で引数を配列渡し（シェル経由しない） |
| CLI 設定ファイル干渉 | `HOME=/tmp` で既存設定を隔離 |
| sandbox エスケープ | Deno 権限 (OS 層) + Pyodide import 制限 (アプリ層) の二重防御 |
| CLI 乱用 | `max_cli_calls` で呼出回数制限 |

## デプロイ方式

### A. Relay 統合デプロイ（推奨）

既存の Relay サーバーと同一 Lambda で MCP エンドポイントを提供します。

- CloudFront / ドメインを共有（追加インフラ不要）
- トークン交換がインプロセス（HTTP ラウンドトリップ不要）
- テナントの `mcp` フィールドで有効化（統合テナント設定）
- シークレットは SM に自動分離、SSM には非秘匿情報のみ

MCP 有効時（いずれかのテナントに `mcp` 設定がある場合）、CDK スタックが自動的に:
- Go CLI バイナリ + Deno + sandbox-worker を Lambda にバンドル
- MCP token key を SM で自動生成 + ローテーション Lambda 作成
- Relay secrets（client_secret, JWKS, passphrase_hash）を SM に保存
- メモリ 512MB → 1024MB、タイムアウト 10s → 120s に拡張

`TokenExchange` インターフェースにより、MCP OAuth ハンドラが Relay の `BacklogAppConfig`（client_secret 含む）を使って Backlog API に直接トークン交換を行います。`relay_url` 経由の HTTP 呼び出しは不要です。

### B. スタンドアロンデプロイ

`packages/mcp-aws/` を使用して独立した Lambda としてデプロイします。
Relay サーバーへの HTTP 呼び出し（`relay_url`）でトークン交換を行います。

## パッケージ構成

```
packages/
├── mcp-server/          # MCP サーバー本体（ライブラリ）
│   ├── src/
│   │   ├── index.ts          # Hono app エントリーポイント
│   │   ├── serve.ts          # standalone サーバー (node:http)
│   │   ├── config/schema.ts  # Zod バリデーション
│   │   ├── crypto/jwe.ts     # JWE 暗号化/復号 (jose)
│   │   ├── oauth/            # MCP OAuth AS (CIMD + DCR + PKCE + TokenExchange)
│   │   ├── transport/        # Streamable HTTP (dual-era: modern + legacy)
│   │   ├── middleware/       # JWE 認証 + CLI アクセス制御
│   │   ├── tools/            # backlog CLI ツール
│   │   └── sandbox/          # Deno + Pyodide sandbox
│   └── Dockerfile            # Docker / Lambda Web Adapter 兼用
├── relay-aws/           # Relay + MCP 統合デプロイ (CDK) ← 推奨
│   ├── lib/relay-stack.ts    # CDK スタック（SM シークレット + MCP 条件付きバンドル）
│   ├── lib/handler.ts        # Lambda ハンドラ（SM マージ + relay + MCP マウント）
│   ├── lib/rotation-handler.ts  # MCP token key ローテーション Lambda
│   ├── lib/types.ts          # 統合設定型（UnifiedTenantInput, McpConfig）
│   └── config.example.ts     # 設定テンプレート
└── mcp-aws/             # MCP スタンドアロンデプロイ (CDK)
    ├── lib/mcp-stack.ts      # CDK スタック定義
    ├── lib/handler.ts        # Lambda ハンドラ
    └── config.example.ts     # 設定テンプレート
```
