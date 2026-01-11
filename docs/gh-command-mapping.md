# gh コマンド対応表

backlog-cli は gh コマンド (GitHub CLI) と同様の操作感を提供することを目指しています。
AIエージェントは「ghコマンドと同様に使える」と理解することで、効率的にBacklog操作が可能です。

## コマンド対応表

### 認証 (auth)

| gh コマンド          | backlog コマンド          | 説明       |
|------------------|-----------------------|----------|
| `gh auth login`  | `backlog auth login`  | 認証（ログイン） |
| `gh auth logout` | `backlog auth logout` | ログアウト    |
| `gh auth status` | `backlog auth status` | 認証状態確認   |
| `gh auth token`  | -                     | (未対応)    |

### 課題 (issue)

| gh コマンド                     | backlog コマンド                  | 説明           |
|-----------------------------|-------------------------------|--------------|
| `gh issue list`             | `backlog issue list`          | 課題一覧         |
| `gh issue view <number>`    | `backlog issue view <key>`    | 課題詳細表示       |
| `gh issue create`           | `backlog issue create`        | 課題作成         |
| `gh issue edit <number>`    | `backlog issue edit <key>`    | 課題編集         |
| `gh issue close <number>`   | `backlog issue close <key>`   | 課題クローズ       |
| `gh issue reopen <number>`  | `backlog issue reopen <key>`  | 課題再オープン      |
| `gh issue comment <number>` | `backlog issue comment <key>` | コメント追加/編集/削除 |
| `gh issue delete <number>`  | `backlog issue delete <key>`  | 課題削除         |
| `gh issue status`           | `backlog issue status`        | 自分関連の課題概要    |

### プルリクエスト (pr)

| gh コマンド                   | backlog コマンド                  | 説明               |
|---------------------------|-------------------------------|------------------|
| `gh pr list`              | `backlog pr list`             | PR一覧             |
| `gh pr view <number>`     | `backlog pr view <number>`    | PR詳細表示           |
| `gh pr create`            | `backlog pr create`           | PR作成             |
| `gh pr comment <number>`  | `backlog pr comment <number>` | PRコメント           |
| `gh pr edit <number>`     | `backlog pr edit <number>`    | PR編集             |
| `gh pr close <number>`    | `backlog pr close <number>`   | PRクローズ           |
| `gh pr merge <number>`    | -                             | (Backlog API未対応) |
| `gh pr checkout <number>` | -                             | (未対応)            |

### リポジトリ (repo)

| gh コマンド                | backlog コマンド               | 説明                   |
|------------------------|----------------------------|----------------------|
| `gh repo list`         | `backlog repo list`        | リポジトリ一覧              |
| `gh repo view <name>`  | `backlog repo view <name>` | リポジトリ詳細              |
| `gh repo clone <name>` | -                          | (未対応、git cloneを直接使用) |
| `gh repo create`       | -                          | (未対応)                |

### プロジェクト (project)

| gh コマンド           | backlog コマンド              | 説明                      |
|-------------------|---------------------------|-------------------------|
| `gh project list` | `backlog project list`    | プロジェクト一覧                |
| `gh project view` | `backlog project view`    | プロジェクト詳細                |
| -                 | `backlog project init`    | プロジェクト初期化 (Backlog固有)   |
| -                 | `backlog project current` | 現在のプロジェクト表示 (Backlog固有) |

### 設定 (config)

| gh コマンド                       | backlog コマンド                       | 説明    |
|-------------------------------|------------------------------------|-------|
| `gh config get <key>`         | `backlog config get <key>`         | 設定値取得 |
| `gh config set <key> <value>` | `backlog config set <key> <value>` | 設定値設定 |
| `gh config list`              | `backlog config list`              | 設定一覧  |

## gh CLI に専用コマンドがない機能

以下は gh CLI に専用サブコマンドがないため、Backlog CLI 独自のコマンド体系を提供しています。
GitHub 自体に同等機能があっても、gh CLI では `gh api` を使う必要があるケースを含みます。

