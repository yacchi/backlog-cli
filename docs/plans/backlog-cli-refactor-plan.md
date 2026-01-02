# Backlog CLI リファクタリング計画

## 概要

Go版中継サーバーを廃止し、TypeScript実装に統一。プロジェクト構造を`packages/`ベースに再編成する。

## 目標

1. Go CLIコードを`packages/backlog/`に移動
2. 中継サーバーをTypeScript（Hono）で実装
3. Webアセットの埋め込みを`packages/web/`に統合（handlerは`packages/backlog/internal/ui/`に残す）
4. `deploy/`ディレクトリを廃止

## 最終ディレクトリ構造

```
backlog-cli/
├── go.mod
├── go.sum
├── cmd/backlog/main.go              # go install エントリ（薄いラッパー）
│
├── packages/
│   ├── backlog/                     # Go CLI
│   │   ├── app/
│   │   │   └── run.go               # 公開エントリポイント
│   │   └── internal/
│   │       ├── api/
│   │       ├── auth/
│   │       ├── cmd/
│   │       ├── config/
│   │       ├── domain/
│   │       ├── jwk/
│   │       └── ui/                  # handler.go など（embed.goはwebへ、web.Assets()を参照）
│   │
│   ├── web/                         # React SPA + Go埋め込み
│   │   ├── src/
│   │   ├── dist/                    # ビルド成果物
│   │   ├── embed.go                 # //go:embed all:dist（Assets()を公開）
│   │   ├── embed_dev.go
│   │   ├── package.json
│   │   └── vite.config.ts
│   │
│   ├── relay-core/                  # 中継サーバー共通ロジック（TS）
│   │   ├── src/
│   │   │   ├── handlers/
│   │   │   │   ├── auth.ts          # OAuth handlers
│   │   │   │   ├── token.ts
│   │   │   │   ├── wellknown.ts
│   │   │   │   └── portal.ts
│   │   │   ├── middleware/
│   │   │   │   ├── access-control.ts
│   │   │   │   ├── rate-limit.ts
│   │   │   │   └── audit.ts
│   │   │   ├── config/
│   │   │   │   └── types.ts
│   │   │   └── index.ts
│   │   ├── package.json
│   │   └── tsconfig.json
│   │
│   ├── relay-cloudflare/            # Cloudflare Workers
│   │   ├── src/index.ts
│   │   ├── wrangler.toml
│   │   └── package.json
│   │
│   ├── relay-aws/                   # AWS Lambda
│   │   ├── src/index.ts
│   │   ├── cdk/
│   │   │   ├── lib/relay-stack.ts
│   │   │   └── bin/app.ts
│   │   └── package.json
│   │
│   └── relay-docker/                # Docker版（ローカル開発/自前ホスト）
│       ├── src/index.ts
│       ├── Dockerfile
│       └── package.json
│
├── proto/                           # Protobuf定義
├── gen/                             # 生成コード
│
├── pnpm-workspace.yaml
├── package.json
├── tsconfig.base.json
├── Makefile
└── version.txt
```

---

## Phase 1: Go パッケージ構造の再編成

### 1.1 packages/backlog ディレクトリ作成

```bash
mkdir -p packages/backlog/app
mkdir -p packages/backlog/internal
```

### 1.2 internal/ を packages/backlog/internal/ に移動

移動対象（中継サーバー関連を除く）:
- `internal/api/` → `packages/backlog/internal/api/`
- `internal/auth/` → `packages/backlog/internal/auth/`
- `internal/cmd/` → `packages/backlog/internal/cmd/`
- `internal/config/` → `packages/backlog/internal/config/`
- `internal/domain/` → `packages/backlog/internal/domain/`
- `internal/jwk/` → `packages/backlog/internal/jwk/`
- `internal/ui/` → `packages/backlog/internal/ui/`（embed.go除く）

削除対象（Go版中継サーバー）:
- `internal/relay/`

