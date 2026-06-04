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

- Relay サーバーがデプロイ済み（Backlog OAuth Client ID/Secret を管理）
- Node.js 22+、Go 1.23+、pnpm がインストール済み
- AWS アカウント（Lambda デプロイの場合）

### 1. JWE 暗号鍵の生成

```bash
node -e "console.log(require('crypto').randomBytes(32).toString('base64url'))"
```

この鍵は `token_key` として設定します。ユーザートークンの暗号化に使用されるため、安全に管理してください。

### 2. 設定ファイルの作成

#### Lambda デプロイ (CDK)

```bash
cd packages/mcp-aws
cp config.example.ts config.ts
```

`config.ts` を編集:

```typescript
export const config: McpStackConfig = {
    parameterName: "/backlog-mcp/config",
    parameterValue: {
        base_url: "https://your-function-url.lambda-url.ap-northeast-1.on.aws",
        relay_url: "https://your-relay-server.example.com",
        token_key: "<上で生成した鍵>",
        backlog_apps: [
            {
                domain: "backlog.jp",
                client_id: "<Relay と同じ client_id>",
            },
        ],
        tenants: {
            "your-space.backlog.jp": {
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
};
```

#### Docker デプロイ

環境変数 `MCP_CONFIG` に JSON 文字列で設定を渡します:

```bash
docker run -p 8080:8080 \
  -e MCP_CONFIG='{"base_url":"https://...","relay_url":"https://...","token_key":"...","backlog_apps":[...],"tenants":{...}}' \
  backlog-mcp-server
```

### 3. デプロイ

#### Lambda (CDK)

```bash
cd packages/mcp-aws
pnpm cdk deploy
```

デプロイ後に出力される `FunctionUrl` を `config.ts` の `base_url` に設定し、再度デプロイします。

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

## 鍵ローテーション

1. 新しい鍵を生成
2. `token_key_prev` に現在の `token_key` の値を設定
3. `token_key` に新しい鍵を設定
4. デプロイ
5. 全ユーザーが新しいトークンを取得した後（= 旧トークンが全て期限切れ後）、`token_key_prev` を削除

`token_key_prev` が設定されている間、復号は両方の鍵で試行されます。暗号化は常に `token_key` を使用します。

## セキュリティモデル

| 脅威 | 対策 |
|------|------|
| トークン窃取 | JWE 暗号化 + HTTPS 必須 |
| トークンリプレイ | `exp` クレームで有効期限を強制 |
| 鍵漏洩 | 鍵ローテーション対応 (`token_key_prev`) |
| 不正な redirect_uri | DCR 時に登録された URI を client_id JWE に内包し、認可時に検証 |
| コマンドインジェクション | `execFile` で引数を配列渡し（シェル経由しない） |
| CLI 設定ファイル干渉 | `HOME=/tmp` で既存設定を隔離 |
| sandbox エスケープ | Deno 権限 (OS 層) + Pyodide import 制限 (アプリ層) の二重防御 |
| CLI 乱用 | `max_cli_calls` で呼出回数制限 |

## パッケージ構成

```
packages/
├── mcp-server/          # MCP サーバー本体
│   ├── src/
│   │   ├── index.ts          # Hono app エントリーポイント
│   │   ├── serve.ts          # standalone サーバー (node:http)
│   │   ├── config/schema.ts  # Zod バリデーション
│   │   ├── crypto/jwe.ts     # JWE 暗号化/復号 (jose)
│   │   ├── oauth/            # MCP OAuth AS (DCR + PKCE)
│   │   ├── transport/        # Streamable HTTP (POST/GET/DELETE /mcp)
│   │   ├── middleware/       # JWE 認証 + CLI アクセス制御
│   │   ├── tools/            # backlog CLI ツール
│   │   └── sandbox/          # Deno + Pyodide sandbox
│   └── Dockerfile            # Docker / Lambda Web Adapter 兼用
└── mcp-aws/             # Lambda デプロイ (CDK)
    ├── lib/mcp-stack.ts      # CDK スタック定義
    ├── lib/handler.ts        # Lambda ハンドラ
    └── config.example.ts     # 設定テンプレート
```
