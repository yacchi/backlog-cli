---
name: backlog
description: General Backlog operations including projects, documents, wiki, pull requests, milestones, issue types, categories, custom fields, notifications, watching, users, AI features, markdown migration, file operations (issue/wiki/PR/project shared files, space attachment upload), and authentication management.
allowed-tools: Bash, Read
---

# Backlog Skill

General Backlog operations beyond issue management.

**For gh CLI command mapping and option compatibility, see [docs/gh-command-mapping.md](../../../docs/gh-command-mapping.md).**

## Authentication

Run commands directly. If you get an authentication error, login first.

```bash
# Login (opens browser)
backlog auth login --web
backlog auth login --web --space myspace  # specific space
backlog auth login --with-token           # API key input
backlog auth login --reuse                # reuse existing token

# Logout
backlog auth logout
```

## Status Summary (gh status equivalent)

```bash
backlog status              # show notifications, watched issues, assigned issues
backlog status -o json      # JSON output
```

## Project Operations

```bash
# List projects
backlog project list
backlog project list --json projectKey,name

# View project
backlog project view PROJ
backlog project view PROJ --json textFormattingRule

# Check/set current project
backlog project current
backlog project init PROJ  # create .backlog.yaml
backlog config set client.default.project PROJ  # global config
```

## File Operations

### Space Attachment Upload (foundation for issue/wiki uploads)

```bash
# Upload a file and get attachment ID (returned as JSON)
backlog space attachment upload report.pdf
# → { "id": 123, "name": "report.pdf", "size": 12345, ... }
```

### Issue Attachments

```bash
# List attachments
backlog issue attachment list PROJ-123
backlog issue attachment list PROJ-123 --json id,name,size

# Upload & attach files (auto-uploads via space/attachment)
backlog issue attachment upload PROJ-123 report.pdf
backlog issue attachment upload PROJ-123 img1.png img2.png

# Download
backlog issue attachment download PROJ-123 42
backlog issue attachment download PROJ-123 42 -o report.pdf
backlog issue attachment download PROJ-123 42 -o -   # stdout

# Delete
backlog issue attachment delete PROJ-123 42
backlog issue attachment delete PROJ-123 42 --yes    # skip confirmation
```

### Issue Shared File Links (project shared files)

```bash
backlog issue sharedfile list PROJ-123
backlog issue sharedfile link PROJ-123 456 789       # link by file ID
backlog issue sharedfile unlink PROJ-123 456
backlog issue sharedfile unlink PROJ-123 456 --yes
```

### Attaching Files at Issue Create/Edit

```bash
backlog issue create -t "Bug" --attach screenshot.png --attach log.txt
backlog issue edit PROJ-123 --attach additional.pdf
```

### Wiki Attachments

```bash
backlog wiki attachment list 100
backlog wiki attachment upload 100 doc.pdf
backlog wiki attachment upload 100 img1.png img2.png
backlog wiki attachment download 100 42 -o doc.pdf
backlog wiki attachment download 100 42 -o -         # stdout
backlog wiki attachment delete 100 42 --yes
```

### Wiki Shared File Links

```bash
backlog wiki sharedfile list 100
backlog wiki sharedfile link 100 456 789
backlog wiki sharedfile unlink 100 456 --yes
```

### Pull Request Attachments

```bash
backlog pr attachment list 3 -R myrepo
backlog pr attachment download 3 42 -R myrepo -o patch.diff
backlog pr attachment download 3 42 -R myrepo -o -   # stdout
backlog pr attachment delete 3 42 -R myrepo --yes
```

### Project Shared Files (read-only: list & download only)

Backlog API has no upload/delete endpoint for project shared files.

```bash
# List (defaults to root "/")
backlog file list -p MYPROJ
backlog file list -p MYPROJ /docs/design
backlog file list -p MYPROJ --json id,name,type --jq '.[] | select(.type=="file")'

# Download by shared file ID (from list output)
backlog file download 999 -p MYPROJ
backlog file download 999 -p MYPROJ -o /tmp/spec.pdf
backlog file download 999 -p MYPROJ -o -             # stdout
```

## Pull Request Operations