### 1.3 公開エントリポイント作成

**packages/backlog/app/run.go**:
```go
package app

import "github.com/yacchi/backlog-cli/packages/backlog/internal/cmd"

func Run() {
    cmd.Execute()
}
```

### 1.4 cmd/backlog/main.go 更新

```go
package main

import "github.com/yacchi/backlog-cli/packages/backlog/app"

func main() {
    app.Run()
}
```

### 1.5 インポートパス更新

全Goファイルで以下を置換:
```
github.com/yacchi/backlog-cli/internal/
↓
github.com/yacchi/backlog-cli/packages/backlog/internal/
```

---

## Phase 2: Web パッケージ再編成

### 2.1 web/ を packages/web/ に移動

```bash
mv web packages/web
```

### 2.2 Go埋め込みファイルを packages/web/ に移動

移動対象（アセット埋め込みのみ）:
- `internal/ui/embed.go` → `packages/web/embed.go`
- `internal/ui/embed_dev.go` → `packages/web/embed_dev.go`

残す対象（実装詳細として隠蔽）:
- `internal/ui/handler.go` → `packages/backlog/internal/ui/handler.go`

### 2.3 packages/web/ のパッケージ名

`package web`（Assets()関数のみ公開）

```go
// packages/web/embed.go
//go:build !dev

package web

import (
    "embed"
    "io/fs"
)

//go:embed all:dist
var distFS embed.FS

// Assets returns the production static file system.
func Assets() (fs.FS, error) {
    return fs.Sub(distFS, "dist")
}
```

### 2.4 packages/web/embed_dev.go のパス調整

```go
// packages/web/embed_dev.go
//go:build dev

package web

import (
    "io/fs"
    "os"
)

// Assets returns the development static file system.
func Assets() (fs.FS, error) {
    return os.DirFS("packages/web/dist"), nil
}
```

### 2.5 packages/backlog/internal/ui/handler.go からの参照

```go
package ui

import (
    "io/fs"
    "net/http"
    "github.com/yacchi/backlog-cli/packages/web"
)

// SPAHandler serves static assets with an index.html fallback.
func SPAHandler() http.Handler {
    assets, err := web.Assets()
    if err != nil {
        return http.NotFoundHandler()
    }
    return spaHandler(assets)
}

func spaHandler(assets fs.FS) http.Handler {
    // 既存の実装...
}
```

---

## Phase 3: TypeScript 中継サーバー実装

### 3.1 pnpm workspace 設定

**pnpm-workspace.yaml**:
```yaml
packages:
  - 'packages/*'
  - '!packages/backlog'
```

### 3.2 relay-core 作成

既存の `deploy/cloudflare-workers/src/index.ts` からロジックを抽出:
- OAuth handlers
- State encoding/decoding
- Error handling
- Config types

### 3.3 relay-cloudflare 移行

`deploy/cloudflare-workers/` → `packages/relay-cloudflare/`
- relay-core を依存に追加
- Hono + Workers adapter

### 3.4 relay-aws 作成

`deploy/aws-cdk/` のCDK定義を移行:
- Lambda handler（Hono + AWS Lambda adapter）
- CDK stack

### 3.5 relay-docker 作成

新規作成:
- Node.js + Hono サーバー
- Dockerfile
- web/dist を静的配信

---

## Phase 4: Go版中継サーバー機能の移植

### 移植対象機能

| Go ファイル | 移植先 (TS) |
|------------|------------|
| handlers.go | relay-core/src/handlers/auth.ts, token.ts |
| wellknown.go | relay-core/src/handlers/wellknown.ts |
| portal.go | relay-core/src/handlers/portal.ts |
| info.go | relay-core/src/handlers/info.ts |
| certs.go | relay-core/src/handlers/certs.ts |
| bundle.go | relay-core/src/handlers/bundle.ts |
| access.go | relay-core/src/middleware/access-control.ts |
| ratelimit.go | relay-core/src/middleware/rate-limit.ts |
| audit.go | relay-core/src/middleware/audit.ts |
| bundle_auth.go | relay-core/src/middleware/bundle-auth.ts |
| encoded_state.go | relay-core/src/utils/state.ts |
| cache.go | relay-core/src/utils/cache.ts |

