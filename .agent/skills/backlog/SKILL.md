---
name: backlog
description: General Backlog operations including projects, wiki, pull requests, milestones, issue types, notifications, watching, and authentication management.
allowed-tools: Bash, Read
---

# Backlog Skill

General Backlog operations beyond issue management.

**For gh CLI command mapping and option compatibility, see [docs/gh-command-mapping.md](../../../docs/gh-command-mapping.md).**

## Authentication

```bash
# Check API access (preferred over auth status)
backlog auth me --quiet && echo "authenticated" || echo "not authenticated"

# Login (opens browser)
backlog auth login --web
backlog auth login --web --space myspace  # specific space

# Logout
backlog auth logout
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
# Filter issues
backlog issue list --category "Bug"
backlog issue list -l "Bug,UI"

# Set on issue (replaces/adds/removes)
backlog issue edit PROJ-123 --category "Bug,Critical"
backlog issue edit PROJ-123 --add-category "Urgent"
backlog issue edit PROJ-123 --remove-category "Low Priority"
```

## Notification & Watching

```bash
# Notifications
backlog notification list
backlog notification read <id>

# Watching
backlog watching list
backlog watching add PROJ-123
backlog watching remove PROJ-123
```

## Wiki Operations

```bash
# List/view
backlog wiki list
backlog wiki view <page-id>

# Create
backlog wiki create --name "Page Title" --content "Content"
```

## Pull Request Operations

```bash
# List
backlog pr list
backlog pr list -s open|closed|merged|all
backlog pr list -L 20

# View
backlog pr view <pr-id>
```

## Configuration

```bash
backlog config list
backlog config get client.default.project
backlog config set client.default.project PROJ
backlog config set display.timezone "Asia/Tokyo"
```

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
