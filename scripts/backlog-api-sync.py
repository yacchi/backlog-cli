#!/usr/bin/env -S uv run
# /// script
# requires-python = ">=3.11"
# dependencies = [
#     "httpx",
#     "beautifulsoup4",
#     "lxml",
#     "pyyaml",
# ]
# ///
"""
Backlog API documentation crawler and OpenAPI spec updater.

Endpoint discovery: fetches https://developer.nulab.com/ja/docs/backlog/
and extracts all API links from the sidebar navigation (#apiNavigation).
No sitemap required.

Change detection: uses HTTP ETag / If-None-Match for conditional GET.
304 Not Modified responses skip parsing, keeping network cost low.

Usage:
  uv run scripts/backlog-api-sync.py sync      # discover + fetch changed + report
  uv run scripts/backlog-api-sync.py check     # discover + show new/removed (fast)
  uv run scripts/backlog-api-sync.py fetch     # fetch changed pages only
  uv run scripts/backlog-api-sync.py fetch --all  # re-fetch all pages unconditionally
  uv run scripts/backlog-api-sync.py generate  # write docs/api/openapi-generated.yaml
  uv run scripts/backlog-api-sync.py status    # show cache stats
"""

import sys
import json
import re
import time
import argparse
from pathlib import Path
from datetime import datetime, timezone
from typing import Optional

import httpx
from bs4 import BeautifulSoup, Tag
import yaml

# ─── Constants ───────────────────────────────────────────────────────────────

PROJECT_ROOT = Path(__file__).parent.parent
CACHE_FILE = PROJECT_ROOT / "docs" / "api" / "cache.json"
OPENAPI_FILE = PROJECT_ROOT / "docs" / "api" / "openapi.yaml"
GENERATED_FILE = PROJECT_ROOT / "docs" / "api" / "openapi-generated.yaml"

DOCS_BASE = "https://developer.nulab.com"
INDEX_URL = f"{DOCS_BASE}/ja/docs/backlog/"
API_BASE_PREFIX = "/api/v2"

TYPE_MAP = {
    "int": "integer",
    "integer": "integer",
    "string": "string",
    "str": "string",
    "boolean": "boolean",
    "bool": "boolean",
    "float": "number",
    "double": "number",
    "file": "string",
    "date": "string",
}

UA = "backlog-cli/api-sync (https://github.com/fujie/backlog-cli)"


# ─── HTTP ────────────────────────────────────────────────────────────────────

def _get(url: str, headers: dict | None = None, delay: float = 0.3) -> httpx.Response | None:
    time.sleep(delay)
    try:
        resp = httpx.get(
            url, timeout=30, follow_redirects=True,
            headers={"User-Agent": UA, **(headers or {})},
        )
        return resp
    except Exception as e:
        print(f"  ERROR {url}: {e}", file=sys.stderr)
        return None


def fetch_html(url: str, delay: float = 0.3) -> tuple[Optional[str], Optional[str]]:
    """Unconditional GET. Returns (html, etag) or (None, None)."""
    resp = _get(url, delay=delay)
    if resp is None or not resp.is_success:
        return None, None
    return resp.text, resp.headers.get("etag")


def fetch_html_conditional(url: str, etag: str, delay: float = 0.3) -> tuple[Optional[str], Optional[str]]:
    """Conditional GET using If-None-Match.
    Returns (html, new_etag) on 200, (None, cached_etag) on 304, (None, None) on error.
    """
    resp = _get(url, headers={"If-None-Match": etag}, delay=delay)
    if resp is None:
        return None, None
    if resp.status_code == 304:
        return None, etag  # not modified
    if resp.is_success:
        return resp.text, resp.headers.get("etag", etag)
    return None, None


# ─── Discovery ───────────────────────────────────────────────────────────────

