# Backlog CLI

Backlog をターミナルから操作するためのコマンドラインツールです。

課題の管理、プルリクエスト、Wiki など、Backlog の主要機能をコマンドラインから利用できます。

## インストール

### Go でインストール

```bash
go install github.com/yacchi/backlog-cli/cmd/backlog@latest
```

### ソースからビルド

```bash
git clone https://github.com/yacchi/backlog-cli.git
cd backlog-cli
make build
```

ビルドされたバイナリは `build/backlog` に出力されます。

## クイックスタート

### 1. ログイン

```bash
backlog auth login --space YOUR_SPACE --domain backlog.jp
```

ブラウザが開き、Backlog の OAuth 認証が行われます。

### 2. プロジェクトの設定（オプション）

リポジトリのルートで以下を実行すると、そのディレクトリでのデフォルトプロジェクトを設定できます：

```bash
backlog project init
```

または `.backlog.yaml` ファイルを手動で作成：

```yaml
space: YOUR_SPACE
domain: backlog.jp
project: PROJECT_KEY
```

### 3. 課題の操作

```bash
# 課題一覧を表示
backlog issue list

# 課題の詳細を表示
backlog issue view ISSUE-123

# 新しい課題を作成
backlog issue create

# 課題にコメントを追加
backlog issue comment ISSUE-123

# 課題をクローズ
backlog issue close ISSUE-123
```

## コマンド一覧

### 認証 (`auth`)

| コマンド | 説明 |
|---------|------|
| `auth login` | Backlog にログイン |
| `auth logout` | ログアウト |
| `auth status` | 認証状態を表示 |
| `auth setup` | 中継サーバーの設定 |

### 課題 (`issue`)

| コマンド | 説明 |
|---------|------|
| `issue list` | 課題一覧を表示 |
| `issue view <KEY>` | 課題の詳細を表示 |
| `issue create` | 新しい課題を作成 |
| `issue edit <KEY>` | 課題を編集 |
| `issue close <KEY>` | 課題をクローズ |
| `issue comment <KEY>` | コメントを追加 |

### プルリクエスト (`pr`)

| コマンド | 説明 |
|---------|------|
| `pr list` | プルリクエスト一覧を表示 |
| `pr view <ID>` | プルリクエストの詳細を表示 |
| `pr create` | 新しいプルリクエストを作成 |

### Wiki (`wiki`)

| コマンド | 説明 |
|---------|------|
| `wiki list` | Wiki ページ一覧を表示 |
| `wiki view <ID>` | Wiki ページの詳細を表示 |
| `wiki create` | 新しい Wiki ページを作成 |

### プロジェクト (`project`)

| コマンド | 説明 |
|---------|------|
| `project list` | プロジェクト一覧を表示 |
| `project view <KEY>` | プロジェクトの詳細を表示 |
| `project init` | 現在のディレクトリにプロジェクト設定を作成 |

### 設定 (`config`)

| コマンド | 説明 |
|---------|------|
| `config get <KEY>` | 設定値を取得 |
| `config set <KEY> <VALUE>` | 設定値を変更 |
| `config list` | すべての設定を表示 |
| `config path` | 設定ファイルのパスを表示 |

### その他

| コマンド | 説明 |
|---------|------|
| `serve` | OAuth 中継サーバーを起動 |
| `version` | バージョン情報を表示 |
| `completion` | シェル補完スクリプトを生成 |

## グローバルオプション

| オプション | 説明 |
|-----------|------|
| `-s, --space` | Backlog スペース名 |
| `--domain` | Backlog ドメイン (`backlog.jp` または `backlog.com`) |
| `-p, --project` | プロジェクトキー |
| `-o, --output` | 出力形式 (`table` または `json`) |
| `--no-color` | カラー出力を無効化 |

## 設定

設定は以下の優先順位で読み込まれます：

1. コマンドライン引数
2. 環境変数
3. `.backlog.yaml`（カレントディレクトリ）
4. `~/.config/backlog/config.yaml`（グローバル設定）

### 環境変数

| 変数名 | 説明 |
|--------|------|
| `BACKLOG_SPACE` | Backlog スペース名 |
| `BACKLOG_DOMAIN` | Backlog ドメイン |
| `BACKLOG_PROJECT` | デフォルトプロジェクトキー |

## シェル補完

### Bash

```bash
backlog completion bash > /etc/bash_completion.d/backlog
```

### Zsh

```bash
backlog completion zsh > "${fpath[1]}/_backlog"
```

### Fish

```bash
backlog completion fish > ~/.config/fish/completions/backlog.fish
```

## ライセンス

MIT License

## 関連リンク

- [Backlog](https://backlog.com/)
- [Backlog API ドキュメント](https://developer.nulab.com/ja/docs/backlog/)