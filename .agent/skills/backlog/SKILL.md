---
name: backlog
description: Backlog CLI operations — issues, projects, wiki, pull requests, documents, milestones, issue types, categories, custom fields, users, files, notifications, watching, AI features, markdown migration, and authentication. Also use automatically when detecting issue key patterns like PROJECT-123, MYPROJ-456.
allowed-tools: Bash, Read
---

# Backlog Skill

Backlog operations via the `backlog` CLI (a gh-compatible command-line tool).

**Flags & usage**: run `backlog <command> --help` (e.g. `backlog wiki create --help`), or `backlog cli-ref` for the full Backlog-specific catalog across all commands. Only notes that are NOT obvious from `--help` are kept below.
**See also**: [docs/gh-command-mapping.md](../../../docs/gh-command-mapping.md) (gh CLI mapping & option compatibility).

If you get an authentication error, log in first: `backlog auth login --web` (also `--with-token`, `--reuse`, `--space <name>`).

## Issues

Triggered automatically by issue key patterns: `[A-Z0-9_]+-[0-9]+` — a project key (uppercase alphanumerics and underscores) followed by `-` and a number (e.g. `PROJECT-123`, `DEV_TEAM-1`).

- `backlog issue view <key>` — view one issue. `--brief` = quick overview (key, summary, status, assignee, URL); `-c default|N|all` includes comments; `--summary [--summary-with-comments]` = AI summary; `--markdown` converts Backlog notation to GFM. A bare number (`123`) uses the configured project.
- `backlog issue list` (alias `ls`) — list/filter issues. `-L 0` fetches ALL via auto-pagination. Filters follow gh: `-s open|closed|all`, `-a`/`-A @me`, `-T` type, `-l` category, `-m` milestone, `-S` search, `--mine`, `--count`.
- `backlog issue create` (alias `new`) — see **Creating an issue** below.
- `backlog issue comment <key>` — add (`-b` / `-F -` / `--editor`), edit (`--edit-last` / `--edit <id>`), or delete (`--delete-last`) a comment.
- `backlog issue edit <key>` — update fields: `-t`, `-b`, `-a`, `--status`, `--priority`, `--due`, `--milestone`, and `--category` / `--add-category` / `--remove-category`. **Patch mode**: `--patch '{"find":"old","replace":"new"}'`, `--append`, `--prepend`, `--safe`. See "Patch Editing" below.
- `backlog issue close <key>` / `reopen <key>` — change state; `-c` adds a comment, `--resolution` on close.
- `backlog issue delete <key>` — **irreversible**; requires admin or project-admin. `--yes` skips confirmation.
- `backlog issue status` — issues relevant to you (assigned, created, recently updated).

### Creating an issue (non-interactive)

Non-interactive create (MCP/script) requires **3 values resolved first**:

| Required   | Flag         | Accepts                  | Resolve with                                    |
|------------|--------------|--------------------------|-------------------------------------------------|
| Issue type | `--type`     | ID or name               | `backlog issue-type list --json`                |
| Priority   | `--priority` | ID only                  | `backlog priority list --json` (2=高, 3=中, 4=低) |
| Assignee   | `--assignee` | ID, userId, name, `@me`  | `backlog user list --json`, or `@me` for self   |

```bash
backlog issue create -t "Title" -b "Desc" --type Bug --priority 3 --assignee @me
echo "Desc" | backlog issue create -t "Title" -F - --type Bug --priority 3 -a @me
```

For long bodies use `--body-file`; attach files with `--attach` (repeatable).

## Other Commands