def discover_endpoints() -> list[str]:
    """Fetch the Backlog API index page and extract all endpoint URLs from nav."""
    print(f"Discovering endpoints from {INDEX_URL} …", end=" ", flush=True)
    html, _ = fetch_html(INDEX_URL, delay=0)
    if not html:
        sys.exit("ERROR: failed to fetch API index page")

    soup = BeautifulSoup(html, "lxml")
    nav = soup.find("ul", id="apiNavigation")
    if not nav:
        sys.exit("ERROR: #apiNavigation not found — page structure may have changed")

    urls = []
    for a in nav.find_all("a", href=True):
        href = a["href"]
        if "/api/2/" in href:
            full = DOCS_BASE + href
            if not full.endswith("/"):
                full += "/"
            urls.append(full)

    print(f"{len(urls)} endpoints found")
    return urls


# ─── HTML parsing ────────────────────────────────────────────────────────────

def _find_section_table(h2: Tag) -> Optional[Tag]:
    """Return the first <table> within this h2 section (stops at next h2)."""
    node = h2.next_sibling
    while node:
        if hasattr(node, "name"):
            if node.name == "table":
                return node
            if node.name == "h2":
                return None
        node = node.next_sibling
    return None


def _parse_type(raw: str) -> str:
    return TYPE_MAP.get(raw.lower().strip(), "string")


def _parse_param_table(table: Tag) -> list[dict]:
    params = []
    rows = table.find_all("tr")
    if len(rows) < 2:
        return params

    for row in rows[1:]:
        cells = row.find_all("td")
        if not cells:
            continue

        name_raw = cells[0].get_text(separator="\n").strip()
        required = "必須" in name_raw
        is_array = "[]" in name_raw

        name = re.sub(r"（[^）]*）", "", name_raw)
        name = re.sub(r"\[.*?\]", "", name)
        name = re.sub(r"\s+", "", name).strip()
        if not name:
            continue

        raw_type = cells[1].get_text().strip() if len(cells) > 1 else "string"
        desc = cells[-1].get_text(separator=" ").strip() if len(cells) > 2 else ""

        params.append({
            "name": name,
            "type": _parse_type(raw_type),
            "required": required,
            "array": is_array,
            "description": desc,
        })
    return params


def parse_page(html: str) -> Optional[dict]:
    """Extract API metadata from a Backlog developer docs page."""
    soup = BeautifulSoup(html, "lxml")

    markdown_div = soup.find("div", class_="markdown")
    if not markdown_div:
        return None
    content = markdown_div.find("div") or markdown_div

    h1 = content.find("h1")
    title = h1.get_text().strip() if h1 else ""

    method, api_path = "GET", ""
    first_pre = content.find("pre")
    if first_pre:
        code = first_pre.find("code")
        if code:
            parts = code.get_text().strip().split(None, 1)
            if len(parts) == 2 and parts[0] in ("GET", "POST", "PATCH", "PUT", "DELETE"):
                method = parts[0]
                raw_path = re.sub(r":([a-zA-Z][a-zA-Z0-9]*)", r"{\1}", parts[1])
                api_path = raw_path[len(API_BASE_PREFIX):] if raw_path.startswith(API_BASE_PREFIX) else raw_path

    description = ""
    if first_pre:
        for sib in first_pre.next_siblings:
            if hasattr(sib, "name") and sib.name == "p":
                description = sib.get_text(separator=" ").strip()
                break

    path_params, query_params, body_params = [], [], []

    for h2 in content.find_all("h2"):
        h2_text = h2.get_text().strip()
        table = _find_section_table(h2)
        if not table:
            continue
        params = _parse_param_table(table)
        if "URL" in h2_text and "パラメーター" in h2_text:
            path_params = params
        elif "クエリ" in h2_text:
            query_params = params
        elif "リクエスト" in h2_text and "パラメーター" in h2_text:
            if method in ("POST", "PATCH", "PUT"):
                body_params = params
            else:
                query_params = params

    # Derive path params from URL template if table was empty
    url_param_names = re.findall(r"\{([^}]+)\}", api_path)
    if url_param_names and not path_params:
        path_params = [
            {"name": n, "type": "string", "required": True, "array": False, "description": ""}
            for n in url_param_names
        ]

    # Path params are always required per OpenAPI spec
    for p in path_params:
        p["required"] = True

    response_status = 200
    for h3 in content.find_all("h3"):
        if "ステータスライン" in h3.get_text():
            pre = h3.find_next_sibling("pre")
            if pre:
                code = pre.find("code")
                if code:
                    m = re.search(r"HTTP/\S+\s+(\d+)", code.get_text())
                    if m:
                        response_status = int(m.group(1))
            break

    return {
        "title": title,
        "method": method,
        "path": api_path,
        "description": description,
        "pathParams": path_params,
        "queryParams": query_params,
        "bodyParams": body_params,
        "responseStatus": response_status,
    }


