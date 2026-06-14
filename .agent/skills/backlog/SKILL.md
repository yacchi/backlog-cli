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
- `backlog issue edit <key>` — update fields: `-t`, `-b`, `-a`, `--status`, `--priority`, `--due`, `--milestone`, and `--category` / `--add-category` / `--remove-category`.
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
- **wiki** — `list`/`view`/`create`/`edit`/`delete`. `--notify` on edit sends mail.
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

When rule is `markdown`, use standard GitHub Flavored Markdown. When rule is `backlog`, use Backlog native syntax (not discoverable via `--help`):

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
