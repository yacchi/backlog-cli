# 11. SPA フロントエンド

## 概要

ログイン画面を含むブラウザベースUIをSPA（Single Page Application）化し、将来的な機能拡張に備える。

### 背景

- ログイン画面のUIが複雑化しており、Goテンプレート内でのHTML/CSS/JS管理が困難になってきた
- 将来的に課題一覧/詳細のWebビューなど、ブラウザベースの機能を追加する可能性がある
- Reactを使用することで、プロジェクトで培ったスキルセットを活用できる

### 目標

- モダンなフロントエンド開発体験（HMR、TypeScript、コンポーネント指向）
- シングルバイナリ配布の維持（`go:embed`）
- Goのビルド速度に影響を与えない高速なフロントエンドビルド

---

## 技術選定

| 項目         | 選定               | 理由                                               |
|------------|------------------|--------------------------------------------------|
| ビルドツール     | **Vite**         | 2025年のデファクトスタンダード。開発時はesbuildで爆速起動、本番はRollupで最適化 |
| UIフレームワーク  | **React**        | ユーザーのスキルセット、エコシステムの成熟度                           |
| 言語         | **TypeScript**   | 型安全性によるバグ防止                                      |
| スタイリング     | **Tailwind CSS** | ユーティリティファーストでコンポーネント指向と相性が良い                     |
| パッケージマネージャ | **pnpm**         | 高速、ディスク効率が良い                                     |

### 不採用とした選択肢

| 選択肢        | 不採用理由                      |
|------------|----------------------------|
| esbuild単体  | HMR等の開発機能を自前実装する必要がある      |
| Bun        | エコシステムやエッジケースでの安定性がViteに劣る |
| Vue/Svelte | プロジェクトでの使用実績がReactより少ない    |

---

## アーキテクチャ

### 開発時

```
┌──────────────────────────────────────────────────────────────┐
│                                                              │
│  ブラウザ ──→ Vite Dev Server (localhost:5173)              │
│                    │                                         │
│                    │ /auth/*, /callback をプロキシ           │
│                    ↓                                         │
│              Go Server (localhost:52847)                     │
│                                                              │
│  メリット:                                                   │
│  - HMR（ホットモジュールリプレースメント）有効               │
│  - Go側のCORS設定不要                                        │
│  - フロントエンドとバックエンドを独立して開発可能            │
│                                                              │
└──────────────────────────────────────────────────────────────┘
```

### 本番時

```
┌──────────────────────────────────────────────────────────────┐
│                                                              │
│  ブラウザ ──→ Go Binary (localhost:52847)                   │
│                    │                                         │
│                    ├─ /auth/ws, /auth/configure → API        │
│                    ├─ /callback → Callback Handler           │
│                    └─ /* → embed.FS (SPA静的ファイル)        │
│                                                              │
│  メリット:                                                   │
│  - シングルバイナリ配布                                      │
│  - 外部依存なし                                              │
│                                                              │
└──────────────────────────────────────────────────────────────┘
```

---

## ディレクトリ構成

```
backlog-cli/
├── cmd/backlog/
├── internal/
│   ├── auth/
│   │   ├── callback.go          # 変更: SPA配信統合
│   │   ├── callback_test.go
│   │   ├── client.go
│   │   └── setup_page.go        # 削除予定（移行完了後）
│   ├── ui/                      # 新規: 静的ファイル配信
│   │   ├── embed.go             # go:embed定義
│   │   ├── embed_dev.go         # 開発用（ビルドタグ）
│   │   └── handler.go           # SPAハンドラー
│   └── ...
├── web/                         # 新規: フロントエンドプロジェクト
│   ├── src/
│   │   ├── main.tsx             # エントリーポイント
│   │   ├── App.tsx              # ルートコンポーネント
│   │   ├── pages/
│   │   │   ├── LoginSetup.tsx   # 設定入力画面
│   │   │   └── LoginConfirm.tsx # ログイン確認画面
│   │   ├── components/
│   │   │   ├── Button.tsx
│   │   │   ├── Input.tsx
│   │   │   ├── InfoBox.tsx
│   │   │   └── StatusIndicator.tsx
│   │   ├── hooks/
│   │   │   └── useWebSocket.ts  # WebSocket接続フック
│   │   └── styles/
│   │       └── index.css        # Tailwind読み込み
│   ├── public/                  # 静的アセット
│   ├── dist/                    # ビルド成果物（.gitignore）
│   ├── index.html
│   ├── vite.config.ts
│   ├── tailwind.config.js
│   ├── postcss.config.js
│   ├── tsconfig.json
│   └── package.json
├── Makefile
└── go.mod
```

