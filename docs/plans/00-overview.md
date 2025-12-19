# Phase 00: 全体概要

## プロジェクトの目的

Backlog用CLI「backlog」を開発し、GitHub CLI (gh) と同様のユーザー体験を提供する。

## 主要機能

1. **認証** - OAuth 2.0（中継サーバー経由）とAPI Key認証の両対応
2. **課題管理** - issue list/view/create/edit/close/comment
3. **プルリクエスト** - pr list/view
4. **Wiki** - wiki list/view/create
5. **プロジェクト管理** - project list/view/init
6. **設定管理** - config get/set/list/path
7. **中継サーバー** - serve コマンドで自前構築可能
8. **複数プロファイル** - 複数のBacklogアカウント切り替え

## アーキテクチャ

```
┌─────────────────────────────────────────────────────────────────┐
│                      backlog CLI                                │
├─────────────────────────────────────────────────────────────────┤
│  cmd/backlog/     │  internal/cmd/    │  internal/config/       │
│  エントリーポイント │  コマンド定義      │  設定管理（レイヤード）    │
├─────────────────────────────────────────────────────────────────┤
│  internal/api/    │  internal/auth/   │  internal/relay/        │
│  Backlog API      │  OAuth認証(CLI)   │  中継サーバー            │
├─────────────────────────────────────────────────────────────────┤
│  internal/ui/     │  internal/cmdutil/                          │
│  対話的UI (survey) │  コマンドヘルパー                             │
└─────────────────────────────────────────────────────────────────┘
```

## 設定の優先順位（レイヤードアーキテクチャ）

```
LayerArgs（コマンド引数）
↑ 最優先
LayerCredentials（~/.config/backlog/credentials/{profile}.yaml）
↑
LayerEnv（環境変数 BACKLOG_*）
↑
LayerProject（.backlog.yaml）
↑
LayerUser（~/.config/backlog/config.yaml）
↑
LayerDefaults（内蔵デフォルト）
↓ 最低優先
```

## コマンド体系

```
backlog <resource> <action> [arguments] [flags]

# グローバルフラグ
--profile     プロファイル指定
--project/-p  プロジェクト指定
--output/-o   出力形式 (table, json)
--no-color    色出力無効化

# 認証
backlog auth login     OAuth/API Key認証でログイン
backlog auth logout    ログアウト（クレデンシャル削除）
backlog auth status    認証状態を表示
backlog auth me        現在のユーザー情報を表示
backlog auth setup     中継サーバーを設定（内部用）

# 課題
backlog issue list     課題一覧表示
backlog issue view     課題詳細表示
backlog issue create   課題作成（対話的）
backlog issue edit     課題編集（対話的）
backlog issue close    課題クローズ
backlog issue comment  コメント追加

# プルリクエスト
backlog pr list        PR一覧表示
backlog pr view        PR詳細表示

# Wiki
backlog wiki list      Wiki一覧表示
backlog wiki view      Wikiページ表示
backlog wiki create    Wikiページ作成

# プロジェクト
backlog project list   プロジェクト一覧表示
backlog project view   プロジェクト詳細表示
backlog project init   .backlog.yamlを作成

# 設定
backlog config get     設定値を取得
backlog config set     設定値を設定
backlog config list    全設定を一覧表示
backlog config path    設定ファイルのパスを表示

# サーバー
backlog serve          OAuth中継サーバーを起動

# その他
backlog version        バージョン表示
backlog completion     シェル補完スクリプト生成
```

## 実装フェーズ

| Phase | 内容            | ファイル                        | 状態 |
|-------|---------------|-----------------------------|-----|
| 01    | 基盤構築          | `01-foundation.md`          | ✅ |
| 02    | 設定管理          | `02-config.md`              | ✅ |
| 03    | 中継サーバー基本      | `03-relay-server.md`        | ✅ |
| 04    | CLI認証         | `04-cli-auth.md`            | ✅ |
| 05    | 中継サーバー拡張      | `05-relay-advanced.md`      | ✅ |
| 06    | APIクライアント     | `06-api-client.md`          | ✅ |
| 07    | issueコマンド     | `07-issue-commands.md`      | ✅ |
| 08    | projectコマンド   | `08-project-commands.md`    | ✅ |
| 09    | 追加コマンド        | `09-additional-commands.md` | ✅ |
| 10    | 改善            | `10-improvements.md`        | 進行中 |

## 主な実装上の変更点（計画からの差異）

1. **レイヤードアーキテクチャ採用** - 設定管理を6層のレイヤーで管理
2. **認証情報の分離** - クレデンシャルは `credentials/{profile}.yaml` に別ファイルで保存
3. **API Key認証追加** - OAuth以外にAPI Keyによる認証もサポート
4. **複数プロファイル** - `--profile` フラグで複数アカウント対応
5. **internal/cmdutil パッケージ追加** - コマンド間で共有するヘルパー関数を分離
6. **表示設定の拡充** - `display.*` 設定でテーブル表示をカスタマイズ可能

## 設計ドキュメント参照

OAuth中継サーバーの詳細設計は以下を参照：

- `/docs/oauth-relay-server-design.md` (プロジェクト内)