```bash
# List
backlog pr list -R <repo>
backlog pr list -R <repo> -s open|closed|merged|all
backlog pr list -R <repo> -L 20
backlog pr list -R <repo> -a @me        # filter by assignee
backlog pr list -R <repo> -A @me        # filter by author
backlog pr list -R <repo> --count       # count only
backlog pr list -R <repo> --web         # open in browser

# View
backlog pr view <number> -R <repo>
backlog pr view <number> -R <repo> -c   # with comments
backlog pr view <number> -R <repo> --web
backlog pr view <number> -R <repo> --markdown  # convert Backlog notation

# Create
backlog pr create -R <repo> -B main -H feature/xxx -t "Title" -b "Description"
backlog pr create -R <repo> -t "Title" --body-file desc.md   # from file
cat desc.md | backlog pr create -R <repo> -t "Title" -F -    # from stdin
backlog pr create -R <repo>             # interactive mode
backlog pr create -R <repo> --assignee <user-id> --reviewer "1234,5678"
backlog pr create -R <repo> --issue <issue-id>  # link related issue

# Edit
backlog pr edit <number> -R <repo> -t "New title"
backlog pr edit <number> -R <repo> -b "Updated description"
backlog pr edit <number> -R <repo> --body-file desc.md       # from file
cat desc.md | backlog pr edit <number> -R <repo> -F -        # from stdin
backlog pr edit <number> -R <repo> --assignee <user-id>
backlog pr edit <number> -R <repo> --issue <issue-id>

# Comment
backlog pr comment <number> -R <repo> -b "LGTM!"
backlog pr comment <number> -R <repo> --body-file review.md  # from file
cat comment.md | backlog pr comment <number> -R <repo> -F -  # from stdin
backlog pr comment <number> -R <repo>   # interactive mode

# Close (without merging)
backlog pr close <number> -R <repo>
backlog pr close <number> -R <repo> -c "Closing - no longer needed"
backlog pr close <number> -R <repo> --yes  # skip confirmation

# Merge
backlog pr merge <number> -R <repo>
backlog pr merge <number> -R <repo> -c "Merging after review"
backlog pr merge <number> -R <repo> --yes  # skip confirmation
```

## Document Operations

Document IDs are **string type** (e.g. `01HXXXXXXXX`), unlike Issue/Wiki which use integers.
The update (PATCH) API is not provided by Backlog — use `backlog wiki` for editable pages.

```bash
# List
backlog document list
backlog document list --keyword "design" --sort updated --order asc
backlog document list --limit 50

# Count
backlog document count

# Tree view (hierarchical structure)
backlog document tree
backlog document tree --include-trash

# View
backlog document view 01HXXXXXXXX
backlog document view 01HXXXXXXXX --web       # open in browser
backlog document view 01HXXXXXXXX --markdown  # show plain text
backlog document view 01HXXXXXXXX -o json     # JSON output

# Create
backlog document create --title "Design Doc" --content "# Design"
backlog document create --title "Notes" --content-file notes.md
cat doc.md | backlog document create --title "Doc" --content-file -
backlog document create --title "Sub" --parent 01HXXXXXXXX --emoji "📘"

# Delete (requires admin/project admin)
backlog document delete 01HXXXXXXXX
backlog document delete 01HXXXXXXXX --yes  # skip confirmation

# Comments (read-only, write API not provided by Backlog)
backlog document comment list 01HXXXXXXXX

# Tags
backlog document tag add 01HXXXXXXXX -t foo -t bar
backlog document tag remove 01HXXXXXXXX -t foo

# Attachments
backlog document attachment download 01HXXXXXXXX 123
backlog document attachment download 01HXXXXXXXX 123 -o report.pdf
backlog document attachment download 01HXXXXXXXX 123 -o -  # stdout
```

## Wiki Operations

```bash
# List/view
backlog wiki list
backlog wiki view <page-id>

# Create
backlog wiki create --name "Page Title" --content "Content"
backlog wiki create --name "Spec" --content-file spec.md     # from file
cat content.md | backlog wiki create --name "Page" -F -      # from stdin

# Edit
backlog wiki edit <id-or-name> --content "Updated content"
backlog wiki edit <id-or-name> --content-file updated.md     # from file
cat content.md | backlog wiki edit <id-or-name> -F -         # from stdin
backlog wiki edit <id-or-name> --name "New Page Name"
backlog wiki edit <id-or-name> --notify  # send mail notification

# Delete
backlog wiki delete <id-or-name>
backlog wiki delete <id-or-name> --yes  # skip confirmation
```

## Milestone Operations

```bash
# List/view
backlog milestone list
backlog milestone view <id>

# Create
backlog milestone create --name "Sprint 1"
backlog milestone create --name "v1.0" --start-date 2024-01-01 --due-date 2024-01-31

# Edit/delete
backlog milestone edit <id> --name "Sprint 1 (Extended)"
backlog milestone delete <id>

# Filter issues by milestone
backlog issue list --milestone "Sprint 1"
backlog issue list -m "v1.0,v1.1"
```

## Issue Type Operations

Backlog-specific feature (GitHub uses labels). Issue type is **required single value** when creating issues.

```bash
# List/view
backlog issue-type list
backlog issue-type view <id>

# Create/edit/delete
backlog issue-type create --name "Feature Request" --color "#00ff00"
backlog issue-type edit <id> --name "Enhancement"
backlog issue-type delete <id>
```

## Category Operations