### マイルストーン (milestone)

GitHub 自体にはマイルストーン機能がありますが、gh CLI には専用コマンドがありません。

| backlog コマンド                    | gh CLI での代替手段                      | 説明        |
|---------------------------------|------------------------------------|-----------|
| `backlog milestone list`        | `gh api repos/.../milestones`      | マイルストーン一覧 |
| `backlog milestone view <id>`   | `gh api repos/.../milestones/{id}` | マイルストーン詳細 |
| `backlog milestone create`      | `gh api -X POST ...`               | マイルストーン作成 |
| `backlog milestone edit <id>`   | `gh api -X PATCH ...`              | マイルストーン編集 |
| `backlog milestone delete <id>` | `gh api -X DELETE ...`             | マイルストーン削除 |

### Wiki

GitHub 自体には Wiki 機能がありますが、gh CLI には専用コマンドがありません。

| backlog コマンド             | gh CLI での代替手段 | 説明       |
|--------------------------|---------------|----------|
| `backlog wiki list`      | (API経由のみ)     | Wiki一覧   |
| `backlog wiki view <id>` | (API経由のみ)     | Wiki詳細表示 |
| `backlog wiki create`    | (API経由のみ)     | Wiki作成   |
| `backlog wiki edit <id>` | (API経由のみ)     | Wiki編集   |

### 課題種別 (issue-type) - Backlog 特有

GitHub には課題種別の概念がありません（ラベルで代替）。Backlog では種別は必須の単一値です。

| backlog コマンド                     | 説明     |
|----------------------------------|--------|
| `backlog issue-type list`        | 課題種別一覧 |
| `backlog issue-type view <id>`   | 課題種別詳細 |
| `backlog issue-type create`      | 課題種別作成 |
| `backlog issue-type edit <id>`   | 課題種別編集 |
| `backlog issue-type delete <id>` | 課題種別削除 |

### 通知・ウォッチ

| backlog コマンド                     | gh CLI での代替手段 | 説明                        |
|----------------------------------|---------------|---------------------------|
| `backlog notification list`      | (なし)          | 通知一覧                      |
| `backlog notification read <id>` | (なし)          | 通知既読                      |
| `backlog watching list`          | (なし)          | ウォッチ一覧                    |
| `backlog watching add <key>`     | (なし)          | ウォッチ追加                    |
| `backlog watching remove <key>`  | (なし)          | ウォッチ解除                    |
| `backlog status`                 | `gh status`   | ステータスサマリー表示 (gh status相当) |

### マスタデータ取得 - AIエージェント向け

AIエージェントが課題を作成・更新する際に必要なID解決に使用します。

| backlog コマンド                | gh CLI での代替手段                       | 説明          |
|-----------------------------|-------------------------------------|-------------|
| `backlog space`             | (なし)                                | スペース情報取得    |
| `backlog user list`         | `gh api /users` (組織ユーザー)            | ユーザー一覧      |
| `backlog user view <id>`    | `gh api /users/:id`                 | ユーザー詳細      |
| `backlog priority list`     | (なし、GitHubにはラベルで代替)                 | 優先度一覧       |
| `backlog resolution list`   | (なし、GitHubには完了理由がない)                | 解決状況一覧      |
| `backlog category list`     | `gh api repos/.../labels`           | カテゴリ一覧      |
| `backlog category create`   | `gh api -X POST repos/.../labels`   | カテゴリ作成      |
| `backlog category delete`   | `gh api -X DELETE repos/.../labels` | カテゴリ削除      |
| `backlog custom-field list` | (なし、GitHubにはカスタムフィールドがない)           | カスタムフィールド一覧 |

### その他

| backlog コマンド               | 説明              |
|----------------------------|-----------------|
| `backlog markdown migrate` | Backlog記法→GFM変換 |

## 共通オプション

gh コマンドと同様のオプションをサポートしています。