---

## 実装手順

### Phase 1: フロントエンド基盤構築

#### 1.1 Vite + React プロジェクト初期化

```bash
mkdir web && cd web
pnpm create vite . --template react-ts
pnpm install
pnpm install -D tailwindcss postcss autoprefixer
npx tailwindcss init -p
```

#### 1.2 vite.config.ts

```typescript
import {defineConfig} from 'vite'
import react from '@vitejs/plugin-react'

export default defineConfig({
    plugins: [react()],
    server: {
        port: 5173,
        proxy: {
            // 認証関連エンドポイントをGoサーバーへプロキシ
            '/auth/ws': {
                target: 'http://localhost:52847',
                changeOrigin: true,
                ws: true,  // WebSocket対応
            },
            '/auth/configure': {
                target: 'http://localhost:52847',
                changeOrigin: true,
            },
            '/auth/popup': {
                target: 'http://localhost:52847',
                changeOrigin: true,
            },
            '/callback': {
                target: 'http://localhost:52847',
                changeOrigin: true,
            },
        },
    },
    build: {
        outDir: 'dist',
        emptyOutDir: true,
        // ソースマップは開発時のみ
        sourcemap: false,
    },
})
```

#### 1.3 tailwind.config.js

```javascript
/** @type {import('tailwindcss').Config} */
export default {
    content: [
        "./index.html",
        "./src/**/*.{js,ts,jsx,tsx}",
    ],
    theme: {
        extend: {
            colors: {
                primary: {
                    DEFAULT: '#667eea',
                    dark: '#764ba2',
                },
            },
        },
    },
    plugins: [],
}
```

#### 1.4 ルーティング設定

React Router を使用してSPAルーティングを実装:

```typescript
// src/App.tsx
import {BrowserRouter, Routes, Route, Navigate} from 'react-router-dom'
import LoginSetup from './pages/LoginSetup'
import LoginConfirm from './pages/LoginConfirm'

function App() {
    return (
        <BrowserRouter>
            <Routes>
                <Route path = "/auth/setup"
    element = { < LoginSetup / >
}
    />
    < Route
    path = "/auth/start"
    element = { < LoginConfirm / >
}
    />
    < Route
    path = "*"
    element = { < Navigate
    to = "/auth/start"
    replace / >
}
    />
    < /Routes>
    < /BrowserRouter>
)
}

export default App
```

### Phase 2: 既存UIのReact移植

#### 2.1 コンポーネント設計

既存の `setup_page.go` から以下のコンポーネントを抽出:

| コンポーネント           | 責務                |
|-------------------|-------------------|
| `Container`       | 中央配置のカードレイアウト     |
| `Button`          | プライマリ/セカンダリボタン    |
| `Input`           | テキスト/URL入力フィールド   |
| `InfoBox`         | 情報表示ボックス          |
| `WarningBox`      | セキュリティ警告ボックス      |
| `StatusIndicator` | スピナー付きステータス表示     |
| `ResultView`      | 成功/エラー/サーバークローズ表示 |

#### 2.2 WebSocketフック

