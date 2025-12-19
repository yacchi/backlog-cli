# Backlog CLI

Backlog をターミナルから操作するためのコマンドラインツールです。

課題の管理、プルリクエスト、Wiki など、Backlog の主要機能をコマンドラインから利用できます。

## インストール

### Go でインストール

```bash
go install github.com/yacchi/backlog-cli/cmd/backlog@latest
```

## クイックスタート

### 1. 認証方式の選択

Backlog CLI は 2 つの認証方式をサポートしています：

| 方式            | 特徴                  | 設定                          |
|---------------|---------------------|-----------------------------|
| **API Key**   | シンプルで手軽。中継サーバー不要    | Backlog の個人設定から API Key を取得 |
| **OAuth 2.0** | セキュア。API Key の管理が不要 | 中継サーバーの設定が必要                |

### 2. ログイン

#### 方式 A: API Key 認証（推奨・簡単）

中継サーバーの設定なしで、すぐに利用を開始できます。

```bash
backlog auth login
```

対話形式でドメイン（backlog.jp / backlog.com）とスペース名を入力し、API Key を入力します。

API Key は Backlog の個人設定ページから取得できます：
`https://YOUR_SPACE.backlog.jp/EditApiSettings.action`

#### 方式 B: OAuth 2.0 認証

OAuth 2.0 を使用する場合は、事前に中継サーバーの設定が必要です。

```bash
# 1. 中継サーバーを設定
backlog auth setup https://relay.example.com

# 2. ログイン（ブラウザが開きます）
backlog auth login
```

> ⚠️ **セキュリティに関する重要な注意**
>
> 中継サーバーは OAuth 認証フローを仲介し、アクセストークンとリフレッシュトークンにアクセスできます。
> **信頼できない中継サーバーを使用すると、あなたの Backlog アカウントへのアクセス権限が漏洩する危険があります。**
>
> 中継サーバーは必ず以下のいずれかを使用してください：
> - 自分自身でホストしたサーバー
> - 所属組織が運営するサーバー
> - 十分に信頼できる第三者が運営するサーバー
>
> 中継サーバーの構築用コードは本リポジトリの `deploy/` ディレクトリに含まれています（AWS Lambda 用の CDK テンプレート）。

### 3. プロジェクトの設定（オプション）

リポジトリのルートで以下を実行すると、そのディレクトリでのデフォルトプロジェクトを設定できます：

```bash
backlog project init
```

これにより `.backlog.yaml` ファイルが作成されます：

```yaml
project:
  name: PROJECT_KEY
```

このファイルをリポジトリにコミットすることで、チームメンバーと設定を共有できます。

### 4. 課題の操作

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

| コマンド               | 説明                                   |
|--------------------|--------------------------------------|
| `auth login`       | Backlog にログイン（API Key または OAuth 2.0） |
| `auth logout`      | ログアウト                                |
| `auth status`      | 認証状態を表示                              |
| `auth setup <URL>` | OAuth 2.0 用の中継サーバーを設定                |

#### 再ログイン（`--reuse` オプション）

トークンの有効期限が切れた場合など、同じ設定で再ログインしたい場合は `--reuse` オプションを使用します：

```bash
backlog auth login --reuse
# または
backlog auth login -r
```

このオプションを使用すると、前回のログイン設定（認証方式、スペース、ドメイン）をそのまま再利用し、確認プロンプトをスキップします。

#### OAuth 認証完了ページの自動クローズ（オプション）

OAuth 認証完了後のブラウザタブを自動で閉じたい場合は、Tampermonkey
等のユーザースクリプトマネージャーをインストールした上で、以下のリンクからユーザースクリプトをインストールしてください：

[Backlog CLI Auto Close をインストール](https://github.com/yacchi/backlog-cli/raw/master/scripts/backlog-cli-auto-close.user.js)

### 課題 (`issue`)

| コマンド                  | 説明       |
|-----------------------|----------|
| `issue list`          | 課題一覧を表示  |
| `issue view <KEY>`    | 課題の詳細を表示 |
| `issue create`        | 新しい課題を作成 |
| `issue edit <KEY>`    | 課題を編集    |
| `issue close <KEY>`   | 課題をクローズ  |
| `issue comment <KEY>` | コメントを追加  |

### プルリクエスト (`pr`)

| コマンド           | 説明            |
|----------------|---------------|
| `pr list`      | プルリクエスト一覧を表示  |
| `pr view <ID>` | プルリクエストの詳細を表示 |
| `pr create`    | 新しいプルリクエストを作成 |

### Wiki (`wiki`)

| コマンド             | 説明              |
|------------------|-----------------|
| `wiki list`      | Wiki ページ一覧を表示   |
| `wiki view <ID>` | Wiki ページの詳細を表示  |
| `wiki create`    | 新しい Wiki ページを作成 |

### プロジェクト (`project`)

| コマンド                 | 説明                    |
|----------------------|-----------------------|
| `project list`       | プロジェクト一覧を表示           |
| `project view <KEY>` | プロジェクトの詳細を表示          |
| `project init`       | 現在のディレクトリにプロジェクト設定を作成 |

### 設定 (`config`)

| コマンド                       | 説明           |
|----------------------------|--------------|
| `config get <KEY>`         | 設定値を取得       |
| `config set <KEY> <VALUE>` | 設定値を変更       |
| `config list`              | すべての設定を表示    |
| `config path`              | 設定ファイルのパスを表示 |

### その他

| コマンド         | 説明              |
|--------------|-----------------|
| `serve`      | OAuth 中継サーバーを起動 |
| `version`    | バージョン情報を表示      |
| `completion` | シェル補完スクリプトを生成   |

## グローバルオプション

| オプション           | 説明                                            |
|-----------------|-----------------------------------------------|
| `-s, --space`   | Backlog スペース名                                 |
| `--domain`      | Backlog ドメイン (`backlog.jp` または `backlog.com`) |
| `-p, --project` | プロジェクトキー                                      |
| `-o, --output`  | 出力形式 (`table` または `json`)                     |
| `--no-color`    | カラー出力を無効化                                     |

## 設定

設定は以下の優先順位で読み込まれます：

1. コマンドライン引数
2. 環境変数
3. `.backlog.yaml`（カレントディレクトリ）
4. `~/.config/backlog/config.yaml`（グローバル設定）

### 環境変数

| 変数名               | 説明            |
|-------------------|---------------|
| `BACKLOG_SPACE`   | Backlog スペース名 |
| `BACKLOG_DOMAIN`  | Backlog ドメイン  |
| `BACKLOG_PROJECT` | デフォルトプロジェクトキー |

## シェル補完

### Bash

```bash
backlog completion bash >/etc/bash_completion.d/backlog
```

### Zsh

```bash
backlog completion zsh >"${fpath[1]}/_backlog"
```

### Fish

```bash
backlog completion fish >~/.config/fish/completions/backlog.fish
```

## ライセンス

Apache License 2.0

## 関連リンク

- [Backlog](https://backlog.com/)
- [Backlog API ドキュメント](https://developer.nulab.com/ja/docs/backlog/)