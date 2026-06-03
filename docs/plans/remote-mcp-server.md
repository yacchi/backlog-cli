# Remote MCP Server 実装プラン

## 1. 背景と動機

### 1.1 課題

Backlog の AI 連携ツール（公式 MCP サーバー、BeeCLI）は、ユーザーが API キーを自分で取得・設定する必要がある。
組織でエンジニア・非エンジニア両方に展開する場合、このハードルは現実的に高い。

### 1.2 このプロジェクトの強み

backlog-cli は以下のインフラを既に持つ:

- **OAuth Relay サーバー**: Client Secret をサーバー側で管理し、ユーザーはブラウザでログインするだけ
- **Relay Config Bundle**: 組織が配布する設定バンドルで、CLI が正当な relay サーバーのみを利用することを保証
- **テナント設定**: 組織単位での設定管理基盤

### 1.3 ゴール

既存の relay インフラに MCP Server 機能を統合し、**ユーザーは URL を追加してログインするだけ**で Backlog を Claude から利用可能にする。

### 1.4 設計原則

- **サーバーにユーザーの認証情報を保存しない**（ステートレス）
- **Relay サーバーとは独立したサービス**（性質が異なるため分離）
- **汎用 HTTP サーバーとして構築**（プラットフォーム固有コードなし。Lambda Web Adapter で Lambda にもそのままデプロイ）
- **組織が公開ツールを制御可能**

### 1.5 公式 MCP サーバーとの棲み分け

| | 公式 Backlog MCP Server | Remote MCP Server (本プラン) |
|---|---|---|
| 対象 | 個人（開発者） | 組織（全ユーザー） |
| 認証 | API キー（手動取得） | OAuth（ブラウザログイン） |
| トランスポート | stdio（ローカル専用） | Streamable HTTP（リモート） |
| 設定 | 環境変数 | URL 追加のみ |
| マルチテナント | 非対応 | 対応（ステートレス） |
| ツール制御 | なし | テナント設定で制御 |

サイドカー構成（公式 MCP の前段に認証プロキシ）は、公式サーバーが API キー専用・シングルユーザー前提のため不適合。

---

## 2. アーキテクチャ

### 2.1 全体構成

MCP サーバーは relay サーバーとは**独立したサービス**として構築する。
性質が異なる（relay: 軽量 OAuth 仲介 / MCP: CLI 実行 + sandbox）ため、同一コードベースにする必要はない。

```
┌──────────────────┐                       ┌──────────────────────────────────┐
│ Claude Desktop   │                       │  MCP Server (独立サービス)        │
│ claude.ai        │  Streamable HTTP      │                                  │
│ Claude Code      │◄─────────────────────►│  ┌───────────┐  ┌────────────┐  │
│                  │  Bearer: JWE token     │  │ MCP OAuth  │  │ MCP Tools  │  │
└──────┬───────────┘                       │  │ AS         │  │ backlog CLI │  │
       │                                    │  └─────┬─────┘  │ + sandbox  │  │
       │  ブラウザで                         │        │        └─────┬──────┘  │
       │  Backlog ログイン                   │        │              │         │
       └────────────────────────────────────│        │              │         │
                                            └────────┼──────────────┼─────────┘
                                                     │              │
                                            ┌────────┴──────┐  ┌───┴───────────┐
                                            │ Relay Server  │  │ Backlog API   │
                                            │ (token 交換)  │  │               │
                                            └───────┬───────┘  └───────────────┘
                                                    │
                                                    ▼
                                            ┌───────────────┐
                                            │ Backlog OAuth  │
                                            └───────────────┘

MCP Server が保持:  JWE 暗号鍵 (環境変数)
MCP Server が保持しない: ユーザートークン, セッション, DB, Client Secret
Client Secret は Relay Server 側で管理（既存のまま）
```

### 2.2 デプロイ構成

MCP サーバーは **Node.js + Hono** の汎用 HTTP サーバーとして構築する。
`run_script` の sandbox は **常駐 Deno プロセス + Pyodide** (Python on WebAssembly) で実現する。
Deno の権限システム（`--deny-net`, `--deny-run` 等）が OS レベルの隔離を提供し、
Pyodide 側のアプリ層制限（`sys.meta_path` + builtins 上書き）と合わせて二重防御となる。
Node.js → Deno 間は HTTP loopback (127.0.0.1) で通信。Pyodide はウォーム状態を維持する。

```
┌─ Node.js プロセス ─────────────────────────────────────────┐
│                                                             │
│  Hono (PORT=8080)                                           │
│  ├── MCP OAuth AS                                           │
│  ├── Streamable HTTP Transport                              │
│  ├── backlog tool → Go CLI subprocess                       │
│  ├── run_script tool → HTTP POST to Deno sandbox ──────┐    │
│  └── /health                                            │    │
│                                                         │    │
│  bin/backlog (Go CLI, linux/arm64 or amd64)             │    │
└─────────────────────────────────────────────────────────┼────┘
                                                          │
┌─ Deno プロセス (常駐, 子プロセス) ──────────────────────────┐
│  --deny-net (127.0.0.1 除く) --deny-env --deny-run        │
│  --deny-write --allow-read=<pyodide-cache>                 │
│                                                            │
│  Pyodide (Python on WASM, warm)                            │
│  ├── sys.meta_path finder (import 制限)                    │
│  ├── builtins.open / input 無効化                          │
│  ├── backlog() → HTTP callback → Node.js → Go CLI         │
│  └── HTTP server on 127.0.0.1:<random-port>                │
└────────────────────────────────────────────────────────────┘
```

#### デプロイパターン

| 環境 | 方式 | 備考 |
|------|------|------|
| **Lambda (推奨)** | `NodejsFunction` | native addon 不要。Go CLI + Deno バイナリを commandHooks でバンドル |
| **Lambda (代替)** | `DockerImageFunction` + Lambda Web Adapter | Docker イメージをそのまま Lambda に |
| **コンテナ** (ECS, Cloud Run 等) | Docker イメージ | `docker run -p 8080:8080` でそのまま動作 |
| **Cloudflare Workers** | 対象外 | サブプロセス実行不可 |

**NodejsFunction デプロイ（推奨）:**

