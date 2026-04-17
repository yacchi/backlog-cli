---
name: backlog-issue
description: Fetch Backlog issue details automatically. Use when detecting issue key patterns like PROJECT-123, MYPROJ-456.
allowed-tools: Bash, Read
---

# Backlog Issue Skill

Issue operations triggered by detecting issue key patterns: `[A-Z][A-Z0-9_]+-[0-9]+` (e.g., `PROJECT-123`, `DEV_TEAM-1`)

**For text formatting rules and JSON output options, see the `backlog` skill.**
**For gh CLI command mapping, see [docs/gh-command-mapping.md](../../../docs/gh-command-mapping.md).**

## Quick Start

```bash
backlog issue view PROJ-123 --brief  # quick overview
backlog issue view PROJ-123          # full details
backlog issue view PROJ-123 -c default  # with comments
```

If you get an authentication error, run `backlog auth login --web`.

## View Issue

```bash
backlog issue view PROJ-123 --brief    # key, summary, status, assignee, URL
backlog issue view PROJ-123            # full details
backlog issue view PROJ-123 -c default # show comments (default count)
backlog issue view PROJ-123 -c 50      # show 50 comments
backlog issue view PROJ-123 -c all     # show all comments
backlog issue view PROJ-123 -c default --comments-order asc  # oldest first
backlog issue view PROJ-123 -c all --comments-since 12345    # comments after ID
backlog issue view PROJ-123 --web      # open in browser
backlog issue view PROJ-123 --json summary,status
backlog issue view PROJ-123 --summary  # AI summary (description only)
backlog issue view PROJ-123 --summary --summary-with-comments  # AI summary with comments
backlog issue view PROJ-123 --markdown # convert Backlog notation to GFM
backlog issue view PROJ-123 --raw      # render raw content without conversion
backlog issue view 123                 # uses configured project (PROJ-123)
```

## List Issues

```bash
# Basic
backlog issue list
backlog issue ls

# Filter by state
backlog issue list -s open|closed|all

# Filter by assignee
backlog issue list -a @me
backlog issue list --mine

# Filter by author
backlog issue list -A @me

# Filter by issue type
backlog issue list -T Bug
backlog issue list --type "タスク"

# Filter by category (like gh --label)
backlog issue list -l "Bug"
backlog issue list --category "UI,Backend"

# Filter by milestone
backlog issue list -m "v1.0"
backlog issue list --milestone "Sprint1,Sprint2"

# Search
backlog issue list -S "keyword"

# Sort and order
backlog issue list --sort priority --order asc
backlog issue list --sort created --order desc
# Sort fields: created, updated, issueType, category, priority, dueDate, etc.

# Limit and pagination
backlog issue list -L 20       # fetch up to 20
backlog issue list -L 100      # fetch up to 100
backlog issue list -L 0        # fetch ALL (auto-pagination)

# Count only
backlog issue list --count

# Open in browser
backlog issue list --web

# AI summary
backlog issue list --summary
backlog issue list --summary --summary-with-comments

# Markdown conversion
backlog issue list --markdown

# JSON output
backlog issue list --json issueKey,summary
backlog issue list --output json
```

## Create Issue

```bash
# Interactive
backlog issue create

# With options
backlog issue create -t "Title" -b "Description"
backlog issue create -t "Title" --body-file desc.md
echo "Description" | backlog issue create -t "Title" -F -

# With type and priority
backlog issue create -t "Bug" --type Bug --priority High
```

## Add/Edit/Delete Comment

```bash
# Add comment
backlog issue comment PROJ-123 -b "Comment text"
backlog issue comment PROJ-123 --body-file comment.md
echo "Comment" | backlog issue comment PROJ-123 -F -
backlog issue comment PROJ-123 --editor

# Edit last comment
backlog issue comment PROJ-123 --edit-last

# Edit specific comment by ID
backlog issue comment PROJ-123 --edit <comment-id>

# Delete last comment
backlog issue comment PROJ-123 --delete-last
backlog issue comment PROJ-123 --delete-last --yes  # skip confirmation
```

## Edit Issue

```bash
backlog issue edit PROJ-123 -t "New title" -b "New description"
backlog issue edit PROJ-123 --body-file desc.md
backlog issue edit PROJ-123 -a @me              # assign to self
backlog issue edit PROJ-123 --status 2          # change status
backlog issue edit PROJ-123 --priority High     # change priority
backlog issue edit PROJ-123 --due 2024-12-31    # set due date
backlog issue edit PROJ-123 -t "Updated" -c "Changed title"  # with comment
backlog issue edit PROJ-123 --milestone "v1.0"  # set milestone
backlog issue edit PROJ-123 --remove-milestone  # remove milestone
backlog issue edit PROJ-123 --category "Bug,Critical"       # replace categories
backlog issue edit PROJ-123 --add-category "Urgent"         # add category
backlog issue edit PROJ-123 --remove-category "Low Priority" # remove category
```

## Close/Reopen Issue

```bash
# Close
backlog issue close PROJ-123
backlog issue close PROJ-123 -c "Fixed in v1.2"
backlog issue close PROJ-123 --resolution 0

# Reopen
backlog issue reopen PROJ-123
backlog issue reopen PROJ-123 -c "Reopening for investigation"
```

## Delete Issue

```bash
backlog issue delete PROJ-123           # with confirmation prompt
backlog issue delete PROJ-123 --yes     # skip confirmation
```

**Warning**: This action cannot be undone. Requires administrator or project administrator permissions.

## Issue Status

```bash
backlog issue status   # show issues relevant to you (assigned, created, recently updated)
```

## Error Handling

| Error | Solution |
|-------|----------|
| Not authenticated | Run `backlog auth login --web` |
| Project not found | Check key with `backlog project list` |
| Issue not found | Verify key format and project access |

## Usage Patterns

**Quick lookup**: `backlog issue view PROJ-123 --brief`

**Detailed analysis**: `backlog issue view PROJ-123 -c all`

**AI-assisted analysis**: `backlog issue view PROJ-123 --summary --summary-with-comments`

**Multiple issues**: Fetch each with `--brief`, present consolidated summary

**Create from context**: Use `-t` and `-b` flags; for long descriptions, use `--body-file`

**Fetch all issues**: `backlog issue list -L 0` (auto-pagination)
