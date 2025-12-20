# Phase 00: 全体概要

## プロジェクトの目的

Backlog用CLI「backlog」を開発し、GitHub CLI (gh) と同様のユーザー体験を提供する。

## 主要機能

1. **OAuth2.0認証** - 中継サーバー経由でのセキュアな認証
2. **課題管理** - issue list/view/create/edit/close/comment
3. **プルリクエスト** - pr list/view/create
4. **Wiki** - wiki list/view/create
5. **プロジェクト管理** - project list/view/init
6. **設定管理** - config get/set/list
7. **中継サーバー** - serve コマンドで自前構築可能

## アーキテクチャ

```
┌─────────────────────────────────────────────────────────────────┐
│                      backlog CLI                                │
├─────────────────────────────────────────────────────────────────┤
│  cmd/backlog/     │  internal/cmd/    │  internal/config/       │
│  エントリーポイント │  コマンド定義      │  設定管理               │
├─────────────────────────────────────────────────────────────────┤
│  internal/api/    │  internal/auth/   │  internal/relay/        │
│  Backlog API      │  OAuth認証(CLI)   │  中継サーバー            │
├─────────────────────────────────────────────────────────────────┤
│  internal/ui/                                                   │
│  対話的UI (survey)                                              │
└─────────────────────────────────────────────────────────────────┘
```

## 設定の優先順位

```
コマンド引数 > 環境変数 > .backlog.yaml > ~/.config/backlog/config.yaml > embed デフォルト
```

## コマンド体系

```
backlog <resource> <action> [arguments] [flags]

# 認証
backlog auth login/logout/status/refresh/setup

# 課題
backlog issue list/view/create/edit/close/comment

# プルリクエスト
backlog pr list/view/create

# Wiki
backlog wiki list/view/create

# プロジェクト
backlog project list/view/init

# 設定
backlog config get/set/list/path

# サーバー
backlog serve
```

## 実装フェーズ

| Phase | 内容 | ファイル |
|-------|------|---------|
| 01 | 基盤構築 | `01-foundation.md` |
| 02 | 設定管理 | `02-config.md` |
| 03 | 中継サーバー基本 | `03-relay-server.md` |
| 04 | CLI認証 | `04-cli-auth.md` |
| 05 | 中継サーバー拡張 | `05-relay-advanced.md` |
| 06 | APIクライアント | `06-api-client.md` |
| 07 | issueコマンド | `07-issue-commands.md` |
| 08 | projectコマンド | `08-project-commands.md` |
| 09 | 追加コマンド | `09-additional-commands.md` |
| 10 | 改善 | `10-improvements.md` |

## 設計ドキュメント参照

OAuth中継サーバーの詳細設計は以下を参照：
- `/docs/oauth-relay-server-design.md` (プロジェクト内)
- または Claude との会話で策定した設計書

## 次のステップ

`01-foundation.md` に進んでプロジェクト基盤を構築してください。