```typescript
// packages/mcp-aws/lib/mcp-stack.ts
const fn = new lambda_nodejs.NodejsFunction(this, "McpServer", {
  entry: "../mcp-server/src/index.ts",
  runtime: lambda.Runtime.NODEJS_22_X,
  architecture: lambda.Architecture.ARM_64,
  memorySize: 1024,               // Deno + Pyodide WASM 用に余裕を持たせる
  timeout: Duration.seconds(120),
  bundling: {
    // forceDockerBundling 不要 — Node.js 側に native addon なし
    commandHooks: {
      beforeBundling: () => [],
      afterBundling: (inputDir, outputDir) => [
        // Go CLI バイナリ
        `cp ${inputDir}/../backlog/dist/backlog-linux-arm64 ${outputDir}/bin/backlog`,
        `chmod +x ${outputDir}/bin/backlog`,
        // Deno バイナリ (sandbox host)
        `cp ${inputDir}/../mcp-server/vendor/deno ${outputDir}/bin/deno`,
        `chmod +x ${outputDir}/bin/deno`,
        // Deno sandbox worker
        `cp ${inputDir}/../mcp-server/src/sandbox-worker.mjs ${outputDir}/sandbox-worker.mjs`,
        // Pyodide WASM + Deno cache (ビルド時に解決済み、ColdStart でダウンロード不要)
        `cp -r ${inputDir}/../mcp-server/.deno-cache ${outputDir}/.deno-cache`,
      ],
      beforeInstall: () => [],
    },
  },
  environment: {
    DENO_DIR: "/var/task/.deno-cache",
    MCP_TOKEN_KEY: "...",
    RELAY_URL: "https://...",
  },
});

const url = fn.addFunctionUrl({ authType: lambda.FunctionUrlAuthType.NONE });
```

**Docker デプロイ（代替 / コンテナ環境用）:**

```dockerfile
FROM golang:1.23 AS go-builder
WORKDIR /build
COPY packages/backlog/ .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -o backlog ./cmd/backlog

FROM node:22-slim AS node-builder
WORKDIR /app
COPY packages/mcp-server/ .
RUN npm ci && npm run build

FROM node:22-slim
COPY --from=public.ecr.aws/awsguru/aws-lambda-web-adapter:0.9.1 /lambda-adapter /opt/extensions/lambda-adapter

WORKDIR /app
COPY --from=go-builder /build/backlog /app/bin/backlog
COPY --from=node-builder /app/dist /app/dist
COPY --from=node-builder /app/node_modules /app/node_modules

ENV PORT=8080
EXPOSE 8080
CMD ["node", "dist/index.js"]
```

### 2.3 パッケージ構成

```
packages/
├── relay-core/          # 既存 (変更なし)
├── relay-aws/           # 既存 (変更なし)
├── relay-cloudflare/    # 既存 (変更なし)
├── relay-docker/        # 既存 (変更なし)
├── mcp-server/          # 新規: MCP サーバー本体
│   ├── src/
│   │   ├── index.ts          # Hono app エントリーポイント (PORT=8080 で listen)
│   │   ├── oauth/            # MCP OAuth AS (JWE ベースステートレス認証)
│   │   ├── transport/        # Streamable HTTP トランスポート
│   │   ├── tools/            # ツール定義・ディスパッチ
│   │   ├── sandbox-client.ts # Deno sandbox との IPC クライアント
│   │   └── sandbox-worker.mjs # Deno 側: Pyodide sandbox (常駐 HTTP サーバー)
│   ├── bin/
│   │   ├── backlog           # Go CLI バイナリ (ビルド時に配置)
│   │   └── deno              # Deno バイナリ (ビルド時に配置)
│   └── Dockerfile            # Docker / Lambda Web Adapter 兼用
├── mcp-aws/             # 新規: Lambda デプロイ (CDK)
└── backlog/             # 既存 CLI (Go)
```

MCP OAuth AS は relay サーバーの `/auth/token` を内部的に利用してトークン交換を行う。
これにより Client Secret は relay サーバー側のみで管理される。

---

## 3. ステートレス認証設計

### 3.1 核心: JWE ラップトークン

サーバーに認証情報を保存せず、ユーザーの Backlog トークンを **JWE (JSON Web Encryption)** で暗号化してクライアント（Claude）側に保持させる。

既存 relay の state エンコーディング（base64url JSON を state パラメータに埋め込む）と同じ発想だが、トークンには暗号化が必要。

```
┌─────────────────────────────────────────────────────────┐
│ JWE (MCP access_token として Claude が保持)               │
│                                                         │
│  暗号化ペイロード:                                        │
│  {                                                      │
│    "bl_access_token": "Backlog のアクセストークン",       │
│    "bl_expires_at": 1704067200,                         │
│    "space": "mycompany",                                │
│    "domain": "backlog.jp",                              │
│    "iat": 1704063600,                                   │
│    "exp": 1704067200                                    │
│  }                                                      │
│                                                         │
│  暗号鍵: サーバー環境変数 MCP_TOKEN_KEY (AES-256-GCM)     │
└─────────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────┐
│ JWE (MCP refresh_token として Claude が保持)              │
│                                                         │
│  暗号化ペイロード:                                        │
│  {                                                      │
│    "bl_refresh_token": "Backlog のリフレッシュトークン",   │
│    "space": "mycompany",                                │
│    "domain": "backlog.jp",                              │
│    "iat": 1704063600                                    │
│  }                                                      │
│                                                         │
│  暗号鍵: 同上                                            │
└─────────────────────────────────────────────────────────┘
```

### 3.2 MCP OAuth 認証フロー

MCP 仕様の OAuth 2.1 + PKCE に準拠し、既存 relay の Backlog OAuth フローを内部で利用する。

#### ディスカバリ

