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
# 1. Check authentication
backlog auth me --quiet || backlog auth login --web

# 2. Fetch issue
backlog issue view PROJ-123 --brief  # quick overview
backlog issue view PROJ-123          # full details
backlog issue view PROJ-123 --comments  # with comments
```

## View Issue

```bash
backlog issue view PROJ-123 --brief    # key, summary, status, assignee, URL
backlog issue view PROJ-123            # full details
backlog issue view PROJ-123 --comments # with comments
backlog issue view PROJ-123 --web      # open in browser
backlog issue view PROJ-123 --json summary,status
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

# Search
backlog issue list -S "keyword"

# Limit
backlog issue list -L 20

# JSON output
backlog issue list --json issueKey,summary
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

## Add Comment

```bash
backlog issue comment PROJ-123 -b "Comment text"
backlog issue comment PROJ-123 --body-file comment.md
echo "Comment" | backlog issue comment PROJ-123 -F -
backlog issue comment PROJ-123 --editor
```

## Edit Issue

```bash
backlog issue edit PROJ-123 -t "New title" -b "New description"
backlog issue edit PROJ-123 --body-file desc.md
backlog issue edit PROJ-123 -a @me              # assign to self
backlog issue edit PROJ-123 --status 2          # change status
backlog issue edit PROJ-123 -t "Updated" -c "Changed title"  # with comment
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

## Error Handling

| Error | Solution |
|-------|----------|
| Not authenticated | Run `backlog auth login --web` |
| Project not found | Check key with `backlog project list` |
| Issue not found | Verify key format and project access |

## Usage Patterns

**Quick lookup**: `backlog issue view PROJ-123 --brief`

**Detailed analysis**: `backlog issue view PROJ-123 --comments`

**Multiple issues**: Fetch each with `--brief`, present consolidated summary

**Create from context**: Use `-t` and `-b` flags; for long descriptions, use `--body-file`