- **status** — `backlog status` (gh status equivalent: notifications, watched & assigned issues).
- **project** — `list`, `view <key>`, `current`, `init <key>` (creates `.backlog.yaml`). Set a global default with `backlog config set client.default.project PROJ`.
- **pr** — `list`/`view`/`create`/`edit`/`comment`/`close`/`merge`, all requiring `-R <repo>`. `--issue` links a related issue; `--markdown` on view converts Backlog notation.
- **document** — `list`/`tree`/`view`/`create`/`delete`/`comment list`/`tag`/`attachment download`. **IDs are strings** (e.g. `01HXXXX`), unlike issue/wiki. No update API — use `wiki` for editable pages; comments are read-only.
- **wiki** — `list`/`view`/`create`/`edit`/`delete`. `--notify` on edit sends mail. **Patch mode**: `edit --patch '{"find":"old","replace":"new"}'`, `--append`, `--prepend`, `--safe`. See "Patch Editing" below.
- **milestone** — `list`/`view`/`create`/`edit`/`delete`. Filter issues with `backlog issue list -m`.
- **issue-type** — `list`/`view`/`create`/`edit`/`delete`. Backlog-specific; **required single value** on issue create.
- **category** — `list`/`create`/`delete`. Equivalent to GitHub labels (`backlog issue list -l`, `issue edit --category`).
- **custom-field** (alias `cf`) — `list`.
- **repo** — `list`/`view <name>`.
- **user** — `list`/`view <id>`.
- **space** — `backlog space` (space info).
- **priority** / **resolution** — `list` (master data for create/edit/close).
- **notification** (alias `notif`) — `list`/`read <id>`.
- **watching** (alias `watch`) — `list`/`add <key>`/`remove <key>`.
- **ai** — `prompt optimize`/`prompt apply` (tune AI summary prompts).
- **api** — direct authenticated requests like `gh api` (`-X`, `-F`, `-i`, `--input -`, `-s`).
- **markdown migrate** — `init`/`list`/`status`/`apply`/`rollback`/`clean`/`snapshot`; `backlog markdown logs` for conversion logs.

## Patch Editing (Wiki & Issue Description)

Wiki ページと課題の説明文（description）を部分的に更新する機能。全文を生成・送信せずに変更箇所だけを指定でき、同時編集による変更消失を自動検出する。

### モード選択ガイド

| やりたいこと | 使うモード | フラグ |
|------------|----------|------|
| 特定のテキストだけ変えたい | Search-and-replace | `--find` + `--replace-with` |
| 末尾に情報を追加したい | Append | `--append` |
| 先頭にメタ情報を入れたい | Prepend | `--prepend` |
| 全文を書き換えたいが他者の変更を消したくない | Safe full replacement | `--content`/`--body` + `--safe` |
| 全文を確実に上書きしたい（フォールバック） | Direct replacement | `--content`/`--body`（既存動作） |

**選択の原則**: 変更範囲が狭いほど安全。`--find`/`--replace-with` > `--append`/`--prepend` > `--safe` > 直接置換 の順に衝突リスクが低い。

### Search-and-replace (`--patch`)

JSON で検索・置換のペアを指定する。単体オブジェクト `{"find":"...","replace":"..."}` または配列 `[{...},{...}]` を受け付ける。対象が見つからなければエラー（古い内容を参照していたことを即座に検出）。

```bash
# 単一の置換
backlog wiki edit 12345 --patch '{"find":"旧テキスト","replace":"新テキスト"}'
backlog issue edit PROJ-123 --patch '{"find":"Status: Draft","replace":"Status: Review"}'

# 複数箇所を一度に置換
backlog wiki edit 12345 --patch '[{"find":"## 未着手","replace":"## 着手済み"},{"find":"担当: 未定","replace":"担当: 山田"}]'

# stdin から読み込み（大きなパッチや MCP 向き）
echo '[{"find":"A","replace":"B"}]' | backlog wiki edit 12345 --patch-file -
```

**エラー例**: `patch target not found: "旧テキスト"` → ページが既に更新されている。`backlog wiki view` で現在の内容を確認してリトライ。

### Append / Prepend

既存の内容に一切触れず、末尾・先頭にテキストを追加する。衝突リスクが最も低い。

```bash
# Wiki に議事録を追記
backlog wiki edit 12345 --append "## 2024-07-10 会議
- 決定事項A
- TODO: Bさんが調査"

# 課題の先頭に注意書きを挿入
backlog issue edit PROJ-123 --prepend "> ⚠ この課題はブロッカーです"
```

### Safe full replacement

全文を差し替えるが、書き込み前にリモートの変更を検出する。異なるセクションへの変更は三方マージで自動解決。同一箇所への変更はコンフリクトとしてエラー報告し、`--content`/`--body`（`--safe` なし）でのフォールバックを案内する。