```
GET /.well-known/oauth-protected-resource
→ {
    "resource": "https://relay.example.com/mcp",
    "authorization_servers": ["https://relay.example.com"],
    "scopes_supported": ["backlog"]
  }

GET /.well-known/oauth-authorization-server
→ {
    "issuer": "https://relay.example.com",
    "authorization_endpoint": "https://relay.example.com/mcp/authorize",
    // ... 670 lines omitted
  }
    // ... 669 lines omitted
{
    // ... 668 lines omitted
}
    // ... 667 lines omitted
  }
    // ... 666 lines omitted
{
    // ... 665 lines omitted
      }
    // ... 664 lines omitted
  }
}
    // ... 662 lines omitted
{
    // ... 661 lines omitted
      }
    // ... 660 lines omitted
  }
}
    // ... 658 lines omitted
    }
  }
    // ... 656 lines omitted
{
    // ... 655 lines omitted
      }
    // ... 654 lines omitted
  }
}
    // ... 652 lines omitted
}
    // ... 651 lines omitted
{
    // ... 650 lines omitted
      }
    // ... 649 lines omitted
  }
}
    // ... 647 lines omitted
interface McpConfig {
    // ... 646 lines omitted
}
    // ... 645 lines omitted
{
    // ... 644 lines omitted
  }
}
    // ... 642 lines omitted
{
    // ... 641 lines omitted
  }
}
    // ... 639 lines omitted
{
    // ... 638 lines omitted
    }
  }
}
    // ... 635 lines omitted
         }
// ... 634 more lines (total: 828)
## 4. MCP ツール設計 — CLI ベースメタツールアーキテクチャ

### 4.1 設計思想

**公式 Backlog MCP サーバーの課題:**
- 60+ ツール定義でコンテキストウィンドウを圧迫
- 結果サイズ上限で応答が切り詰められる
- AI が 60 個から適切なツールを選ぶのは非効率
- 新 API 追加のたびにツール追加が必要

**AWS MCP サーバーの解決策** (参考: 2025 年 GA):
15,000+ API を `call_aws` + `search_documentation` + `run_script` の 3 メタツールに集約。
Skills（ベストプラクティスガイダンス）を併用。

**本プランの進化:** AWS のメタツール思想 + backlog-cli 固有の強みを組み合わせる。

backlog-cli は `gh` (GitHub CLI) と同等の使い方を目指して設計されている。
AI モデルは `gh` のコマンド体系と出力加工パターンに深い知識を持つ。
**この知識をそのまま転用可能にする。**

| 要素 | AWS MCP | 本プラン |
|------|---------|--------|
| API 仕様の提供 | `search_documentation`（毎回ツール呼出） | **Skill で事前注入**（0 ラウンドトリップ） |
| API 呼び出し | `call_aws`（生 API 指定） | **CLI コマンド実行**（`gh` パターン流用） |
| 複雑な処理 | `run_script`（Python sandbox） | **`run_script`**（V8 isolate sandbox + CLI ヘルパー） |

### 4.2 ツール構成: 2 ツール + 1 Skill

```
┌─ Skill (MCP prompts / セッション開始時に注入) ─────────────┐
│                                                           │
│  backlog CLI リファレンス:                                  │
│  - gh との対応表 (gh issue list → backlog issue list)      │
│  - 全コマンド + フラグ一覧                                  │
│  - 出力形式 (--json, --jq, --format)                      │
│  - プロジェクト固有メタデータ (課題タイプ/ステータス ID 等)    │
│                                                           │
│  → search_documentation 不要（事前注入で 0 RTT）           │
│  → get_context 不要（Skill に含めるか backlog コマンドで取得）│
└───────────────────────────────────────────────────────────┘

┌─ Tool 1: backlog ─────────────────────────────────────────┐
│  CLI コマンドを実行して結果を返す                            │
│  入力: コマンド引数文字列                                    │
│  実行: サーバー上の backlog バイナリをサブプロセスで起動      │
│  認証: JWE から復号した OAuth トークンを env var で注入      │
└───────────────────────────────────────────────────────────┘

┌─ Tool 2: run_script (JavaScript, isolated-vm) ───────────┐
│  複数 CLI 呼出 + フィルタ/集計を 1 往復で実行                │
│  V8 isolate 内で実行（Node.js API アクセス不可）            │
│  backlog() host callback が CLI を呼出（認証・アクセス制御はホスト側）│
└───────────────────────────────────────────────────────────┘
```

#### Tool 1: `backlog` — CLI コマンド実行

```json
{
  "name": "backlog",
  "description": "Execute a backlog CLI command (similar to gh CLI). Returns command output. Use --json flag for structured output, --jq for filtering.",
  "inputSchema": {
    "type": "object",
    "properties": {
      "args": {
        "type": "string",
        "description": "CLI arguments (e.g., 'issue list --project PROJ -L 20 --json issueKey,summary,status')"
      }
    },
    "required": ["args"]
  }
}
```

**実行例:**

```
# gh 知識からの自然な転用
backlog({ args: "issue list --project PROJ-A --assignee @me --json issueKey,summary,dueDate,status" })
backlog({ args: "issue view PROJ-A-42 --comments --json" })
backlog({ args: "pr list --project PROJ-A --status Open" })
backlog({ args: "wiki view 12345" })
backlog({ args: "notification list -L 10" })

# raw API アクセス（CLI にない操作のフォールバック）
backlog({ args: "api /api/v2/issues/count --method GET -f projectId[]=12345" })
```

**サーバー側の実行:**

```typescript
// packages/mcp-server/src/tools/backlog.ts
import { execFile } from "node:child_process";

async function executeBacklogCommand(args: string, backlogToken: string, tenant: TenantConfig): Promise<string> {
  const { stdout } = await execFile("./bin/backlog", parseArgs(args), {
    env: {
      BACKLOG_ACCESS_TOKEN: backlogToken,  // JWE から復号したトークン
      BACKLOG_SPACE: tenant.space,
      BACKLOG_DOMAIN: tenant.domain,
    },
    timeout: 30000,
  });
  return stdout;
}
```

CLI バイナリは Lambda デプロイパッケージまたは Docker イメージに同梱する。

#### Tool 2: `run_script` — Python Sandbox (Deno 常駐 + Pyodide)

```json
{
  "name": "run_script",
  "description": "Execute Python in a sandboxed environment with backlog() helper. Use for chaining multiple CLI calls, filtering, aggregating, or computing derived data in one round trip. Python standard library available (json, datetime, re, collections, itertools, math, statistics, csv, etc.). Dangerous modules (os, subprocess, socket, etc.) are blocked.",
  "inputSchema": {
    "type": "object",
    "properties": {
      "script": {
        "type": "string",
        "description": "Python code. Available: backlog(args) returns parsed JSON (runs CLI with --json). The last expression is the return value."
      }
    },
    "required": ["script"]
  }
}
```

**ユースケース例:**

```python
# 「各プロジェクトの期限切れ課題を担当者別に集計」
# → 公式 MCP だと 10+ 回のツール呼出 + AI 側処理
# → run_script なら 1 往復

import json
from datetime import date
from collections import defaultdict

projects = backlog("project list --json projectKey,id,name")
today = date.today().isoformat()
result = []

