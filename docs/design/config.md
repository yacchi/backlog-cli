# 設定（Config）

このプロジェクトの設定は `github.com/yacchi/jubako` を使って「複数レイヤーのマージ + 解決済み（resolved）構造体」へ変換して扱います。

## 設定レイヤーと優先順位

実装: `packages/backlog/internal/config/store.go`

上にあるほど優先（後勝ち）です。

1. **コマンド引数**（`LayerArgs`）: `backlog --profile/--project/--output/...`
2. **環境変数**（`LayerEnv`）: `BACKLOG_...`
3. **プロジェクト設定**（`LayerProject`）: リポジトリ内の `.backlog.yaml`
4. **クレデンシャル**（`LayerCredentials`）: `~/.config/backlog/credentials.yaml`（0600）
5. **ユーザー設定**（`LayerUser`）: `~/.config/backlog/config.yaml`
6. **デフォルト**（`LayerDefaults`）: `packages/backlog/internal/config/defaults.yaml`（`go:embed`）

## ファイル配置

- プロジェクト設定: `.backlog.yaml`（リポジトリ内、`findProjectConfigPath()` で上位ディレクトリも探索）
- ユーザー設定: `~/.config/backlog/config.yaml`
- クレデンシャル: `~/.config/backlog/credentials.yaml`（センシティブ専用）

実装: `packages/backlog/internal/config/paths.go`, `packages/backlog/internal/config/store.go`

## プロファイル

- 設定は `profile.<name>.*` として複数持てます。
- 実行時の「アクティブプロファイル」は `Store` が保持します。
  - `backlog --profile <name>` が指定されていれば最優先で切り替え（`packages/backlog/internal/cmd/root.go`）。
  - それ以外は `.backlog.yaml` / 環境変数側の `project.profile` が有効なら採用（`Store.LoadAll()`）。

## 環境変数

環境変数は `BACKLOG_` プレフィックスで取り込みます（`env.NewWithAutoSchema()`）。

### ショートカット環境変数

利便性のため、`BACKLOG_SPACE` などの省略形は `BACKLOG_PROFILE_default_SPACE` のような完全形式に展開してからマッピングします。

実装: `packages/backlog/internal/config/resolved.go`（`expandEnvShortcuts()`）

## センシティブ値（資格情報）

- `credential.*` は「センシティブ」扱いで、原則 `credentials.yaml` だけに保存します。
- マスク表示は `jubako.WithSensitiveMaskString()` で行います（`packages/backlog/internal/config/store.go`）。

`Credential` は OAuth / API Key を併用でき、後方互換性のため `auth_type` 未設定時の扱いがあります。

実装: `packages/backlog/internal/config/config.go`

## 信頼バンドル（Relay Config Bundle）

- `client.trust.bundles[]` にインポート済みのバンドルを保持します。
- 認証UIの設定保存時に、該当ドメインの信頼バンドルがあれば追加検証を行います。

実装: `packages/backlog/internal/config/trust.go`, `packages/backlog/internal/config/relay_info.go`

仕様: `docs/design/relay-config-bundle.md`