| オプション                   | 説明                            |
|-------------------------|-------------------------------|
| `-p, --project <key>`   | プロジェクトキー指定 (`-R, --repo` に相当) |
| `--output <format>`     | 出力形式 (table, json)            |
| `--json <fields>`       | 指定フィールドのみJSON出力               |
| `-q, --jq <expression>` | jq式でJSON出力をフィルタリング            |
| `--no-color`            | カラー出力無効化                      |

**注**: `--output` と `--json` にはショートオプションがありません（gh CLI との互換性のため）。

## 使用例

```bash
# 課題一覧を取得
gh issue list # GitHub
backlog issue list # Backlog

# 課題を作成
gh issue create --title "Bug" # GitHub
backlog issue create --title "Bug" # Backlog

# PR一覧を取得
gh pr list # GitHub
backlog pr list # Backlog

# リポジトリ一覧を取得
gh repo list # GitHub
backlog repo list # Backlog

# JSON形式で出力
gh issue list --json title,number # GitHub
backlog issue list --json title,issueKey # Backlog

# jqでフィルタリング
gh issue list --json title,number --jq '.[].title' # GitHub
backlog issue list --json title,issueKey -q '.[].title' # Backlog
```

## オプション対応状況

各コマンドの主要オプションの対応状況です。

### issue list

| gh オプション          | backlog オプション      | 対応状況                   |
|-------------------|--------------------|------------------------|
| `--assignee / -a` | `--assignee / -a`  | ✅ 対応 (@me対応)           |
| `--state / -s`    | `--state / -s`     | ✅ 対応 (open/closed/all) |
| `--limit / -L`    | `--limit / -L`     | ✅ 対応                   |
| `--search / -S`   | `--search / -S`    | ✅ 対応                   |
| `--author / -A`   | `--author / -A`    | ✅ 対応                   |
| `--web / -w`      | `--web / -w`       | ✅ 対応                   |
| `--label / -l`    | `--category / -l`  | ✅ 対応 (Backlogのカテゴリで代替) |
| `--milestone`     | `--milestone / -m` | ✅ 対応                   |
| -                 | `--mine`           | ✅ Backlog独自            |
| -                 | `--count`          | ✅ Backlog独自            |
| -                 | `--summary`        | ✅ Backlog独自 (AI要約)     |

### issue edit

| gh オプション           | backlog オプション        | 対応状況                   |
|--------------------|----------------------|------------------------|
| `--title / -t`     | `--title / -t`       | ✅ 対応                   |
| `--body / -b`      | `--body / -b`        | ✅ 対応                   |
| `--body-file / -F` | `--body-file / -F`   | ✅ 対応                   |
| `--assignee / -a`  | `--assignee / -a`    | ✅ 対応 (@me対応)           |
| `--add-label`      | `--add-category`     | ✅ 対応 (カテゴリ追加)          |
| `--remove-label`   | `--remove-category`  | ✅ 対応 (カテゴリ削除)          |
| `--milestone / -m` | `--milestone / -m`   | ✅ 対応                   |
| -                  | `--remove-milestone` | ✅ Backlog独自            |
| -                  | `--status`           | ✅ Backlog独自            |
| -                  | `--priority`         | ✅ Backlog独自            |
| -                  | `--due`              | ✅ Backlog独自            |
| -                  | `--comment / -c`     | ✅ Backlog独自            |
| -                  | `--category`         | ✅ Backlog独自 (カテゴリ完全置換) |

### issue comment

| gh オプション           | backlog オプション      | 対応状況                  |
|--------------------|--------------------|-----------------------|
| `--body / -b`      | `--body / -b`      | ✅ 対応                  |
| `--body-file / -F` | `--body-file / -F` | ✅ 対応                  |
| `--editor / -e`    | `--editor / -e`    | ✅ 対応                  |
| `--edit-last`      | `--edit-last`      | ✅ 対応                  |
| `--delete-last`    | `--delete-last`    | ✅ 対応                  |
| -                  | `--edit`           | ✅ Backlog独自 (ID指定で編集) |
| -                  | `--yes`            | ✅ Backlog独自 (確認スキップ)  |