for p in projects:
    issues = backlog(f"issue list --project {p['projectKey']} -L 0 "
        f"--due-date-until {today} --status Open,InProgress "
        f"--json issueKey,summary,assignee,dueDate")
    if not issues:
        continue

    by_assignee = defaultdict(list)
    for i in issues:
        name = (i.get("assignee") or {}).get("name", "未割当")
        by_assignee[name].append({
            "key": i["issueKey"], "summary": i["summary"], "dueDate": i["dueDate"],
        })
    result.append({"project": p["name"], "overdueByAssignee": dict(by_assignee)})

json.dumps(result, ensure_ascii=False, indent=2)
```

```python
# 「先週マージされた PR のサマリ」
import json
from datetime import date, timedelta

last_week = (date.today() - timedelta(days=7)).isoformat()
repos = backlog("repo list --project MYPROJ --json id,name")

merged_prs = []
for repo in repos:
    prs = backlog(f"pr list --project MYPROJ --repo {repo['name']} "
        f"--status Merged --json number,summary,createdUser,updated")
    merged_prs.extend(pr for pr in prs if pr["updated"] >= last_week)

json.dumps(merged_prs, ensure_ascii=False, indent=2)
```

**Sandbox 設計 — 常駐 Deno + Pyodide（二重防御）:**

| 項目 | 仕様 |
|------|------|
| 実行環境 | 常駐 Deno プロセス内の [Pyodide](https://pyodide.org/) (CPython on WebAssembly) |
| 言語 | Python（標準ライブラリ利用可。危険モジュールのみブロック） |
| **防御層 1: Deno 権限** | `--deny-net`(loopback 除く), `--deny-run`, `--deny-write`, `--deny-env` — OS レベルで遮断 |
| **防御層 2: Python import 制限** | `sys.meta_path` に `_SandboxFinder` を挿入。**ブロックリスト**（os, subprocess, socket 等 I/O・プロセス系）に該当するモジュールは `ImportError`。それ以外の標準ライブラリは利用可 |
| **防御層 3: builtins 無効化** | `open()`, `input()`, `io.open()`, `breakpoint()` を例外を投げる関数に差し替え |
| ネットワーク | 不可（Deno `--deny-net` + Python `socket` モジュール import ブロック） |
| ファイルシステム | 不可（Deno `--deny-write` + `open()` 無効化 + `os`/`pathlib` import ブロック） |
| `backlog()` ヘルパー | Pyodide `registerJsModule` で注入。Deno → Node.js への HTTP callback で CLI 実行 |
| 実行時間制限 | Node.js 側 IPC のタイムアウト + Deno 側 AbortSignal |
| メモリ制限 | Deno プロセスの `--v8-flags=--max-heap-size=128` |
| CLI 呼出回数 | Node.js 側の callback handler でカウント（テナント設定 `max_cli_calls`） |
| 起動コスト | 初回 ~800ms (Pyodide WASM ロード)、以降 ~5ms/呼出 (HTTP loopback IPC) |

**ローカル検証結果（macOS arm64）:**

| 計測項目 | 値 |
|---------|-----|
| Deno + Pyodide 起動（コールド） | ~825ms |
| Sandbox セットアップ | ~30ms |
| Warm 実行（IPC 込み） | **~5ms** |
| セキュリティ（import os 等） | 全ブロック確認済み |
| 許可モジュール（json, datetime 等） | 全動作確認済み |

**サーバー側の実装:**

```typescript
// packages/mcp-server/src/sandbox-client.ts
import { spawn, type ChildProcess } from "node:child_process";

class SandboxClient {
  private proc: ChildProcess | null = null;
  private port: number = 0;
  private ready: Promise<void>;

  constructor() {
    this.ready = this.boot();
  }

  private async boot() {
    this.proc = spawn("./bin/deno", [
      "run",
      "--allow-read", "--allow-net=127.0.0.1",
      "--deny-env", "--deny-run", "--deny-write",
      "./sandbox-worker.mjs",
    ]);

    // 最初の stdout 行からポート番号を取得
    this.port = await new Promise((resolve) => {
      this.proc!.stdout!.once("data", (data) => {
        resolve(JSON.parse(data.toString().trim()).port);
      });
    });
  }

  async runScript(script: string, opts: { timeoutMs: number }): Promise<string> {
    await this.ready;
    const res = await fetch(`http://127.0.0.1:${this.port}/`, {
      method: "POST",
      body: script,
      signal: AbortSignal.timeout(opts.timeoutMs),
    });
    const json = await res.json();
    if (!json.ok) throw new Error(json.error);
    return json.result;
  }
}
```

```javascript
// packages/mcp-server/src/sandbox-worker.mjs
// Deno 側: 常駐 HTTP サーバー + Pyodide sandbox
// 起動: deno run --allow-read --allow-net=127.0.0.1
//              --deny-env --deny-run --deny-write sandbox-worker.mjs <callback-url>
import { loadPyodide } from "npm:pyodide";

const callbackUrl = Deno.args[0];  // Node.js 側の backlog() callback endpoint
const pyodide = await loadPyodide();

// backlog() bridge — Node.js 側へ HTTP で CLI 実行を委譲
pyodide.registerJsModule("_backlog_bridge", {
  call: (args) => {
    const res = new XMLHttpRequest();  // Deno 内では同期 XHR が使用可能
    res.open("POST", callbackUrl, false);
    res.send(args);
    if (res.status !== 200) throw new Error(res.responseText);
    return JSON.parse(res.responseText);
  },
});

