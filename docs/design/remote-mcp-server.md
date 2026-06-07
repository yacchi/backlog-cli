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
| 不正な redirect_uri | DCR 時に登録された URI を client_id JWE に内包し、認可時に検証 |
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
│   │   ├── oauth/            # MCP OAuth AS (DCR + PKCE + TokenExchange)
│   │   ├── transport/        # Streamable HTTP (POST/GET/DELETE /mcp)
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
