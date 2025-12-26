# Backlog CLI

Backlog をターミナルから操作するためのコマンドラインツールです。

課題の管理、プルリクエスト、Wiki など、Backlog の主要機能をコマンドラインから利用できます。

## インストール

### Go でインストール

```bash
go install github.com/yacchi/backlog-cli/cmd/backlog@latest
```

### Claude Code プラグイン

[Claude Code](https://claude.com/claude-code) から Backlog を操作するためのプラグインも提供しています。
[yacchi/claude-plugins](https://github.com/yacchi/claude-plugins) Marketplace からインストールできます：

```bash
# Marketplace を追加（初回のみ）
/plugin marketplace add yacchi/claude-plugins

# プラグインをインストール
/plugin install backlog-cli
```

プラグインの詳細は [Claude Code プラグイン](#claude-code-プラグイン-1) セクションを参照してください。

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
# ログイン（ブラウザが開きます）
backlog auth login
```

初回はブラウザの設定画面で中継サーバーURLとスペース情報を登録します。
事前に設定しておきたい場合は `backlog config set profile.default.relay_server <URL>` を使用できます。

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

#### Relay Config Bundle（組織向け）

Relay Config Bundle は **中継サーバー側で作成・配布する前提** です。
利用者はバンドルをダウンロードして `backlog config import` するだけになります。

##### 1. 中継サーバーの環境変数を設定する

`docs/relay-config-bundle-spec.md` の「サーバー側の設定と実装要件」に従って、
サーバー設定 (`server.tenants`) を用意します。

```yaml
server:
  tenants:
    SPACEID_BACKLOG_JP:
      allowed_domain: spaceid.backlog.jp
      jwks: '{"keys":[{"kty":"OKP","crv":"Ed25519","kid":"2025-01","x":"...","d":"..."}]}'
      active_keys: "2025-01"
      info_ttl: 600
      passphrase_hash: "$2a$12$..."
```

JWKS の作成が必要な場合は、以下の Go スニペットで Ed25519 の
「秘密JWK」「公開JWKS」「thumbprint」を生成できます。

```bash
cat <<'EOF' > /tmp/relay-keygen.go
package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
)

type jwk struct {
	Kty string `json:"kty"`
	Crv string `json:"crv"`
	Kid string `json:"kid"`
	X   string `json:"x"`
	D   string `json:"d,omitempty"`
}

func main() {
	kid := os.Getenv("KID")
	if kid == "" {
		kid = "2025-01"
	}
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		panic(err)
	}
	x := base64.RawURLEncoding.EncodeToString(pub)
	d := base64.RawURLEncoding.EncodeToString(priv.Seed())

	privJWK := jwk{Kty: "OKP", Crv: "Ed25519", Kid: kid, X: x, D: d}
	pubJWK := jwk{Kty: "OKP", Crv: "Ed25519", Kid: kid, X: x}

	canonical := fmt.Sprintf(`{"crv":"%s","kty":"%s","x":"%s"}`, pubJWK.Crv, pubJWK.Kty, pubJWK.X)
	sum := sha256.Sum256([]byte(canonical))
	thumbprint := base64.RawURLEncoding.EncodeToString(sum[:])

	privJSON, _ := json.MarshalIndent(privJWK, "", "  ")
	pubJSON, _ := json.MarshalIndent(map[string][]jwk{"keys": {pubJWK}}, "", "  ")

	fmt.Println("== Private JWK (keep secret) ==")
	fmt.Println(string(privJSON))
	fmt.Println("== Public JWKS ==")
	fmt.Println(string(pubJSON))
	fmt.Println("== Thumbprint ==")
	fmt.Println(thumbprint)
}
EOF

KID=2025-01 go run /tmp/relay-keygen.go
```

##### 2. パスフレーズハッシュの生成

ポータル機能を使用する場合は、パスフレーズのbcryptハッシュを生成して設定に追加します：

```bash
# 引数で指定
backlog config hash your-secret-passphrase

# 対話的入力（パスワードが隠される）
backlog config hash
```

生成されたハッシュを `passphrase_hash` に設定してください。

##### 3. バンドルをダウンロードする

**方法A: セルフサービスポータル（推奨）**

中継サーバーのポータルURLにアクセスし、管理者から提供されたパスフレーズを入力してダウンロードします：

```
https://relay.example.com/portal/spaceid.backlog.jp
```

**方法B: API経由**

```
GET /v1/relay/tenants/<spaceid.backlogdomain>/bundle
```

ダウンロードした ZIP を `backlog config import` で取り込みます。

```bash
backlog config import spaceid.backlog.jp.backlog-cli.zip
```

#### セルフサービスポータル

組織のメンバーが自分でバンドルをダウンロードできるポータル機能を提供しています。

##### 機能

- パスフレーズ認証によるアクセス制御
- テナント情報（スペース、ドメイン、リレーサーバー）の表示
- バンドルのダウンロードボタン

##### ポータルURL

```
https://<relay-server>/portal/<spaceid.backlogdomain>
```

例: `https://relay.example.com/portal/myspace.backlog.jp`

##### 管理者向け設定

1. テナント設定にパスフレーズハッシュを追加:

```yaml
server:
  tenants:
    MYSPACE_BACKLOG_JP:
      allowed_domain: myspace.backlog.jp
      jwks: '{"keys":[...]}'
      active_keys: "2025-01"
      passphrase_hash: "$2a$12$..."  # backlog config hash で生成
```

2. パスフレーズをメンバーに共有

3. メンバーはポータルURLにアクセスし、パスフレーズを入力してバンドルをダウンロード

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

| コマンド          | 説明                                   |
|---------------|--------------------------------------|
| `auth login`  | Backlog にログイン（API Key または OAuth 2.0） |
| `auth logout` | ログアウト                                |
| `auth status` | 認証状態を表示                              |
| `auth me`     | ログイン中のユーザー情報を表示                      |

#### 再ログイン（`--reuse` オプション）

トークンの有効期限が切れた場合など、同じ設定で再ログインしたい場合は `--reuse` オプションを使用します：

```bash
backlog auth login --reuse
# または
backlog auth login -r
```

このオプションを使用すると、前回のログイン設定（認証方式、スペース、ドメイン）をそのまま再利用し、確認プロンプトをスキップします。

#### ブラウザ完結型認証（`--web` オプション）

自動化や、ターミナルでの入力が難しい環境では `--web` オプションを使用できます：

```bash
backlog auth login --web
```

このオプションを使用すると、すべての認証ステップ（ドメイン選択、スペース入力、認証方式選択など）がブラウザ上で行われます。

#### OAuth 認証完了ページの自動クローズ（オプション）

OAuth 認証完了後のブラウザタブを自動で閉じたい場合は、Tampermonkey
等のユーザースクリプトマネージャーをインストールした上で、以下のリンクからユーザースクリプトをインストールしてください：

[Backlog CLI Auto Close をインストール](https://github.com/yacchi/backlog-cli/raw/master/scripts/backlog-cli-auto-close.user.js)

### 課題 (`issue`)

| コマンド                  | 説明         |
|-----------------------|------------|
| `issue list`          | 課題一覧を表示    |
| `issue view <KEY>`    | 課題の詳細を表示   |
| `issue create`        | 新しい課題を作成   |
| `issue edit <KEY>`    | 課題を編集      |
| `issue close <KEY>`   | 課題をクローズ    |
| `issue comment <KEY>` | コメントを追加・編集 |

#### コメントの編集

既存のコメントを編集することもできます：

```bash
# コメントIDを指定して編集
backlog issue comment PROJ-123 --edit 12345 --body "Updated comment"
backlog issue comment PROJ-123 --edit 12345 --editor

# 自分の最後のコメントを編集
backlog issue comment PROJ-123 --edit-last --body "Updated comment"
backlog issue comment PROJ-123 --edit-last --editor
```

### プルリクエスト (`pr`)

| コマンド           | 説明            |
|----------------|---------------|
| `pr list`      | プルリクエスト一覧を表示  |
| `pr view <ID>` | プルリクエストの詳細を表示 |

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
| `project current`    | 現在のプロジェクトキーを表示        |

### 課題種別 (`issue-type`)

課題種別の作成・編集・削除を行います。エイリアス: `type`

| コマンド                         | 説明      |
|------------------------------|---------|
| `issue-type list`            | 種別一覧を表示 |
| `issue-type view <ID\|名前>`   | 種別詳細を表示 |
| `issue-type create`          | 種別を作成   |
| `issue-type edit <ID\|名前>`   | 種別を編集   |
| `issue-type delete <ID\|名前>` | 種別を削除   |

### 設定 (`config`)

| コマンド                       | 説明                           |
|----------------------------|------------------------------|
| `config get <KEY>`         | 設定値を取得                       |
| `config set <KEY> <VALUE>` | 設定値を変更                       |
| `config list`              | すべての設定を表示                    |
| `config path`              | 設定ファイルのパスを表示                 |
| `config import <ZIP>`      | Relay Config Bundle を取り込む   |
| `config hash [PASSPHRASE]` | bcryptハッシュを生成                |
| `config bundle create`     | Relay Config Bundle を作成     |

### Markdown (`markdown`)

Backlog 独自記法から GFM（GitHub Flavored Markdown）への変換をサポートします。

| コマンド               | 説明                       |
|--------------------|--------------------------|
| `markdown logs`    | Markdown 変換ログを表示         |
| `markdown migrate` | プロジェクト全体の Markdown を一括変換 |

#### Markdown マイグレーション

プロジェクト内の課題や Wiki、課題種別テンプレートを Backlog 記法から GFM に変換できます。
作業ディレクトリはデフォルトでカレントディレクトリを使います。

```bash
# 作業ディレクトリを初期化
backlog markdown migrate init DEV

# 変更点のプレビュー
backlog markdown migrate list --diff

# 変換を適用（対話モード）
backlog markdown migrate apply

# 自動適用（確認なし）
backlog markdown migrate apply --auto

# Dry-run（差分だけ表示、Backlog への反映とマージは行わない）
backlog markdown migrate apply --dry-run --auto

# 問題があればロールバック
backlog markdown migrate rollback

# 新規作成分を追加取り込み
backlog markdown migrate snapshot --append
```

作業ディレクトリは Git リポジトリとして扱われ、取得・変換・適用の差分がコミットとして記録されます。

#### 変換ルール

以下の変換ルールがサポートされています：

| ルール ID             | 変換内容                              |
|--------------------|-----------------------------------|
| `heading_asterisk` | `*見出し` → `# 見出し`                  |
| `quote_block`      | `{quote}` → `>`                   |
| `code_block`       | `{code}` → ` ``` `                |
| `emphasis_bold`    | `''bold''` → `**bold**`           |
| `emphasis_italic`  | `'''italic'''` → `*italic*`       |
| `strikethrough`    | `%%strike%%` → `~~strike~~`       |
| `backlog_link`     | `[[label>url]]` → `[label](url)`  |
| `toc`              | `#contents` → `[toc]`             |
| `line_break`       | `&br;` → `<br>`                   |
| `list_plus`        | `+ list` → `1. list`              |
| `list_dash_space`  | `-list` / `--nested` → インデント・空行補正 |
| `table_separator`  | `\|h` などのテーブル補正                   |
| `image_macro`      | `#image(...)` → `![image](...)`   |

#### Unsafe ルールの設定

一部のルールは誤変換のリスクがあるため、デフォルトでは `markdown migrate apply` 時にスキップされます。
これらは設定ファイルの `display.markdown_unsafe_rules` で管理されます：

```yaml
display:
  markdown_unsafe_rules:
    - heading_asterisk
    - list_plus
    - list_dash_space
    - table_separator
    - emphasis_bold
    - emphasis_italic
    - strikethrough
```

Unsafe ルールを適用するには、設定から該当ルールを削除してください。

### その他

| コマンド         | 説明              |
|--------------|-----------------|
| `serve`      | OAuth 中継サーバーを起動 |
| `version`    | バージョン情報を表示      |
| `completion` | シェル補完スクリプトを生成   |

## グローバルオプション

| オプション           | 説明                        |
|-----------------|---------------------------|
| `--profile`     | 使用する設定プロファイル名             |
| `-p, --project` | プロジェクトキー                  |
| `-o, --output`  | 出力形式 (`table` または `json`) |
| `-f, --format`  | Go テンプレートで出力をフィルタリング      |
| `--no-color`    | カラー出力を無効化                 |
| `--debug`       | デバッグログを有効化                |

### Go テンプレート出力 (`--format`)

JSON 出力から必要なフィールドだけを抽出できます：

```bash
# 課題キーとサマリーのみを表示
backlog issue list --format '{{range .}}{{.IssueKey}}: {{.Summary}}{{"\n"}}{{end}}'

# 特定のフィールドを取得
backlog issue view PROJ-123 --format '{{.Status.Name}}'
```

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

## Claude Code プラグイン

インストール方法は [インストール](#インストール) セクションを参照してください。

### 提供されるスキル

| スキル             | 説明                   |
|-----------------|----------------------|
| `backlog`       | プロジェクト・Wiki・PR・認証の操作 |
| `backlog-issue` | 課題キーパターン検出と課題操作      |

### 機能

- **課題キーの自動検出**: ブランチ名やコミットメッセージから課題キーを検出
- **認証状態チェック**: `--quiet` オプションでスクリプト向けの終了コード
- **テキスト形式対応**: プロジェクトの設定に応じた Backlog 記法 / Markdown の使い分け

## ライセンス

Apache License 2.0

## 関連リンク

- [Backlog](https://backlog.com/)
- [Backlog API ドキュメント](https://developer.nulab.com/ja/docs/backlog/)