# ─── Cache ───────────────────────────────────────────────────────────────────

def load_cache() -> dict:
    if CACHE_FILE.exists():
        return json.loads(CACHE_FILE.read_text(encoding="utf-8"))
    return {"_meta": {"generatedAt": None, "totalEndpoints": 0}, "endpoints": {}}


def save_cache(cache: dict):
    CACHE_FILE.parent.mkdir(parents=True, exist_ok=True)
    endpoints = cache.get("endpoints", {})
    cache["_meta"]["generatedAt"] = datetime.now(timezone.utc).isoformat()
    cache["_meta"]["totalEndpoints"] = len(endpoints)
    CACHE_FILE.write_text(json.dumps(cache, ensure_ascii=False, indent=2), encoding="utf-8")


# ─── OpenAPI generation ──────────────────────────────────────────────────────

def _slug_to_camel(slug: str) -> str:
    parts = slug.split("-")
    return parts[0] + "".join(p.capitalize() for p in parts[1:])


def _param_schema(p: dict) -> dict:
    if p.get("array"):
        return {"type": "array", "items": {"type": p["type"]}}
    return {"type": p["type"]}


def _build_operation(data: dict, url: str) -> dict:
    slug = url.rstrip("/").rsplit("/", 1)[-1]
    op: dict = {"operationId": _slug_to_camel(slug), "summary": data.get("title", "")}
    if data.get("description"):
        op["description"] = data["description"]

    params = []
    for p in data.get("pathParams", []):
        entry: dict = {"name": p["name"], "in": "path", "required": True, "schema": _param_schema(p)}
        if p.get("description"):
            entry["description"] = p["description"][:120]
        params.append(entry)
    for p in data.get("queryParams", []):
        qname = p["name"] + ("[]" if p.get("array") else "")
        entry = {"name": qname, "in": "query", "schema": _param_schema(p)}
        if p.get("required"):
            entry["required"] = True
        if p.get("description"):
            entry["description"] = p["description"][:120]
        params.append(entry)
    if params:
        op["parameters"] = params

    method = data.get("method", "GET").lower()
    if method in ("post", "patch", "put") and data.get("bodyParams"):
        props, required_props = {}, []
        for p in data["bodyParams"]:
            schema = _param_schema(p)
            if p.get("description"):
                schema["description"] = p["description"][:120]
            props[p["name"]] = schema
            if p.get("required"):
                required_props.append(p["name"])
        body_schema: dict = {"type": "object", "properties": props}
        if required_props:
            body_schema["required"] = required_props
        op["requestBody"] = {"content": {"application/x-www-form-urlencoded": {"schema": body_schema}}}

    status = str(data.get("responseStatus", 200))
    op["responses"] = {status: {"description": "Successful response",
                                "content": {"application/json": {"schema": {}}}}}
    return op


def generate_openapi(cache: dict) -> dict:
    base: dict = {}
    if OPENAPI_FILE.exists():
        with open(OPENAPI_FILE, encoding="utf-8") as f:
            base = yaml.safe_load(f) or {}

    paths: dict = {}
    for url, data in sorted(cache.get("endpoints", {}).items(), key=lambda x: x[1].get("path", "")):
        api_path = data.get("path", "")
        if not api_path:
            continue
        method = data.get("method", "GET").lower()
        if api_path not in paths:
            paths[api_path] = {}
        paths[api_path][method] = _build_operation(data, url)

    return {
        "openapi": base.get("openapi", "3.0.3"),
        "info": base.get("info", {"title": "Backlog API", "version": "2.0.0"}),
        "servers": base.get("servers", []),
        "components": base.get("components", {}),
        "security": base.get("security", []),
        "paths": paths,
    }