// Sandbox 制限セットアップ — ブロックリスト方式
// 標準ライブラリは原則利用可。I/O・プロセス操作系のみブロック。
const SANDBOX_SETUP = `
import sys
from importlib.abc import MetaPathFinder
from importlib.machinery import ModuleSpec

# ブロック対象: I/O、ネットワーク、プロセス操作、低レベルアクセス
_BLOCKED_TOP = frozenset({
    'os', 'subprocess', 'socket', 'http', 'urllib', 'xmlrpc',
    'shutil', 'pathlib', 'signal', 'ctypes', 'multiprocessing',
    'tempfile', 'glob', 'fcntl', 'termios', 'pty', 'resource',
    'select', 'selectors', 'asyncio', 'concurrent',
    'threading', 'mmap', 'webbrowser', 'ftplib', 'smtplib',
    'imaplib', 'poplib', 'nntplib', 'telnetlib', 'socketserver',
    'ssl', 'sqlite3', 'dbm', 'shelve', 'pickle',
    'code', 'codeop', 'compileall', 'py_compile',
    'ensurepip', 'venv', 'pip', 'setuptools',
})

# 既にロード済みの危険モジュールを削除
for m in list(sys.modules.keys()):
    if m.split('.')[0] in _BLOCKED_TOP:
        del sys.modules[m]

class _SandboxFinder(MetaPathFinder):
    """ブロックリストに該当するモジュールの import を阻止"""
    def find_spec(self, name, path, target=None):
        top = name.split('.')[0]
        if top in _BLOCKED_TOP:
            return ModuleSpec(name, _BlockLoader())
        return None  # それ以外は通す

class _BlockLoader:
    def create_module(self, spec): return None
    def exec_module(self, mod):
        raise ImportError(f"Module '{mod.__name__}' is not allowed in sandbox")

sys.meta_path.insert(0, _SandboxFinder())

# builtins: I/O 関連のみ無効化
import builtins, io
def _blocked(*a, **kw):
    raise PermissionError("Not allowed in sandbox")
builtins.open = _blocked
builtins.input = _blocked
builtins.breakpoint = _blocked
io.open = _blocked
`;

await pyodide.runPythonAsync(SANDBOX_SETUP);

// HTTP サーバーとしてリッスン
const server = Deno.serve({ hostname: "127.0.0.1", port: 0 }, async (req) => {
  if (req.method !== "POST") return new Response("POST only", { status: 405 });
  const script = await req.text();
  try {
    const result = await pyodide.runPythonAsync(script);
    return Response.json({ ok: true, result: String(result) });
  } catch (e) {
    const msg = String(e).split("\n").filter(l => l.trim()).pop();
    return Response.json({ ok: false, error: msg }, { status: 400 });
  }
});

console.log(JSON.stringify({ port: server.addr.port }));
```

**Sandbox 方式の比較検証結果（ローカル実測ベース）:**

| 方式 | セキュリティ | Warm 速度 | Lambda 対応 | 判定 |
|------|:---:|:---:|:---:|---|
| Node.js `vm` | 1/5 | ~0ms | NodejsFunction | **不可**: 公式に「セキュリティ機構ではない」。vm2 は廃止 + CVSS 10.0 |
| Node.js + Pyodide (アプリ層制限のみ) | 2/5 | ~1.4ms | NodejsFunction | **不採用**: Cellbreak 等の Pyodide エスケープに無防備 |
| isolated-vm (V8 isolate) | 4/5 | ~1ms | NodejsFunction (要 Docker bundling) | 堅牢だが JS のみ。データ処理の標準ライブラリ不足 |
| Deno subprocess (毎回起動) | 4/5 | ~1000ms | NodejsFunction | Pyodide WASM の毎回ロードで実用不可 |
| **Deno 常駐 IPC + Pyodide** | **4/5** | **~5ms** | **NodejsFunction** | **採用**: Deno 権限 + Pyodide import 制限の二重防御。Python 標準ライブラリ利用可 |
| Bubblewrap | - | - | 不可 | Lambda が user namespace をブロック |

#### Skill: CLI リファレンス（MCP Prompts）

MCP の `prompts` 機能またはセッション開始時のシステムプロンプトとして提供。
テナント設定の `skill_projects` に指定されたプロジェクトのメタデータを動的に含める。

```markdown
# Backlog CLI Reference

backlog CLI は gh (GitHub CLI) と同様の使い方ができる Backlog 用 CLI です。

## gh との対応表

| 操作 | gh | backlog |
|------|-----|---------|
| 課題一覧 | gh issue list | backlog issue list |
| 課題詳細 | gh issue view 123 | backlog issue view PROJ-123 |
| 課題作成 | gh issue create | backlog issue create |
| PR 一覧 | gh pr list | backlog pr list |
| PR 詳細 | gh pr view 42 | backlog pr view --project PROJ --repo repo 42 |
| API 直接 | gh api /repos/... | backlog api /api/v2/... |

## 主要コマンド

### issue
  list    [-L LIMIT] [--project KEY] [--assignee USER] [--status STATUS,...] [--json FIELDS]
  view    ISSUE-KEY [--comments] [--json FIELDS]
  create  --project KEY --type TYPE --summary "..." [--description "..."] [--assignee USER]
  edit    ISSUE-KEY [--status STATUS] [--assignee USER] [--due-date DATE]
  close   ISSUE-KEY
  comment ISSUE-KEY --body "..."

### pr
  list    --project KEY [--repo NAME] [--status STATUS] [--json FIELDS]
  view    --project KEY --repo NAME NUMBER [--json FIELDS]

### wiki
  list    [--project KEY] [--json FIELDS]
  view    WIKI-ID [--json FIELDS]

### project
  list    [--json FIELDS]
  view    KEY [--json FIELDS]

### notification
  list    [-L LIMIT] [--json FIELDS]

### api
  PATH [-X METHOD] [-f key=value]  # raw API アクセス

## 出力形式

--json FIELDS     指定フィールドのみ JSON 出力 (例: --json issueKey,summary,status)
--jq EXPR         jq フィルタ適用 (例: --jq '.[].issueKey')
--format TMPL     Go テンプレート (例: --format '{{.issueKey}}: {{.summary}}')
-L N              取得件数 (0=全件)

## プロジェクト固有情報 (テナント設定から動的生成)

### MYPROJ のメタデータ
課題タイプ: バグ(id:1), タスク(id:2), 要望(id:3), その他(id:4)
ステータス: Open(id:1), InProgress(id:2), Resolved(id:3), Closed(id:4)
カテゴリ: Backend(id:10), Frontend(id:11), Infra(id:12)
```

### 4.3 比較

| | 公式 MCP (60+ ツール) | AWS MCP (3 メタツール) | 本プラン (2 ツール + Skill) |
|---|---|---|---|
| ツール定義のトークン消費 | 大（60+ 定義） | 小（3 定義） | **極小**（2 定義） |
| API 仕様の提供 | なし（学習データ依存） | 毎回ツール呼出 | **Skill 事前注入（0 RTT）** |
| モデル既存知識の活用 | なし | なし | **gh 知識を直接転用** |
| 複数 API チェーン | AI が逐次呼出（遅い） | run_script で 1 往復 | **run_script で 1 往復** |
| 結果サイズ制御 | トークン制限で切り詰め | フィルタ | **--json FIELDS で必要分のみ** |
| 新 API / コマンド対応 | MCP サーバー更新必要 | SDK 更新必要 | **CLI 更新で自動対応** |
| 実装コスト | 60+ ツールハンドラ | API クライアント実装 | **CLI バイナリ同梱のみ** |

### 4.4 CLI バイナリの同梱

MCP サーバーに backlog CLI の Go バイナリを同梱する。

**Lambda デプロイ (NodejsFunction):**

`commandHooks` で esbuild バンドル後に Go バイナリをコピーする。
CDK スタック定義はセクション 2.2 を参照。

```
Lambda デプロイパッケージ:
├── index.js              # esbuild でバンドルされた MCP サーバー
├── node_modules/         # Node.js 依存 (native addon なし)
├── sandbox-worker.mjs    # Deno 側 Pyodide sandbox worker
└── bin/
    ├── backlog           # Go CLI バイナリ (linux/arm64)
    └── deno              # Deno バイナリ (linux/arm64, ~40MB compressed)