### issue status

| gh オプション | backlog オプション   | 対応状況                 |
|----------|-----------------|----------------------|
| -        | `--output json` | ✅ 対応 (JSON出力)        |
| -        | `--json`        | ✅ 対応 (フィールド指定JSON出力) |
| -        | `-q / --jq`     | ✅ 対応 (jqフィルタ)        |

### pr list

| gh オプション          | backlog オプション     | 対応状況                          |
|-------------------|-------------------|-------------------------------|
| `--state / -s`    | `--state / -s`    | ✅ 対応 (open/closed/merged/all) |
| `--limit / -L`    | `--limit / -L`    | ✅ 対応                          |
| `--author / -A`   | `--author / -A`   | ✅ 対応 (@me対応)                  |
| `--assignee / -a` | `--assignee / -a` | ✅ 対応 (@me対応)                  |
| `--base / -B`     | -                 | ❌ 非対応 (Backlog APIでフィルタ非対応)   |
| `--head / -H`     | -                 | ❌ 非対応 (Backlog APIでフィルタ非対応)   |
| `--web / -w`      | `--web / -w`      | ✅ 対応                          |
| `-R, --repo`      | `-R, --repo`      | ✅ Backlog必須                   |
| -                 | `--count`         | ✅ Backlog独自                   |

### pr view

| gh オプション          | backlog オプション     | 対応状況        |
|-------------------|-------------------|-------------|
| `--web / -w`      | `--web / -w`      | ✅ 対応        |
| `--comments / -c` | `--comments / -c` | ✅ 対応        |
| `-R, --repo`      | `-R, --repo`      | ✅ Backlog必須 |
| -                 | `--markdown`      | ✅ Backlog独自 |

### pr create

| gh オプション          | backlog オプション  | 対応状況                     |
|-------------------|----------------|--------------------------|
| `--base / -B`     | `--base / -B`  | ✅ 対応                     |
| `--head / -H`     | `--head / -H`  | ✅ 対応                     |
| `--title / -t`    | `--title / -t` | ✅ 対応                     |
| `--body / -b`     | `--body / -b`  | ✅ 対応                     |
| `--reviewer / -r` | `--reviewer`   | ✅ 対応                     |
| `--assignee / -a` | `--assignee`   | ✅ 対応                     |
| `--draft / -d`    | -              | ❌ 非対応 (Backlog APIに機能なし) |
| `-R, --repo`      | `-R, --repo`   | ✅ Backlog必須              |
| -                 | `--issue`      | ✅ Backlog独自 (関連課題)       |

### pr close

| gh オプション               | backlog オプション          | 対応状況                                |
|------------------------|------------------------|-------------------------------------|
| `--comment / -c`       | `--comment / -c`       | ✅ 対応                                |
| `--delete-branch / -d` | `--delete-branch / -d` | ⚠️ オプションあり (Backlog APIで非対応のため警告表示) |
| `-R, --repo`           | `-R, --repo`           | ✅ Backlog必須                         |
| -                      | `--yes`                | ✅ Backlog独自                         |

### auth login

| gh オプション          | backlog オプション         | 対応状況                        |
|-------------------|-----------------------|-----------------------------|
| `--with-token`    | `--with-token`        | ✅ 対応 (APIキー入力)              |
| `--hostname / -h` | `--space`, `--domain` | ✅ 対応 (Backlog形式)            |
| `--web`           | `--web`               | ✅ 対応                        |
| `--scopes / -s`   | -                     | ❌ 非対応 (BacklogはOAuth固定スコープ) |
| -                 | `--reuse`             | ✅ Backlog独自                 |

## GitHub Label と Backlog カテゴリの対応

GitHub の `--label` オプションは、Backlog の `--category` オプションで代替できます。

