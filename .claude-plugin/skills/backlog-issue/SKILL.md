---
name: backlog-issue
description: Fetch Backlog issue details automatically. Use when detecting issue key patterns like PROJECT-123, MYPROJ-456.
allowed-tools: Bash, Read
---

# Backlog Issue Skill

This skill handles Backlog issue operations when issue key patterns are detected.

## Trigger Conditions

Use this skill when you detect issue key patterns matching: `[A-Z][A-Z0-9_]+-[0-9]+`

Examples:
- `PROJECT-123`
- `MYPROJ-456`
- `DEV_TEAM-1`

## Usage Flow

### 1. Check Authentication

Before any operation, verify authentication:

```bash
backlog auth status --quiet && echo "authenticated" || echo "not authenticated"
```

If not authenticated, prompt the user to run:
```bash
backlog auth login
```

### 2. Check Current Project (Optional)

If the issue key has no project prefix, check if a default project is configured:

```bash
backlog project current --quiet && backlog project current
```

### 3. Fetch Issue Details

For quick overview:
```bash
backlog issue view <ISSUE-KEY> --brief
```

For full details:
```bash
backlog issue view <ISSUE-KEY>
```

For JSON output:
```bash
backlog issue view <ISSUE-KEY> --output json
```

## Issue Commands

### View Issue

```bash
# Brief summary (key, summary, status, assignee, URL)
backlog issue view PROJ-123 --brief

# Full details
backlog issue view PROJ-123

# With comments
backlog issue view PROJ-123 --comments

# Open in browser
backlog issue view PROJ-123 --web
```

### List Issues

```bash
# List issues in current project
backlog issue list

# Filter by status
backlog issue list --status open
backlog issue list --status closed

# Filter by assignee
backlog issue list --assignee @me
backlog issue list --assignee "User Name"

# Limit results
backlog issue list --limit 20

# JSON output for processing
backlog issue list --output json
```

### Create Issue

```bash
# Interactive mode
backlog issue create

# With options
backlog issue create --summary "Issue title" --description "Details"

# With issue type and priority
backlog issue create --summary "Bug report" --type Bug --priority High
```

### Add Comment

```bash
# Add comment to issue
backlog issue comment PROJ-123 --body "Comment text"

# Interactive mode
backlog issue comment PROJ-123
```

### Close Issue

```bash
backlog issue close PROJ-123
```

### Edit Issue

```bash
# Interactive edit
backlog issue edit PROJ-123

# Direct field update
backlog issue edit PROJ-123 --status "In Progress"
backlog issue edit PROJ-123 --assignee "@me"
```

## Error Handling

### Not Authenticated

If `backlog auth status --quiet` returns exit code 1:
- Inform the user they need to authenticate
- Suggest running `backlog auth login`

### Project Not Found

If the project key doesn't exist:
- Check if the project key is correct
- Suggest running `backlog project list` to see available projects

### Issue Not Found

If the issue doesn't exist:
- Verify the issue key format
- Check if the user has access to the project

## Use Cases

### Quick Issue Lookup

When user mentions "PROJ-123" in conversation:
1. Check authentication
2. Fetch brief info: `backlog issue view PROJ-123 --brief`
3. Present the summary to the user

### Detailed Analysis

When user asks for details about an issue:
1. Fetch full details: `backlog issue view PROJ-123`
2. Include comments if relevant: `backlog issue view PROJ-123 --comments`

### Multiple Issues

When user provides multiple issue keys:
1. Fetch each issue with `--brief` flag
2. Present a consolidated summary
