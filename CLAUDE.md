# CLAUDE.md - Backlog CLI Project

## プロジェクト概要

Backlog用のコマンドラインインターフェース「backlog」を開発するプロジェクトです。
GitHub CLI (gh) と同様のユーザー体験を目指します。

## 技術スタック

- **言語**: Go 1.23+
- **ツール管理**: mise
- **ビルド**: GNU Make

### 外部ライブラリ

| 用途          | ライブラリ                              |
|-------------|------------------------------------|
| CLI フレームワーク | `github.com/spf13/cobra`           |
| YAML パース    | `gopkg.in/yaml.v3`                 |
| 対話的 UI      | `github.com/AlecAivazis/survey/v2` |
| JWT         | `github.com/golang-jwt/jwt/v5`     |
| ブラウザ起動      | `github.com/pkg/browser`           |

## ディレクトリ構成

```
backlog-cli/
├── cmd/backlog/          # エントリーポイント
├── internal/
│   ├── cmd/              # コマンド定義 (cobra)
│   ├── api/              # Backlog API クライアント
│   ├── config/           # 設定管理
│   ├── auth/             # OAuth認証 (CLI側)
│   ├── relay/            # 中継サーバー
│   └── ui/               # 対話的UI
├── docs/plans/           # 実装プランファイル
├── .mise.toml
├── Makefile
└── go.mod
```

## 実装プラン

`docs/plans/` ディレクトリに番号付きのプランファイルがあります。
順番に読んで実装を進めてください。

1. `00-overview.md` - 全体概要
2. `01-foundation.md` - 基盤構築
3. `02-config.md` - 設定管理
4. `03-relay-server.md` - 中継サーバー基本
5. `04-cli-auth.md` - CLI認証
6. `05-relay-advanced.md` - 中継サーバー拡張
7. `06-api-client.md` - APIクライアント
7. `07-issue-commands.md` - issueコマンド
8. `08-project-commands.md` - projectコマンド
9. `09-additional-commands.md` - 追加コマンド
10. `10-improvements.md` - 改善

## 開発コマンド

```bash
# ビルド（アーティファクト作成時）
make build

# テスト
make test

# リント
make lint

# 開発用実行（go run使用）
make run ARGS="<コマンド引数>"

# 中継サーバー起動（開発用、go run使用）
make serve

# クリーン
make clean
```

## 設計ドキュメント

- `docs/oauth-relay-server-design.md` - OAuth中継サーバー設計書

## 重要な設計判断

### 1. 設定の優先順位

```
コマンド引数 > 環境変数 > .backlog.yaml > ~/.config/backlog/config.yaml > デフォルト
```

### 2. OAuth認証フロー

CLIにClient Secretを持たせず、中継サーバー経由でトークンを取得します。
詳細は `docs/plans/03-relay-server.md` と `docs/plans/04-cli-auth.md` を参照。

### 3. 複数ドメイン対応

backlog.jp と backlog.com の両方に対応。中継サーバーで複数の Client ID/Secret を管理。

### 4. プロジェクトローカル設定

`.backlog.yaml` をリポジトリルートに配置することで、Git リポジトリと Backlog プロジェクトを紐付け。

## コーディング規約

- Go の標準的なスタイルに従う
- エラーは適切にラップして返す (`fmt.Errorf("context: %w", err)`)
- パッケージ間の依存は internal/ 内で完結させる
- テストは `*_test.go` に記述
- **internal/ 配下は破壊的変更可**: プロジェクト内でコンパイルが通れば後方互換性の維持は不要

## 注意事項

- 外部ライブラリは選定済みのもののみ使用
- 標準ライブラリで実現可能なものは標準ライブラリを優先
- セキュリティに関わる部分（トークン保存、Cookie署名等）は特に注意