Equivalent to GitHub Labels.

```bash
# List categories
backlog category list

# Create/delete categories
backlog category create --name "New Category"
backlog category delete <id>

# Filter issues
backlog issue list --category "Bug"
backlog issue list -l "Bug,UI"

# Set on issue (replaces/adds/removes)
backlog issue edit PROJ-123 --category "Bug,Critical"
backlog issue edit PROJ-123 --add-category "Urgent"
backlog issue edit PROJ-123 --remove-category "Low Priority"
```

## Custom Field Operations

```bash
backlog custom-field list       # list all custom fields
backlog cf list                 # alias
backlog custom-field list --output json
```

## Repository Operations

```bash
backlog repo list               # list Git repositories
backlog repo view <name>        # view repository details
backlog repo list --json name,description
```

## User Operations

```bash
backlog user list               # list users in the space
backlog user view <id>          # view user details
backlog user list --output json
```

## Space Information

```bash
backlog space                   # display space information
backlog space --json spaceKey,name,textFormattingRule
backlog space --output json
```

## Priority / Resolution (Master Data)

```bash
backlog priority list           # list available priorities
backlog resolution list         # list available resolutions
```

## Notification & Watching

```bash
# Notifications
backlog notification list
backlog notif list              # alias
backlog notification read <id>

# Watching
backlog watching list
backlog watch list              # alias
backlog watching add PROJ-123
backlog watching remove PROJ-123
```

## AI Features

```bash
# Prompt optimization for AI summaries
backlog ai prompt optimize      # optimize AI summary prompts
backlog ai prompt apply         # apply optimized prompt from history
```

## API (Direct API Access)

Make authenticated API requests directly (like `gh api`).

```bash
# GET requests
backlog api /api/v2/space
backlog api /api/v2/projects
backlog api /api/v2/issues -F "projectId[]=12345" -F "count=10"

# POST requests
backlog api /api/v2/issues -X POST -F "projectId=12345" -F "summary=New Issue" -F "issueTypeId=1" -F "priorityId=3"

# Include response headers
backlog api /api/v2/space -i

# Pass request body from stdin
echo '{"name":"test"}' | backlog api /api/v2/projects -X POST --input -

# Silent mode (no response body)
backlog api /api/v2/issues -X DELETE -s
```

## Markdown Migration

Migrate Backlog notation to GitHub Flavored Markdown (GFM) for a project.

```bash
# Full workflow
backlog markdown migrate init <projectKey>   # initialize workspace
backlog markdown migrate list                # list items to migrate
backlog markdown migrate status              # check migration status
backlog markdown migrate apply               # apply converted data
backlog markdown migrate rollback            # rollback if needed
backlog markdown migrate clean               # remove workspace
backlog markdown migrate snapshot --append   # snapshot data

# View conversion logs
backlog markdown logs
backlog markdown logs --limit 20
backlog markdown logs -o json
```

## Configuration

```bash
backlog config list
backlog config get client.default.project
backlog config set client.default.project PROJ
backlog config set display.timezone "Asia/Tokyo"
```

## Global Flags

Available on all commands:

| Flag                  | Description                                         |
|-----------------------|-----------------------------------------------------|
| `-p, --project <key>` | Backlog project key                                 |
| `--output <format>`   | Output format (table, json)                         |
| `--json <fields>`     | Output JSON with specified fields (comma-separated) |
| `--jq <expression>`   | Filter JSON output using a jq expression            |
| `--no-color`          | Disable color output                                |
| `--debug`             | Enable debug logging                                |
| `--profile <name>`    | Configuration profile to use                        |

## Text Formatting

**Check project's formatting rule before posting content:**

```bash
backlog project view PROJ --json textFormattingRule
# Returns: "backlog" or "markdown"
```

### Backlog Native Syntax (when rule is "backlog")

```
*Heading 1  **Heading 2  ***Heading 3
''bold''  '''italic'''  %%strikethrough%%
[[link text>URL]]  [[WikiPageName]]
#issue-key (auto-links)
-bullet list  --sub item  ---sub sub item
+numbered list  ++sub item
{code}code block{/code}
{code:javascript}highlighted{/code}
>quote  >>nested quote
|header1|header2|h
|cell1|cell2|
&color(red){colored text}
#image(file.png)  #thumbnail(file.png)
```

### Markdown Format (when rule is "markdown")

Use standard GitHub Flavored Markdown (GFM).

## JSON Output (gh CLI Compatible)

```bash
# Full JSON
backlog issue list --output json

# Specific fields
backlog issue list --json issueKey,summary,status

# With jq filter
backlog issue list --json issueKey -q '.[].issueKey'
```

Common fields: `issueKey`, `summary`, `description`, `status`, `assignee`, `priority`, `created`, `updated`, `projectKey`, `name`, `textFormattingRule`