# ─── Commands ────────────────────────────────────────────────────────────────

def cmd_status(_args):
    cache = load_cache()
    meta = cache.get("_meta", {})
    endpoints = cache.get("endpoints", {})
    print(f"Cache:    {CACHE_FILE}")
    print(f"Entries:  {meta.get('totalEndpoints', len(endpoints))}")
    print(f"Updated:  {meta.get('generatedAt') or '(never)'}")
    if endpoints:
        methods: dict = {}
        for e in endpoints.values():
            m = e.get("method", "?")
            methods[m] = methods.get(m, 0) + 1
        for m, count in sorted(methods.items()):
            print(f"  {m}: {count}")


def cmd_check(_args):
    urls = discover_endpoints()
    cache = load_cache()
    cached = cache.get("endpoints", {})

    url_set = set(urls)
    new_urls = [u for u in urls if u not in cached]
    removed_urls = [u for u in cached if u not in url_set]

    print(f"Discovered: {len(urls)}  |  Cached: {len(cached)}  |  "
          f"New: {len(new_urls)}  |  Removed: {len(removed_urls)}")

    for u in new_urls:
        print(f"  NEW     {u.rstrip('/').rsplit('/', 1)[-1]}")
    for u in removed_urls:
        print(f"  REMOVED {u.rstrip('/').rsplit('/', 1)[-1]}")

    if not new_urls and not removed_urls:
        print("Endpoint list is up to date. Run 'sync' to check for content changes.")


def _fetch_and_update(urls: list[str], cache: dict, force: bool = False) -> tuple[int, int, int]:
    """Fetch pages and update cache. Returns (fetched, skipped_304, errors)."""
    endpoints = cache.setdefault("endpoints", {})
    fetched = skipped = errors = 0
    total = len(urls)

    for i, url in enumerate(urls, 1):
        slug = url.rstrip("/").rsplit("/", 1)[-1]
        print(f"  [{i:3d}/{total}] {slug:<46}", end="", flush=True)

        cached_entry = endpoints.get(url, {})
        cached_etag = cached_entry.get("etag") if not force else None

        if cached_etag:
            html, etag = fetch_html_conditional(url, cached_etag)
            if html is None and etag:  # 304 Not Modified
                print("304 (unchanged)")
                skipped += 1
                continue
        else:
            html, etag = fetch_html(url)

        if html is None:
            print("FAILED")
            errors += 1
            continue

        parsed = parse_page(html)
        if parsed is None:
            print("PARSE_ERROR")
            errors += 1
            continue

        endpoints[url] = {
            "etag": etag,
            "crawledAt": datetime.now(timezone.utc).isoformat(),
            **parsed,
        }
        print(f"{parsed['method']:<7} {parsed['path']}")
        fetched += 1

    return fetched, skipped, errors


def cmd_fetch(args):
    cache = load_cache()

    if getattr(args, "url", None):
        urls = [args.url]
        force = True
    elif getattr(args, "all", False):
        urls = discover_endpoints()
        force = True
    else:
        discovered = discover_endpoints()
        cached = cache.get("endpoints", {})
        url_set = set(discovered)
        # New URLs + existing URLs (conditional GET handles change detection)
        urls = discovered
        force = False

    if not urls:
        print("Nothing to fetch.")
        return

    print(f"Fetching {len(urls)} pages (ETag conditional)…")
    fetched, skipped, errors = _fetch_and_update(urls, cache, force=force)

    # Remove endpoints no longer in discovery (only when doing full sync)
    if not getattr(args, "url", None):
        discovered_set = set(urls)
        removed = [u for u in list(cache.get("endpoints", {}).keys()) if u not in discovered_set]
        for u in removed:
            cache["endpoints"].pop(u, None)
        if removed:
            print(f"Removed {len(removed)} stale endpoints from cache")

    save_cache(cache)
    print(f"\n✓ {fetched} updated  ⏩ {skipped} unchanged  ✗ {errors} errors")
    print(f"Cache: {CACHE_FILE}")