```typescript
// src/hooks/useWebSocket.ts
import {useEffect, useRef, useState, useCallback} from 'react'

type Status = 'connecting' | 'connected' | 'success' | 'error' | 'closed'

interface UseWebSocketResult {
    status: Status
    error: string | null
    isActive: boolean
    disconnect: () => void
}

export function useWebSocket(): UseWebSocketResult {
    const [status, setStatus] = useState<Status>('connecting')
    const [error, setError] = useState<string | null>(null)
    const wsRef = useRef<WebSocket | null>(null)
    const activeRef = useRef(true)

    useEffect(() => {
        const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:'
        const wsUrl = `${protocol}//${window.location.host}/auth/ws`

        try {
            wsRef.current = new WebSocket(wsUrl)
        } catch (err) {
            setStatus('closed')
            return
        }

        wsRef.current.onopen = () => {
            setStatus('connected')
        }

        wsRef.current.onmessage = (event) => {
            if (!activeRef.current) return

            try {
                const data = JSON.parse(event.data)
                if (data.status === 'success') {
                    activeRef.current = false
                    setStatus('success')
                } else if (data.status === 'error') {
                    activeRef.current = false
                    setStatus('error')
                    setError(data.error || '認証に失敗しました')
                }
            } catch (err) {
                console.error('WebSocket message parse error:', err)
            }
        }

        wsRef.current.onclose = () => {
            if (activeRef.current) {
                activeRef.current = false
                setStatus('closed')
            }
        }

        return () => {
            activeRef.current = false
            wsRef.current?.close()
        }
    }, [])

    const disconnect = useCallback(() => {
        activeRef.current = false
        wsRef.current?.close()
    }, [])

    return {
        status,
        error,
        isActive: activeRef.current,
        disconnect,
    }
}
```

#### 2.3 ページコンポーネント例

```typescript
// src/pages/LoginConfirm.tsx
import {useState} from 'react'
import {useWebSocket} from '../hooks/useWebSocket'
import Container from '../components/Container'
import Button from '../components/Button'
import InfoBox from '../components/InfoBox'
import StatusIndicator from '../components/StatusIndicator'
import ResultView from '../components/ResultView'

interface Props {
    space: string
    domain: string
    relayServer: string
}

export default function LoginConfirm({space, domain, relayServer}: Props) {
    const {status, error, disconnect} = useWebSocket()
    const [isLoggingIn, setIsLoggingIn] = useState(false)

    const handleLogin = () => {
        setIsLoggingIn(true)
        // ポップアップを開く
        const width = 600
        const height = 700
        const left = window.screenX + (window.outerWidth - width) / 2
        const top = window.screenY + (window.outerHeight - height) / 2
        window.open(
            '/auth/popup',
            'backlog_auth',
            `width=${width},height=${height},left=${left},top=${top}`
        )
    }

    if (status === 'success') {
        return <ResultView type = "success" / >
    }

    if (status === 'error') {
        return <ResultView type = "error"
        message = {error}
        />
    }

    if (status === 'closed') {
        return <ResultView type = "closed" / >
    }

    return (
        <Container>
            <h1 className = "text-2xl font-semibold text-gray-800 mb-6" >
            Backlog
    CLI
    ログイン
    < /h1>

    < div
    className = "bg-blue-50 border-l-4 border-primary p-4 mb-6 text-left text-sm text-gray-600" >
        Backlog
    CLI
    がターミナルからの操作で
    Backlog
    API
    にアクセスするための認証を行います。
      </div>

      < InfoBox
    label = "スペース"
    value = {`${space}.${domain}`
}
    />
    < InfoBox
    label = "リレーサーバー"
    value = {relayServer}
    />

    < div
    className = "flex gap-4 justify-center mt-6" >
    <Button
        variant = "secondary"
    onClick = {()
=>
    {
        disconnect()
        window.location.href = '/auth/setup'
    }
}
>
    設定を変更
    < /Button>
    < Button
    onClick = {handleLogin}
    disabled = {isLoggingIn} >
        ログインする
        < /Button>
        < /div>

    {
        isLoggingIn && (
            <StatusIndicator message = "ポップアップで認証を進めてください..." / >
        )
    }
    </Container>
)
}
```

### Phase 3: Go側の静的ファイル配信

#### 3.1 internal/ui/embed.go

```go
//go:build !dev

package ui

import (
	"embed"
	"io/fs"
)

//go:embed all:dist
var distFS embed.FS

// Assets は本番用の静的ファイルシステムを返す
func Assets() (fs.FS, error) {
	return fs.Sub(distFS, "dist")
}
```

#### 3.2 internal/ui/embed_dev.go

```go
//go:build dev

package ui

import (
	"io/fs"
	"os"
)

// Assets は開発用のファイルシステムを返す（実際のファイルを参照）
func Assets() (fs.FS, error) {
	return os.DirFS("web/dist"), nil
}
```

#### 3.3 internal/ui/handler.go

```go
package ui

import (
	"io/fs"
	"net/http"
	"strings"
)

