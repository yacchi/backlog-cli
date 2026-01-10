# CLI構成（コマンドと主要パッケージ）

## エントリーポイント

- `cmd/backlog/main.go` → `packages/backlog/app.Run()` → `packages/backlog/internal/cmd.Execute()`

## コマンド体系

実装: `packages/backlog/internal/cmd/root.go`

- `backlog auth ...`
- `backlog config ...`
- `backlog issue ...`
- `backlog issue-type ...`
- `backlog markdown ...`
- `backlog pr ...`
- `backlog project ...`
- `backlog wiki ...`

## 設定の読み込み

全コマンド共通で、`PersistentPreRunE` で設定をロードしてから実行します。

- `packages/backlog/internal/config.Load()` で `jubako` ベースのストアを初期化・ロード
- `--profile/--project/--output/--format` 等のグローバルフラグは、Args レイヤーに反映して上書き

詳細は `docs/design/config.md` を参照。

## APIクライアント

- `packages/backlog/internal/api/*`: CLIのユースケース層（Backlog API呼び出し、リトライ/エラー整形など）
- `packages/backlog/internal/backlog/*`: OpenAPI から `ogen` で生成されたクライアント/型

## 出力/UI

- `packages/backlog/internal/ui/*`: テーブル描画・色・プロンプト
- `packages/backlog/internal/cmdutil/*`: 出力形式（table/json/テンプレート）、Markdown view/render など

## 認証UI（SPA）

- `packages/backlog/internal/auth/callback.go`: ローカルHTTPサーバー + Connect RPC
- `packages/web/`: SPA（`go:embed`）

詳細は `docs/design/auth-flow.md` を参照。