### 概念の対応

| GitHub | Backlog         | 特徴                 |
|--------|-----------------|--------------------|
| Label  | カテゴリ (Category) | 複数指定可、任意設定         |
| -      | 種別 (Issue Type) | 単一指定、必須設定、テンプレート連携 |

### Backlog の種別とカテゴリの違い

- **種別 (issueTypeId)**: 課題作成時に必須の単一値。「バグ」「タスク」「要望」など。テンプレートに紐付く
- **カテゴリ (categoryId[])**: 任意で複数指定可能。GitHubのラベルと同様の用途

### 使用例

```bash
# カテゴリでフィルター（GitHub の --label に相当）
backlog issue list --category "バグ"
backlog issue list --category "UI,バックエンド" # 複数指定
backlog issue list -l "緊急対応" # 短縮形

# マイルストーンでフィルター
backlog issue list --milestone "v1.0"
backlog issue list -m "Sprint1,Sprint2" # 複数指定

# 複合フィルター
backlog issue list --category "バグ" --milestone "v1.0" --state open

# 課題作成時にカテゴリを指定
backlog issue create --title "新機能" --category "機能追加,UI"

# 課題編集時にカテゴリを変更
backlog issue edit PROJ-123 --category "バグ,緊急"

```

## Backlog 独自機能

以下は gh にない Backlog CLI 独自の機能です。

| 機能                     | 説明                  |
|------------------------|---------------------|
| `--summary`            | AI による課題要約          |
| `--markdown`           | Backlog 記法→GFM 自動変換 |
| `--type`, `--priority` | 種別・優先度指定（種別は必須の単一値） |
| `--resolution`         | 完了理由指定              |

## AIエージェント向けガイダンス

AIエージェントがBacklog CLIを使用する際のガイドです。

### 基本原則

1. **コマンド構造**: gh コマンドと同じ `<resource> <action>` 構造
2. **引数の指定**: 課題は `PROJECT-123` 形式のキーで指定
3. **プロジェクト指定**: `-p PROJECT` または `.backlog.yaml` で設定
4. **出力形式**: `-o json` でプログラム処理しやすいJSON出力が可能
5. **ヘルプ**: `backlog <command> --help` で詳細なヘルプを表示

### タスク別コマンドリファレンス

#### 課題管理

| やりたいこと          | コマンド                                             |
|-----------------|--------------------------------------------------|
| 課題一覧を取得         | `backlog issue list`                             |
| 自分に割り当てられた課題を見る | `backlog issue list -a @me`                      |
| 特定の課題の詳細を見る     | `backlog issue view PROJECT-123`                 |
| 新しい課題を作成        | `backlog issue create --title "タイトル"`            |
| 課題のタイトルや説明を編集   | `backlog issue edit PROJECT-123 --title "新タイトル"` |
| 課題にコメントを追加      | `backlog issue comment PROJECT-123 -b "コメント"`    |
| 課題をクローズ         | `backlog issue close PROJECT-123`                |
| 自分関連の課題概要を見る    | `backlog issue status`                           |

#### マイルストーン管理

| やりたいこと          | コマンド                                        |
|-----------------|---------------------------------------------|
| マイルストーン一覧を取得    | `backlog milestone list`                    |
| マイルストーンの詳細を見る   | `backlog milestone view <id>`               |
| 新しいマイルストーンを作成   | `backlog milestone create --name "v1.0"`    |
| マイルストーンを編集      | `backlog milestone edit <id> --name "v1.1"` |
| マイルストーンを削除      | `backlog milestone delete <id>`             |
| 特定マイルストーンの課題を見る | `backlog issue list --milestone "v1.0"`     |

#### Wiki管理

| やりたいこと     | コマンド                                |
|------------|-------------------------------------|
| Wiki一覧を取得  | `backlog wiki list`                 |
| Wikiの内容を見る | `backlog wiki view <id>`            |
| 新しいWikiを作成 | `backlog wiki create --name "タイトル"` |
| Wikiを編集    | `backlog wiki edit <id>`            |

