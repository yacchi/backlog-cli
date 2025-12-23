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
# List issues in current project (alias: ls)
backlog issue list
backlog issue ls

# Filter by state (open/closed/all, default: open)
backlog issue list --state open
backlog issue list -s closed
backlog issue list -s all

# Filter by assignee
backlog issue list --assignee @me
backlog issue list --mine

# Search issues
backlog issue list --search "keyword"
backlog issue list -S "bug fix"

# Limit results
backlog issue list -L 20

# Open in browser
backlog issue list --web

# JSON output for processing
backlog issue list --output json
```

### Create Issue

```bash
# Interactive mode
backlog issue create

# With options (alias: new)
backlog issue create --title "Issue title" --body "Details"
backlog issue new -t "Issue title" -b "Details"

# Read body from file
backlog issue create -t "Issue title" --body-file description.md

# Read body from stdin
echo "Description" | backlog issue create -t "Issue title" -F -

# With issue type and priority
backlog issue create -t "Bug report" --type Bug --priority High
```

Output includes the created issue URL:
```
Created PROJ-123
URL: https://space.backlog.com/view/PROJ-123
```

### Add Comment

```bash
# Add comment to issue
backlog issue comment PROJ-123 --body "Comment text"
backlog issue comment PROJ-123 -b "Quick note"

# Read comment from file
backlog issue comment PROJ-123 --body-file comment.md

# Read comment from stdin
echo "Comment" | backlog issue comment PROJ-123 -F -

# Open editor to write comment
backlog issue comment PROJ-123 --editor

# Interactive mode
backlog issue comment PROJ-123
```

Output includes the comment URL:
```
Added comment #456 to PROJ-123
URL: https://space.backlog.com/view/PROJ-123#comment-456
```

### Edit Issue

```bash
# Update title and body
backlog issue edit PROJ-123 --title "New title" --body "Updated description"
backlog issue edit PROJ-123 -t "New title" -b "Updated description"

# Read body from file
backlog issue edit PROJ-123 --body-file description.md

# Assign to yourself
backlog issue edit PROJ-123 --assignee @me

# Change status (by ID)
backlog issue edit PROJ-123 --status 2

# Add comment with edit
backlog issue edit PROJ-123 -t "Updated" --comment "Changed the title"
```

Output includes the issue URL:
```
Updated PROJ-123
URL: https://space.backlog.com/view/PROJ-123
```

### Close Issue

```bash
# Close an issue
backlog issue close PROJ-123

# Close with comment
backlog issue close PROJ-123 --comment "Fixed in v1.2"

# Close with resolution
backlog issue close PROJ-123 --resolution 0
```

Output includes the issue URL:
```
Closed PROJ-123
URL: https://space.backlog.com/view/PROJ-123
```

### Reopen Issue

```bash
# Reopen a closed issue
backlog issue reopen PROJ-123

# Reopen with comment
backlog issue reopen PROJ-123 --comment "Reopening for further investigation"
```

Output includes the issue URL:
```
Reopened PROJ-123
URL: https://space.backlog.com/view/PROJ-123
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

### Create Issue from Context

When user wants to create an issue:
1. Use `--title` and `--body` flags for non-interactive creation
2. For long descriptions, write to a temp file and use `--body-file`
3. The output URL can be shared with the user

### Update and Close

When closing or updating issues:
1. Commands output the issue URL after completion
2. Share the URL with the user for verification

## Text Formatting

**IMPORTANT**: Before posting any text content (comments, descriptions), check the project's text formatting rule and format your text accordingly.

### Get Formatting Rule

```bash
# Efficient: Get only the formatting rule
backlog project view PROJ --output json | jq -r '.textFormattingRule'
```

Returns either `backlog` or `markdown`.

### backlog format (Backlog native syntax)

```
*Heading 1  **Heading 2  ***Heading 3
''bold''  '''italic'''
[[link text>URL]]
-bullet list  --sub item
+numbered list  ++sub item
{code}code block{/code}
>quote
|header1|header2|h
|cell1|cell2|
```

### markdown format (GitHub Flavored Markdown)

```markdown
# Heading 1  ## Heading 2  ### Heading 3
**bold**  *italic*
[link text](URL)
- bullet list
  - sub item
1. numbered list
```code block```
> quote
| header1 | header2 |
|---------|---------|
| cell1   | cell2   |
```

### Workflow for Posting Text

1. Get project key from issue key (e.g., `PROJ-123` → `PROJ`)
2. Check formatting rule: `backlog project view PROJ --output json | jq -r '.textFormattingRule'`
3. Format your text content according to the rule
4. Post using the appropriate command
