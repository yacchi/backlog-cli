---
name: backlog
description: General Backlog operations including projects, wiki, pull requests, and authentication management.
allowed-tools: Bash, Read
---

# Backlog Skill

This skill handles general Backlog operations beyond issue management.

## Authentication Management

### Check Authentication Status

```bash
# Check if credentials exist (no output, exit code only)
backlog auth status --quiet && echo "has credentials" || echo "no credentials"

# Show detailed status
backlog auth status
```

### Verify API Access

```bash
# Check if authenticated with API (no output, exit code only)
backlog auth me --quiet && echo "authenticated" || echo "not authenticated"

# Show current user info
backlog auth me
```

### Login

```bash
# Interactive OAuth login
backlog auth login

# Login to specific space
backlog auth login --space myspace
```

### Logout

```bash
backlog auth logout
```

## Project Operations

### List Projects

```bash
# List all accessible projects
backlog project list

# JSON output
backlog project list --output json
```

### View Project Details

```bash
backlog project view PROJ
```

### Check Current Project

```bash
# Get current project key (exit code 1 if not set)
backlog project current

# Quiet mode for scripting
backlog project current --quiet && echo "project set" || echo "no project"
```

### Initialize Project in Repository

```bash
# Create .backlog.yaml in current directory
backlog project init PROJ
```

## Wiki Operations

### List Wiki Pages

```bash
# List wiki pages in current project
backlog wiki list

# List wiki pages in specific project
backlog wiki list --project PROJ

# JSON output
backlog wiki list --output json
```

### View Wiki Page

```bash
backlog wiki view <page-id>
```

### Create Wiki Page

```bash
# Interactive mode
backlog wiki create

# With options
backlog wiki create --name "Page Title" --content "Page content"
```

## Pull Request Operations

### List Pull Requests

```bash
# List PRs in repository (alias: ls)
backlog pr list
backlog pr ls

# Filter by state (open/closed/merged/all, default: open)
backlog pr list --state open
backlog pr list -s closed
backlog pr list -s merged
backlog pr list -s all

# Limit results
backlog pr list -L 20

# Open in browser
backlog pr list --web

# JSON output
backlog pr list --output json
```

### View Pull Request

```bash
backlog pr view <pr-id>
```

## Configuration Management

### View Configuration

```bash
# List all configuration
backlog config list

# Get specific value
backlog config get client.default.project
```

### Set Configuration

```bash
# Set default project
backlog config set client.default.project PROJ

# Set display options
backlog config set display.timezone "Asia/Tokyo"
backlog config set display.hyperlink true
```

## Common Workflows

### Initial Setup

1. Login to Backlog:
   ```bash
   backlog auth login
   ```

2. Verify authentication:
   ```bash
   backlog auth me
   ```

3. List available projects:
   ```bash
   backlog project list
   ```

4. Set default project:
   ```bash
   backlog config set client.default.project PROJ
   ```

Or initialize in a git repository:
   ```bash
   backlog project init PROJ
   ```

### Check Current Environment

```bash
# Check authentication
backlog auth status

# Check current project
backlog project current

# Show user info
backlog auth me
```

## Error Handling

### Not Authenticated

When authentication is required:
1. Run `backlog auth me --quiet` to check (verifies actual API access including token refresh)
2. If exit code is 1, suggest `backlog auth login`

### No Project Configured

When project context is needed:
1. Run `backlog project current --quiet` to check
2. If exit code is 1, suggest:
   - `backlog project init PROJ` for repository-local config
   - `backlog config set client.default.project PROJ` for global config

## Text Formatting

**IMPORTANT**: Before posting any text content (wiki pages, PR comments, etc.), check the project's text formatting rule and format your text accordingly.

### Get Formatting Rule

```bash
# Efficient: Get only the formatting rule
backlog project view PROJ --output json | jq -r '.textFormattingRule'

# Get current project's formatting rule
backlog project current --quiet && \
  backlog project view "$(backlog project current)" --output json | jq -r '.textFormattingRule'
```

Returns either `backlog` or `markdown`.

### backlog format (Backlog native syntax)

```
*Heading 1  **Heading 2  ***Heading 3
''bold''  '''italic'''
%%strikethrough%%
[[link text>URL]]
[[WikiPageName]]
#issue-key (auto-links to issue)
-bullet list  --sub item  ---sub sub item
+numbered list  ++sub item
{code}code block{/code}
{code:javascript}highlighted code{/code}
>quote
>>nested quote
|header1|header2|h
|cell1|cell2|
&color(red){colored text}
#image(file.png)
#thumbnail(file.png)
```

### markdown format (GitHub Flavored Markdown)

```markdown
# Heading 1  ## Heading 2  ### Heading 3
**bold**  *italic*  ~~strikethrough~~
[link text](URL)
- bullet list
  - sub item
1. numbered list
```code block```
```javascript
highlighted code
```
> quote
| header1 | header2 |
|---------|---------|
| cell1   | cell2   |
![alt text](image.png)
```

### Applicable Contexts

Text formatting applies to:
- Issue descriptions and comments
- Wiki page content
- Pull request descriptions and comments
- Git commit comments

### Workflow for Posting Text

1. Identify the target project
2. Check formatting rule: `backlog project view PROJ --output json | jq -r '.textFormattingRule'`
3. Format your text content according to the rule
4. Post using the appropriate command