### プラットフォーム抽象化

```typescript
// relay-core/src/platform/types.ts
export interface ConfigProvider {
  get(key: string): Promise<string | undefined>;
}

export interface CacheProvider {
  get(key: string): Promise<string | undefined>;
  set(key: string, value: string, ttl?: number): Promise<void>;
}

export interface AuditLogger {
  log(event: AuditEvent): void;
}
```

---

## Phase 5: deploy/ ディレクトリ削除

### 削除対象

- `deploy/aws-cdk/` → `packages/relay-aws/cdk/` に移行済み
- `deploy/cloudflare-workers/` → `packages/relay-cloudflare/` に移行済み

---

## Phase 6: ビルドシステム更新

### Makefile 更新

```makefile
# Web ビルド（コピー不要）
build-web: buf-generate
	pnpm --filter web build

# Go ビルド
build: build-web
	go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY) ./cmd/backlog

# 中継サーバー（Docker）
build-relay-docker: build-web
	pnpm --filter relay-docker build
	docker build -t backlog-relay packages/relay-docker

# 中継サーバー（Cloudflare Workers）
deploy-relay-cf:
	pnpm --filter relay-cloudflare deploy

# 中継サーバー（AWS）
deploy-relay-aws:
	pnpm --filter relay-aws deploy
```

### package.json（ルート）

```json
{
  "name": "backlog-cli",
  "private": true,
  "scripts": {
    "build": "pnpm -r build",
    "test": "pnpm -r test",
    "deploy:cf": "pnpm --filter relay-cloudflare deploy",
    "deploy:aws": "pnpm --filter relay-aws deploy"
  }
}
```

---

## 実装順序

1. **Phase 1**: Go パッケージ構造の再編成
   - ディレクトリ移動
   - インポートパス更新
   - ビルド・テスト確認

2. **Phase 2**: Web パッケージ再編成
   - embed.go 移動
   - ビルド確認

3. **Phase 3**: relay-core 作成
   - 基本OAuth機能の移植
   - 既存CF Workers実装をベースに

4. **Phase 4**: 各プラットフォームアダプタ
   - relay-cloudflare
   - relay-aws
   - relay-docker

5. **Phase 5**: 追加機能移植
   - アクセス制御
   - 監査ログ
   - Bundle認証

6. **Phase 6**: 旧コード削除
   - internal/relay/
   - deploy/

---

## 変更対象ファイル一覧

### 移動

| From | To |
|------|-----|
| internal/api/ | packages/backlog/internal/api/ |
| internal/auth/ | packages/backlog/internal/auth/ |
| internal/cmd/ | packages/backlog/internal/cmd/ |
| internal/config/ | packages/backlog/internal/config/ |
| internal/domain/ | packages/backlog/internal/domain/ |
| internal/jwk/ | packages/backlog/internal/jwk/ |
| internal/ui/handler.go | packages/backlog/internal/ui/handler.go |
| internal/ui/embed.go | packages/web/embed.go |
| internal/ui/embed_dev.go | packages/web/embed_dev.go |
| internal/ui/*.go（その他） | packages/backlog/internal/ui/ |
| web/ | packages/web/ |
| deploy/cloudflare-workers/ | packages/relay-cloudflare/ |
| deploy/aws-cdk/ | packages/relay-aws/cdk/ |

### 新規作成

- packages/backlog/app/run.go
- packages/relay-core/
- packages/relay-docker/

### 削除

- internal/relay/
- deploy/

### 更新

- cmd/backlog/main.go
- go.mod（必要に応じて）
- Makefile
- package.json
- pnpm-workspace.yaml
