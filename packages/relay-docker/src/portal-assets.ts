/**
 * Portal assets loader for local development.
 */

import { readFile, readdir, stat } from "node:fs/promises";
import { join, extname } from "node:path";
import type { PortalAssets } from "@backlog-cli/relay-core";

/**
 * MIME type mapping for common file extensions.
 */
const MIME_TYPES: Record<string, string> = {
  ".html": "text/html",
  ".css": "text/css",
  ".js": "application/javascript",
  ".json": "application/json",
  ".png": "image/png",
  ".jpg": "image/jpeg",
  ".jpeg": "image/jpeg",
  ".gif": "image/gif",
  ".svg": "image/svg+xml",
  ".ico": "image/x-icon",
  ".woff": "font/woff",
  ".woff2": "font/woff2",
  ".ttf": "font/ttf",
  ".eot": "application/vnd.ms-fontobject",
};

/**
 * Get MIME type for a file extension.
 */
function getMimeType(filename: string): string {
  const ext = extname(filename).toLowerCase();
  return MIME_TYPES[ext] || "application/octet-stream";
}

/**
 * Recursively read all files from a directory.
 */
async function readDirRecursive(
  dir: string,
  basePath: string = ""
): Promise<Map<string, { content: Uint8Array; contentType: string }>> {
  const assets = new Map<string, { content: Uint8Array; contentType: string }>();

  try {
    const entries = await readdir(dir, { withFileTypes: true });

    for (const entry of entries) {
      const fullPath = join(dir, entry.name);
      const relativePath = basePath ? `${basePath}/${entry.name}` : entry.name;

      if (entry.isDirectory()) {
        const subAssets = await readDirRecursive(fullPath, relativePath);
        for (const [key, value] of subAssets) {
          assets.set(key, value);
        }
      } else if (entry.isFile()) {
        const content = await readFile(fullPath);
        const contentType = getMimeType(entry.name);
        assets.set(relativePath, { content: new Uint8Array(content), contentType });
      }
    }
  } catch {
    // Directory doesn't exist or can't be read
  }

  return assets;
}

/**
 * Load portal assets from the web package's dist directory.
 */
export async function loadPortalAssets(
  webDistPath: string
): Promise<PortalAssets | undefined> {
  try {
    // Check if dist directory exists
    const distStat = await stat(webDistPath).catch(() => null);
    if (!distStat?.isDirectory()) {
      console.warn(`[portal-assets] Web dist directory not found: ${webDistPath}`);
      return undefined;
    }

    // Read index.html
    const indexPath = join(webDistPath, "index.html");
    const indexHtml = await readFile(indexPath, "utf-8").catch(() => null);
    if (!indexHtml) {
      console.warn(`[portal-assets] index.html not found in: ${webDistPath}`);
      return undefined;
    }

    // Read all assets from the assets subdirectory
    const assetsDir = join(webDistPath, "assets");
    const assets = await readDirRecursive(assetsDir);

    console.log(`[portal-assets] Loaded ${assets.size} assets from ${webDistPath}`);

    return {
      indexHtml,
      assets,
    };
  } catch (err) {
    console.error("[portal-assets] Failed to load portal assets:", err);
    return undefined;
  }
}