// SPAHandler はSPAのためのハンドラを返す
// 静的ファイルが存在すればそれを配信し、存在しなければindex.htmlを返す（History API Fallback）
func SPAHandler(assets fs.FS) http.Handler {
	fileServer := http.FileServer(http.FS(assets))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/")

		// ファイルが存在するか確認
		f, err := assets.Open(path)
		if err == nil {
			defer f.Close()
			stat, _ := f.Stat()
			if !stat.IsDir() {
				// ファイルが存在すればそのまま配信
				fileServer.ServeHTTP(w, r)
				return
			}
		}

		// ファイルが存在しない場合は index.html を返す
		content, err := fs.ReadFile(assets, "index.html")
		if err != nil {
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write(content)
	})
}
```

#### 3.4 CallbackServerの変更

```go
// internal/auth/callback.go の変更箇所

func (cs *CallbackServer) setupRoutes() {
mux := http.NewServeMux()

// API エンドポイント（優先度高）
mux.HandleFunc("/auth/configure", cs.handleConfigure)
mux.HandleFunc("/auth/ws", cs.handleWebSocket)
mux.HandleFunc("/auth/popup", cs.handlePopup)
mux.HandleFunc("/callback", cs.handleCallback)

// SPA配信（その他全てのパス）
assets, err := ui.Assets()
if err != nil {
// フォールバック: 旧テンプレート使用
mux.HandleFunc("/", cs.handleLegacyStart)
return
}
mux.Handle("/", ui.SPAHandler(assets))

cs.handler = mux
}
```

### Phase 4: ビルドパイプライン統合

#### 4.1 Makefile追加・変更

```makefile
.PHONY: build-web dev-web

# フロントエンドビルド
build-web:
	cd web && pnpm install --frozen-lockfile && pnpm build

# フロントエンドをビルドしてからGoをビルド
build: build-web
	@mkdir -p $(BUILD_DIR)
	go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY) ./cmd/backlog

# フロントエンド開発サーバー
dev-web:
	cd web && pnpm dev

# クリーン（フロントエンドも含む）
clean:
	rm -rf $(BUILD_DIR)
	rm -f $(BINARY)
	rm -f coverage.out coverage.html
	rm -rf web/dist web/node_modules/.vite
```

#### 4.2 .gitignore追加

```gitignore
# Frontend
web/node_modules/
web/dist/
web/.vite/
```

### Phase 5: 移行と検証

#### 5.1 段階的移行

1. **並行稼働期間**: 環境変数またはフラグで旧テンプレートとSPAを切り替え可能にする
2. **検証項目**:
    - 全ブラウザでの動作確認（Chrome, Firefox, Safari, Edge）
    - WebSocket接続の安定性
    - ポップアップブロック時の挙動
    - エラーハンドリング
3. **移行完了後**: `setup_page.go` を削除

#### 5.2 テスト観点

| カテゴリ      | テスト内容                         |
|-----------|-------------------------------|
| 機能        | ログインフロー全体が正常に動作する             |
| UI        | 各画面が正しく表示される                  |
| エラー       | ポップアップブロック、ネットワークエラー時の挙動      |
| WebSocket | 接続、メッセージ受信、切断時の挙動             |
| ビルド       | `make build` でSPA含むバイナリが生成される |

---

## ビルド速度への影響

| 項目                          | 時間目安  | 備考                  |
|-----------------------------|-------|---------------------|
| `pnpm install` (初回)         | 5-10秒 | node_modulesのダウンロード |
| `pnpm install` (CI/キャッシュあり) | 1-2秒  | lockfileから復元        |
| `pnpm build` (Vite)         | 2-5秒  | esbuild + Rollup    |
| `go build`                  | 変化なし  | embed.FSの読み込みは高速    |

Viteのビルドは非常に高速（2-5秒程度）なので、Goのビルド速度に対して足を引っ張ることはありません。

---

## 将来の拡張性

SPA化により以下の機能追加が容易になります:

| 機能         | 概要                     |
|------------|------------------------|
| 課題一覧/詳細ビュー | ブラウザで課題を閲覧・編集          |
| 設定画面       | GUIでの設定管理              |
| ダッシュボード    | 統計情報の可視化               |
| オフライン対応    | Service Workerによるキャッシュ |
| PWA化       | インストール可能なWebアプリ化       |

---

## 参考資料

- [Vite公式ドキュメント](https://vitejs.dev/)
- [React公式ドキュメント](https://react.dev/)
- [Tailwind CSS](https://tailwindcss.com/)
- [Go embed パッケージ](https://pkg.go.dev/embed)