```

**Docker デプロイ（コンテナ環境用）:**

```dockerfile
FROM golang:1.23 AS go-builder
WORKDIR /build
COPY packages/backlog/ .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -o backlog ./cmd/backlog

FROM node:22-slim AS node-builder
WORKDIR /app
COPY packages/mcp-server/ .
RUN npm ci && npm run build

FROM denoland/deno:bin AS deno-bin

# Deno cache: ビルド時に npm:pyodide を解決（Lambda ColdStart でダウンロードしない）
FROM denoland/deno:latest AS deno-cache
WORKDIR /app
COPY packages/mcp-server/src/sandbox-worker.mjs .
RUN deno cache sandbox-worker.mjs

FROM node:22-slim
COPY --from=public.ecr.aws/awsguru/aws-lambda-web-adapter:0.9.1 /lambda-adapter /opt/extensions/lambda-adapter

WORKDIR /app
COPY --from=go-builder /build/backlog /app/bin/backlog
COPY --from=deno-bin /deno /app/bin/deno
COPY --from=node-builder /app/dist /app/dist
COPY --from=node-builder /app/node_modules /app/node_modules
COPY --from=node-builder /app/src/sandbox-worker.mjs /app/sandbox-worker.mjs
COPY --from=deno-cache /root/.cache/deno /app/.deno-cache

ENV PORT=8080
ENV DENO_DIR=/app/.deno-cache
EXPOSE 8080
CMD ["node", "dist/index.js"]
```

native addon 不要。同梱するバイナリは Go CLI + Deno の2つ。
**Pyodide WASM はビルド時に解決**し、デプロイパッケージに静的に含める（Lambda ColdStart のたびにダウンロードしない）。
`deno cache sandbox-worker.mjs` でビルド時に npm:pyodide を Deno キャッシュに展開し、そのキャッシュディレクトリごと同梱する。

### 4.5 CLI への必要な改修: env var トークン注入

現状 CLI は credentials.yaml / keyring からのみ認証情報を取得する。
MCP サーバーからトークンを注入するため、環境変数サポートを追加する。

**変更箇所**: `packages/backlog/internal/config/config.go` の `Credential` 構造体

```go
// Before
type Credential struct {
    AccessToken  string `yaml:"access_token,omitempty" jubako:"sensitive" storage:"keyring"`
    RefreshToken string `yaml:"refresh_token,omitempty" jubako:"sensitive" storage:"keyring"`
    APIKey       string `yaml:"api_key,omitempty" jubako:"sensitive" storage:"keyring"`
}

