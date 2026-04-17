---
name: backlog-api-sync
description: Sync Backlog API documentation from developer.nulab.com into local cache and OpenAPI spec. Use when API endpoints are missing, outdated, or need to be implemented.
triggers:
  - /backlog-api-sync
  - backlog api sync
  - backlog apiを更新
  - openapi.yamlにエンドポイントを追加
---

# Backlog API Sync Skill

Crawls Backlog developer documentation and maintains a local cache for efficient AI access.

## Key files

| File | Purpose |
|------|---------|
| `tmp/sitemap.xml` | Backlog API site sitemap — update manually when needed |
| `docs/api/cache.json` | **Primary reference** — structured metadata for all API endpoints |
| `docs/api/openapi.yaml` | Manually maintained OpenAPI spec used by ogen code generation |
| `docs/api/openapi-generated.yaml` | Auto-generated spec from cache (reference only, not used by ogen) |

## When to use this skill

- User asks "Backlog の `{operation}` API を実装して" → check cache first before fetching HTML
- `docs/api/openapi.yaml` is missing an endpoint → run `generate` to see what's available
- Backlog releases new API features → run `sync` to update the cache
- User runs `/backlog-api-sync`

## Workflow

### Step 1: Check if cache is up to date

```bash
uv run scripts/backlog-api-sync.py check
```

If output shows "New: 0 / Changed: 0", cache is current — skip to Step 3.

### Step 2: Fetch changed pages (only when needed)

```bash
# Fetch only changed/new pages (efficient, uses lastmod from sitemap)
uv run scripts/backlog-api-sync.py sync

# Re-fetch everything (when HTML structure changes)
uv run scripts/backlog-api-sync.py fetch --all && uv run scripts/backlog-api-sync.py generate
```

This updates `docs/api/cache.json`. Typically takes 1–3 minutes for a full crawl.

### Step 3: Use the cache

Read `docs/api/cache.json` directly — no HTTP needed.

Each entry has:
```json
{
  "title": "課題の追加",
  "method": "POST",
  "path": "/issues",
  "description": "参加しているプロジェクトに新しい課題を追加します。",
  "pathParams": [],
  "queryParams": [],
  "bodyParams": [
    {"name": "projectId", "type": "integer", "required": true, "array": false},
    {"name": "summary",   "type": "string",  "required": true, "array": false},
    ...
  ],
  "responseStatus": 201
}
```

### Step 4: Add endpoint to openapi.yaml (when implementing)

1. Check `docs/api/openapi-generated.yaml` for the auto-generated path entry
2. Copy the path entry to `docs/api/openapi.yaml`
3. Add proper `$ref` response schemas under `components/schemas`
4. Run `make generate` to regenerate the ogen client

## Common commands

```bash
# Show cache statistics
uv run scripts/backlog-api-sync.py status

# Check what has changed (reads local sitemap.xml, no network)
uv run scripts/backlog-api-sync.py check

# Fetch a single page
uv run scripts/backlog-api-sync.py fetch --url https://developer.nulab.com/ja/docs/backlog/api/2/get-space/

# Regenerate openapi-generated.yaml from existing cache (no network)
uv run scripts/backlog-api-sync.py generate
```

## How endpoint discovery works

The script fetches `https://developer.nulab.com/ja/docs/backlog/` (1 request) and
extracts all API links from the `#apiNavigation` sidebar — no sitemap required.

## Notes

- The script respects a 0.3s delay between requests to avoid hammering the docs server.
- `openapi-generated.yaml` is **not** used by `make generate` — it's a reference document.
- The `openapi.yaml` paths section should be maintained manually based on what the project actually implements.
- Cache entries include `etag` — conditional GET (`If-None-Match`) returns 304 when unchanged, skipping HTML parsing entirely.