def cmd_generate(args):
    cache = load_cache()
    if not cache.get("endpoints"):
        sys.exit("Cache is empty. Run 'fetch' first.")

    doc = generate_openapi(cache)

    existing_paths: set = set()
    if OPENAPI_FILE.exists():
        with open(OPENAPI_FILE, encoding="utf-8") as f:
            existing = yaml.safe_load(f) or {}
        existing_paths = set((existing.get("paths") or {}).keys())

    new_paths = [p for p in doc["paths"] if p not in existing_paths]

    out = Path(getattr(args, "output", None) or GENERATED_FILE)
    out.parent.mkdir(parents=True, exist_ok=True)
    with open(out, "w", encoding="utf-8") as f:
        yaml.dump(doc, f, allow_unicode=True, sort_keys=False, default_flow_style=False)

    print(f"Generated: {out}")
    print(f"Total paths: {len(doc['paths'])}  |  New (not in openapi.yaml): {len(new_paths)}")
    if new_paths:
        print("Paths missing from openapi.yaml:")
        for p in sorted(new_paths):
            method = next(iter(doc["paths"][p]))
            print(f"  {method.upper():<7} {p}")


def cmd_sync(args):
    urls = discover_endpoints()
    cache = load_cache()
    cached = cache.get("endpoints", {})

    url_set = set(urls)
    new_count = sum(1 for u in urls if u not in cached)
    removed = [u for u in cached if u not in url_set]
    print(f"New: {new_count}  |  Removed: {len(removed)}")

    force = getattr(args, "force", False)
    print(f"Fetching {len(urls)} pages (ETag conditional, force={force})…")
    fetched, skipped, errors = _fetch_and_update(urls, cache, force=force)

    for u in removed:
        cache["endpoints"].pop(u, None)
    if removed:
        print(f"Removed {len(removed)} stale endpoints")

    save_cache(cache)
    print(f"\n✓ {fetched} updated  ⏩ {skipped} unchanged  ✗ {errors} errors")

    print("\n=== Generating openapi-generated.yaml ===")
    cmd_generate(args)


# ─── Entry point ─────────────────────────────────────────────────────────────

def main():
    parser = argparse.ArgumentParser(
        description="Sync Backlog API docs to local cache (no sitemap required)",
        formatter_class=argparse.RawDescriptionHelpFormatter,
    )
    sub = parser.add_subparsers(dest="command", metavar="COMMAND")

    sub.add_parser("status", help="Show cache statistics")
    sub.add_parser("check", help="Discover endpoints and show new/removed vs cache")

    p_fetch = sub.add_parser("fetch", help="Fetch pages and update cache")
    p_fetch.add_argument("--all", action="store_true", help="Re-fetch all (ignore ETag)")
    p_fetch.add_argument("--url", metavar="URL", help="Fetch a single URL")

    p_gen = sub.add_parser("generate", help="Write docs/api/openapi-generated.yaml")
    p_gen.add_argument("--output", metavar="FILE")

    p_sync = sub.add_parser("sync", help="Full sync: discover + fetch changed + generate")
    p_sync.add_argument("--force", action="store_true", help="Ignore ETag, re-fetch all")

    args = parser.parse_args()
    dispatch = {
        "status": cmd_status,
        "check": cmd_check,
        "fetch": cmd_fetch,
        "generate": cmd_generate,
        "sync": cmd_sync,
    }
    fn = dispatch.get(args.command)
    if fn:
        fn(args)
    else:
        parser.print_help()


if __name__ == "__main__":
    main()