```bash
backlog wiki edit 12345 --content "完全に新しい内容" --safe
backlog issue edit PROJ-123 --body "新しい本文" --safe
```

### 組み合わせ

パッチフラグは組み合わせ可能。適用順: patch → prepend → append。

```bash
# テキスト置換 + 末尾に変更履歴を追記
backlog wiki edit 12345 \
  --patch '{"find":"v1.0","replace":"v1.1"}' \
  --append "- 2024-07-10: v1.1 にバージョンアップ"
```

### MCP でのベストプラクティス

MCP ツール経由（LLM がバックログを操作する場合）では、パッチモードが特に有効:

1. **`--patch` JSON を最優先で使う** — モデルは変更箇所だけ生成すればよく、入出力トークンを大幅に節約。巨大な Wiki を丸ごと生成する必要がない
2. **追記には `--append`** — 議事録の追加、ステータス更新の追記など。内容を読む必要すらない場合がある
3. **`--safe` は全文書き換えが必要な時のみ** — 構造を大きく変える場合。衝突があれば自動マージを試みる
4. **直接置換はフォールバック** — パッチモードでコンフリクトが解消できない場合の最終手段

### エラーハンドリング

| エラー | 原因 | 対処 |
|-------|------|------|
| `patch target not found: "..."` | `--patch` の `find` 対象が現在の内容にない | `view` で現在の内容を確認し、正しいテキストで再試行 |
| `conflict: N region(s) could not be automatically merged` | 同じ箇所を別の人が変更済み | 内容を確認し、`--content`/`--body`（`--safe` なし）で上書き、または手動で再パッチ |
| `invalid patch JSON...` | `--patch` の JSON が不正 | `{"find":"...","replace":"..."}` または配列形式を確認 |

## File Operations

- **Space attachment** (upload foundation): `backlog space attachment upload <file>` → returns attachment ID as JSON.
- **Issue / Wiki / PR attachments**: `<noun> attachment list|upload|download|delete`. Upload auto-uploads via space attachment. (`pr` requires `-R <repo>`.)
- **Attach at create/edit**: `backlog issue create --attach f.png` / `backlog issue edit <key> --attach f.pdf`.
- **Shared file links** (link existing project files): `issue|wiki sharedfile list|link|unlink`.
- **Project shared files**: `backlog file list|download -p <proj>` — **read-only** (Backlog has no upload/delete API for these).

## Global Flags

Available on all commands: `-p/--project`, `--output table|json`, `--json <fields>`, `--jq <expr>`, `--profile <name>`, `--space <host>`, `--no-color`, `--debug`.

## Text Formatting

**Check the project's rule before posting content** — `backlog project view PROJ --json textFormattingRule` returns `"backlog"` or `"markdown"`.

When rule is `markdown`, use standard GitHub Flavored Markdown — **except** for these Backlog extensions that differ from GitHub:

```
![alt][filename]   ![alt][N]   embed an attached image inline (N = attachment's 1-based index).
                               NOT ![alt](url) — upload the file first (--attach / attachment upload),
                               then reference it by filename or index.
[label][filename]  [label][N]  link to an attached file (non-image).
[[BLG-98]]   BLG-95            link to an issue (bare key also auto-links).
[[WikiPageName]]              link to a wiki page.
```

When rule is `backlog`, use Backlog native syntax (not discoverable via `--help`):

```
*Heading 1  **Heading 2  ***Heading 3
''bold''  '''italic'''  %%strikethrough%%
[[link text>URL]]  [[WikiPageName]]
#issue-key (auto-links)
-bullet list  --sub item  ---sub sub item
+numbered list  ++sub item
{code}code block{/code}   {code:javascript}highlighted{/code}
>quote  >>nested quote
|header1|header2|h    |cell1|cell2|
&color(red){colored text}
#image(file.png)  #thumbnail(file.png)
```

## JSON Output (gh-compatible)

`--output json` (full), `--json field1,field2` (selected fields), `--json field -q '.[].field'` (jq filter).
Common fields: `issueKey`, `summary`, `description`, `status`, `assignee`, `priority`, `created`, `updated`, `projectKey`, `name`, `textFormattingRule`.