// After: env var タグを追加
type Credential struct {
    AccessToken  string `yaml:"access_token,omitempty" jubako:"sensitive,env:ACCESS_TOKEN" storage:"keyring"`
    RefreshToken string `yaml:"refresh_token,omitempty" jubako:"sensitive,env:REFRESH_TOKEN" storage:"keyring"`
    APIKey       string `yaml:"api_key,omitempty" jubako:"sensitive,env:API_KEY" storage:"keyring"`
}
```

加えて `expandEnvShortcuts()` にショートカットを追加:
- `BACKLOG_ACCESS_TOKEN` → アクティブプロファイルの access_token として解決

jubako の環境変数レイヤー（Layer 5）が既に存在するため、タグ追加で自動統合される。

### 4.6 アクセス制御（テナント設定）

CLI コマンドパターンで制御する。

```typescript
interface McpConfig {
  enabled: boolean;
  token_key: string;
  token_key_prev?: string;
  tenants: {
    [domain: string]: {
      cli_access: {
        allow: string[];     // 許可コマンドパターン (glob)
        deny?: string[];     // 拒否パターン (allow より優先)
      };
      script?: {
        enabled: boolean;
        max_cli_calls: number;
        timeout_ms: number;
        memory_limit_mb: number;  // Deno プロセスの V8 ヒープ上限 (デフォルト: 128)
      };
      skill_projects?: string[];   // Skill に含めるプロジェクトキー
    };
  };
}
```

**設定例: 読み取り専用**
```json
{
  "mycompany.backlog.jp": {
    "cli_access": {
      "allow": ["issue list *", "issue view *", "pr list *", "pr view *",
                "wiki list *", "wiki view *", "project list *", "project view *",
                "notification list *", "api /api/v2/* -X GET"],
      "deny": ["issue create *", "issue edit *", "issue close *",
               "* --method POST", "* --method PATCH", "* --method DELETE"]
    },
    "script": { "enabled": true, "max_cli_calls": 30, "timeout_ms": 30000 },
    "skill_projects": ["PROJ-A", "PROJ-B"]
  }
}
```

**設定例: フル機能**
```json
{
  "mycompany.backlog.jp": {
    "cli_access": {
      "allow": ["*"],
      "deny": ["config *", "auth *"]
    },
    "script": { "enabled": true, "max_cli_calls": 50, "timeout_ms": 30000 }
  }
}
```

---

## 5. MCP Streamable HTTP トランスポート

### 5.1 エンドポイント

```
POST /mcp       ← MCP JSON-RPC メッセージ（tool call 等）
GET  /mcp       ← SSE ストリーム（サーバー→クライアント通知）
DELETE /mcp     ← セッション終了
```

### 5.2 セッション管理

MCP Streamable HTTP はセッション ID (`Mcp-Session-Id`) を使うが、本設計ではサーバー側にセッション状態を持たない。

方針: セッション ID は発行するが、サーバーはそれを検証しない（またはセッション ID 自体を JWE にして最小限のメタデータを内包）。MCP ツール呼び出しは個々に完結するため、セッション状態は不要。

---

## 6. 新規 MCP エンドポイント一覧

| メソッド | パス | 説明 | 認証 |
|---------|------|------|------|
| GET | `/.well-known/oauth-protected-resource` | PRM | なし |
| GET | `/.well-known/oauth-authorization-server` | AS メタデータ | なし |
| POST | `/mcp/register` | DCR | なし |
| GET | `/mcp/authorize` | 認可開始 | なし（ブラウザ） |
| GET | `/mcp/authorize/callback` | Backlog からのコールバック | なし |
| POST | `/mcp/token` | トークン交換/リフレッシュ | なし |
| POST | `/mcp` | MCP JSON-RPC | Bearer JWE |
| GET | `/mcp` | MCP SSE | Bearer JWE |
| DELETE | `/mcp` | セッション終了 | Bearer JWE |

既存の `/auth/*` エンドポイント（CLI 用）はそのまま残す。

---

## 7. セキュリティ考慮事項

### 7.1 JWE トークンのセキュリティ

| 脅威 | 対策 |
|------|------|
| トークン窃取 | JWE 暗号化により中身は読めない。HTTPS 必須。 |
| トークンリプレイ | `exp` クレームで有効期限を強制 |
| 鍵漏洩 | 鍵ローテーション対応（`kid`）。旧鍵は復号のみ許可。 |
| 不正な redirect_uri | DCR で登録された redirect_uri を client_id JWE に内包し、認可時に検証 |

### 7.2 既存セキュリティモデルとの整合

| 既存の仕組み | MCP での扱い |
|------------|-------------|
| Client Secret の保護 | 変更なし（サーバー環境変数） |
| state パラメータ | MCP OAuth は PKCE を使用（state は Claude が管理） |
| Rate Limiting | 既存ミドルウェアを MCP エンドポイントにも適用 |
| Audit Logging | MCP ツール呼び出しも監査ログに記録 |

### 7.3 `run_script` (Deno 常駐 + Pyodide) のセキュリティ

**二重防御アーキテクチャ:**
Pyodide 単体ではサンドボックスエスケープの前例あり（Grist Cellbreak 脆弱性 — Cyera Research）。
LangChain Sandbox (Deno + Pyodide) は方式自体の問題ではなく LangChain の方針転換でアーカイブ。
Cohere Terrarium は JS プロトタイプチェーン走査による CVSS 9.3 脆弱性でアーカイブ。
→ **Deno 権限システム（OS 層）+ Pyodide import 制限（アプリ層）の二重防御**を採用。

| 脅威 | 防御層 | 対策 |
|------|--------|------|
| コード実行の隔離 | Deno 権限 + WASM | Deno プロセスが `--deny-run` で外部コマンド実行を OS レベルで遮断。Pyodide は WASM 内で実行 |
| ネットワークアクセス | Deno 権限 + import 制限 | `--deny-net`(loopback 除く) + Python `socket`/`http`/`urllib` の import ブロック |
| ファイルシステム書き込み | Deno 権限 + builtins | `--deny-write` + `open()` を例外関数に差し替え + `os`/`pathlib`/`shutil` import ブロック |
| 危険モジュール import | import 制限 | `sys.meta_path` に `_SandboxFinder` (PEP 451 `find_spec`) を挿入。ブロックリスト（os, subprocess, socket 等 ~30 モジュール）は `ImportError` |
| `sys.modules` キャッシュ経由 | import 制限 | ブロック対象モジュールを sandbox セットアップ時に `sys.modules` から削除 |
| `__import__` / `importlib` 経由 | import 制限 | `find_spec` が全経路をインターセプト（`exec("import os")` / `eval("__import__('os')")` 含む） |
| CLI 乱用 | Node.js 側 | `backlog()` callback は Node.js 側で処理。呼出回数カウント（テナント設定 `max_cli_calls`） |
| CLI アクセス制御バイパス | Node.js 側 | `backlog()` は IPC 経由で Node.js に委譲。Node.js 側で `cli_access` パターンを強制。Deno プロセスから直接 CLI を呼ぶ手段なし (`--deny-run`) |
| 無限ループ / CPU 浪費 | Node.js 側 | IPC リクエストに `AbortSignal.timeout()` を設定 |
| メモリ浪費 | Deno 側 | `--v8-flags=--max-heap-size=128` で Deno プロセスのヒープ上限を設定 |
| Pyodide サンドボックスエスケープ | Deno 権限 | 仮に Pyodide の WASM 境界を突破しても、Deno の `--deny-*` 権限が最終防衛線 |

### 7.4 CLI サブプロセスのセキュリティ

| 脅威 | 対策 |
|------|------|
| コマンドインジェクション | `execFile` で引数を配列渡し（シェル経由しない） |
| 設定ファイル干渉 | 一時ディレクトリを `HOME` に設定し、既存設定を隔離 |
| 危険なコマンド | `cli_access.deny` パターンで `auth *`, `config *` 等をブロック |

### 7.5 組織管理上の注意

- デフォルト設定は読み取り専用を推奨
- `run_script` は組織が明示的に有効化する（`script.enabled: false` がデフォルト）
- 監査ログで全ツール呼び出しを記録

---

## 8. 実装フェーズ

### Phase 0: CLI 改修 + JWE 基盤 ✅

- [x] CLI: `credentialFromEnv()` で `BACKLOG_ACCESS_TOKEN` / `BACKLOG_API_KEY` サポート
- [x] CLI: `GOOS=linux GOARCH=arm64` クロスコンパイル確認
- [x] JWE 暗号化/復号ユーティリティ (`dir` + `A256GCM`)
- [x] テスト: CLI env var 認証テスト 4 件 + JWE テスト 9 件

### Phase 1: MCP OAuth AS ✅

- [x] MCP OAuth AS エンドポイント（ステートレス DCR + JWE トークン）
  - `/.well-known/oauth-protected-resource`
  - `/.well-known/oauth-authorization-server`
  - `POST /mcp/register`
  - `GET /mcp/authorize` + `/mcp/authorize/callback`
  - `POST /mcp/token`
- [x] テスト: OAuth フロー 16 件（DCR, authorize, token 交換, 不正入力拒否）

### Phase 2: `backlog` ツール + Skill ✅

- [x] Streamable HTTP トランスポート (POST/GET/DELETE `/mcp`)
- [x] `backlog` ツール実装（CLI バイナリ実行 + アクセス制御）
- [x] Skill（CLI リファレンス）の MCP prompts 実装
- [x] テナント設定によるコマンドパターン制御 (allow/deny)
- [x] JWE 認証ミドルウェア
- [x] テスト: トランスポート 11 件 + アクセス制御 5 件 + parseArgs 6 件

### Phase 3: `run_script` (Deno 常駐 + Pyodide Sandbox) ✅

- [x] `sandbox-worker.mjs` 実装 (Deno 側)
  - [x] Pyodide ロード + `_SandboxFinder` + builtins 制限セットアップ
  - [x] `_backlog_bridge` の `registerJsModule`（Node.js へ IPC callback）
  - [x] HTTP サーバー (127.0.0.1, ランダムポート)
- [x] `sandbox-client.ts` 実装 (Node.js 側)
  - [x] Deno プロセス起動・管理（ポート取得、ヘルスチェック、再起動）
  - [x] `backlog()` callback endpoint（アクセス制御 + CLI 実行 + 呼出回数制限）
  - [x] IPC タイムアウト (`AbortSignal.timeout`)
- [ ] NodejsFunction + `commandHooks` での Lambda デプロイ確認（Deno + Go バイナリ同梱）
- [ ] テスト: sandbox セキュリティ E2E（Deno 実行環境が必要）

### Phase 4: 運用 + 拡張 ✅

- [x] サーバーエントリーポイント (`serve.ts` + `@hono/node-server`)
- [x] Dockerfile (マルチステージ + Lambda Web Adapter)
- [ ] CDK スタック (`mcp-aws/`)
- [ ] 組織向け管理者ドキュメント

---

## 9. 設定例

### 9.1 relay config への追加

```typescript
const config: RelayConfig = {
  // ... 既存設定 ...
  mcp: {
    enabled: true,
    token_key: "base64url-encoded-32-byte-key",
    tenants: {
      "mycompany.backlog.jp": {
        cli_access: {
          allow: ["issue list *", "issue view *", "project list *", "project view *",
                  "wiki list *", "wiki view *", "notification list *",
                  "api /api/v2/* -X GET"],
          deny: [],
        },
        script: {
          enabled: false,
          max_cli_calls: 20,
          timeout_ms: 30000,
        },
        skill_projects: ["PROJ-A", "PROJ-B"],
      },
    },
  },
};
```

### 9.2 ユーザー側の設定（Claude Desktop）

```json
{
  "mcpServers": {
    "backlog": {
      "url": "https://relay.example.com/mcp"
    }
  }
}
```

これだけ。初回アクセス時にブラウザが開き、Backlog にログインすれば完了。

### 9.3 ツール呼び出し例（Claude との対話イメージ）

```
ユーザー: 「PROJECT-A で期限切れの課題を教えて」

Claude の内部動作 (Skill により gh パターンで推論):
  1. backlog({ args: "issue list --project PROJECT-A --due-date-until 2026-06-03
               --status Open,InProgress --json issueKey,summary,assignee,dueDate,status" })
     → コンパクトな JSON 結果

Claude: 「PROJECT-A で期限切れの課題は 3 件です:
  - PROJECT-A-42: ○○の修正 (担当: 田中, 期限: 5/28)
  - ...」
```

```
ユーザー: 「各プロジェクトの未完了課題数を集計して」

Claude の内部動作 (run_script で 1 往復):
  1. run_script({
       script: `
import json

projects = backlog("project list --json projectKey,name,id")
result = []
for p in projects:
    count = backlog(f"api /api/v2/issues/count -f projectId[]={p['id']}"
        " -f statusId[]=1 -f statusId[]=2 -f statusId[]=3")
    result.append({"project": p["name"], "open": count["count"]})
result.sort(key=lambda x: x["open"], reverse=True)
json.dumps(result, ensure_ascii=False, indent=2)
       `
     })
     → 1 往復で全プロジェクトの集計結果

Claude: 「未完了課題数の多い順:
  1. プロジェクトA: 42 件
  2. プロジェクトB: 15 件
  - ...」
```

---

## 10. 未決事項

- [ ] `jose` ライブラリの採用可否（relay-core の既存依存を確認）
- [ ] JWE 暗号鍵のローテーション戦略の詳細
- [ ] Backlog OAuth のスコープ制限と MCP ツールのマッピング
- [ ] MCP Streamable HTTP のセッション ID の扱い（完全無視 vs 軽量 JWE）
- [ ] claude.ai の Remote MCP 対応状況の最新確認（DCR 実装の詳細）
- [ ] 既存 well-known エンドポイント (`/.well-known/backlog-oauth-relay`) との共存
- [x] ~~Sandbox 実装方式~~ → **Deno 常駐 + Pyodide** を採用。Node.js `vm` は不可（公式に非推奨）、isolated-vm は堅牢だが JS のみで標準ライブラリ不足、Pyodide 単体は Cellbreak 等のエスケープリスク、Deno サブプロセス毎回起動は WASM ロードで ~1s → Deno 常駐 IPC + Pyodide の二重防御で解決
- [x] ~~Lambda Web Adapter の採用可否~~ → Docker デプロイ時は採用。Lambda 推奨は NodejsFunction
- [x] ~~スクリプト言語~~ → Python (Pyodide)。json, datetime, re, collections, statistics 等の標準ライブラリが利用可能。LLM のデータ処理コード生成に最適
- [ ] Deno バイナリの Lambda ARM64 向けバンドル方法（`deno compile` でシングルバイナリ化 or `denoland/deno:bin` イメージから取得）
- [ ] `backlog()` IPC callback の設計（Deno → Node.js への HTTP callback で CLI 実行を委譲。認証トークンは Node.js 側のみ保持）
- [ ] Pyodide WASM のビルド時バンドル検証（`deno cache` → `.deno-cache` ディレクトリごとデプロイパッケージに含める。Lambda ColdStart で初回ダウンロード不可）
- [ ] デプロイパッケージサイズの見積もり（Go CLI ~15MB + Deno ~40MB + Pyodide WASM ~25MB + Node.js コード → Lambda 250MB 制限内に収まるか）
- [ ] Skill のメタデータ動的生成（プロジェクトのメタデータを定期取得するか、リクエスト時に取得するか）
- [ ] Skills の拡張形態（MCP `prompts` vs ツール説明への埋め込み vs 別途参照）