#### プルリクエスト管理

| やりたいこと     | コマンド                                                     |
|------------|----------------------------------------------------------|
| PR一覧を取得    | `backlog pr list -R <repo>`                              |
| PRの詳細を見る   | `backlog pr view <number> -R <repo>`                     |
| 新しいPRを作成   | `backlog pr create -R <repo> --base main --head feature` |
| PRにコメントを追加 | `backlog pr comment <number> -R <repo> -b "LGTM"`        |
| PRをクローズ    | `backlog pr close <number> -R <repo>`                    |

#### 課題種別・カテゴリ管理

| やりたいこと     | コマンド                                             |
|------------|--------------------------------------------------|
| 課題種別一覧を取得  | `backlog issue-type list`                        |
| 新しい課題種別を作成 | `backlog issue-type create --name "機能追加"`        |
| カテゴリでフィルター | `backlog issue list --category "バグ"`             |
| 課題にカテゴリを設定 | `backlog issue edit PROJECT-123 --category "UI"` |

#### 通知・ウォッチ

| やりたいこと    | コマンド                                  |
|-----------|---------------------------------------|
| 通知一覧を確認   | `backlog notification list`           |
| 通知を既読にする  | `backlog notification read <id>`      |
| 課題をウォッチ   | `backlog watching add PROJECT-123`    |
| ウォッチを解除   | `backlog watching remove PROJECT-123` |
| ウォッチ一覧を確認 | `backlog watching list`               |

#### マスタデータ取得（課題作成・更新時のID解決に必須）

| やりたいこと         | コマンド                                     |
|----------------|------------------------------------------|
| スペース情報を確認      | `backlog space`                          |
| ユーザー一覧を取得      | `backlog user list`                      |
| ユーザー詳細を確認      | `backlog user view <id>`                 |
| 優先度一覧を取得       | `backlog priority list`                  |
| 解決状況一覧を取得      | `backlog resolution list`                |
| カテゴリ一覧を取得      | `backlog category list`                  |
| カテゴリを作成        | `backlog category create --name "新カテゴリ"` |
| カテゴリを削除        | `backlog category delete <id>`           |
| カスタムフィールド一覧を取得 | `backlog custom-field list`              |

**AIエージェントが課題を作成する際の典型的なワークフロー:**

```bash
# 1. 課題種別IDを取得
backlog issue-type list --output json

# 2. 優先度IDを取得
backlog priority list --output json

# 3. 担当者候補のユーザーIDを取得
backlog user list --output json

# 4. カテゴリIDを取得（必要に応じて）
backlog category list --output json

# 5. 課題を作成
backlog issue create --title "タイトル" --type <type-id >--priority <priority-id >--assignee <user-id>
```

### gh CLI との使い分け

AIエージェントとして GitHub と Backlog の両方を操作する場合：

| 操作対象    | 使用するCLI        |
|---------|----------------|
| GitHub  | `gh` コマンド      |
| Backlog | `backlog` コマンド |

**判断基準**:

- リポジトリURLが `github.com` → `gh` を使用
- リポジトリURLが `*.backlog.com` または `*.backlog.jp` → `backlog` を使用
- プロジェクトディレクトリに `.backlog.yaml` がある → Backlog プロジェクト

### gh CLI にない機能を使う場合

以下の操作は Backlog CLI 固有のコマンドを使用します（gh では `gh api` が必要）：

```bash
# マイルストーン操作
backlog milestone list
backlog milestone create --name "Sprint1" --start-date 2024-01-01 --due-date 2024-01-14

# Wiki操作
backlog wiki list
backlog wiki create --name "設計書" --content "# 概要\n..."

# 課題種別操作
backlog issue-type list
backlog issue-type create --name "改善" --color "#ff0000"

# 通知・ウォッチ
backlog notification list
backlog watching add PROJECT-123
```
