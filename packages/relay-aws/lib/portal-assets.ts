/**
 * Portal assets loader for embedded SPA serving.
 */

import * as fs from "fs";
import * as path from "path";
import type { PortalAssets } from "@backlog-cli/relay-core";

/**
 * Content type mapping for static assets.
 */
const CONTENT_TYPES: Record<string, string> = {
  ".html": "text/html; charset=utf-8",
  ".js": "application/javascript; charset=utf-8",
  ".mjs": "application/javascript; charset=utf-8",
  ".css": "text/css; charset=utf-8",
  ".json": "application/json; charset=utf-8",
  ".svg": "image/svg+xml",
  ".png": "image/png",
  ".ico": "image/x-icon",
  ".woff": "font/woff",
  ".woff2": "font/woff2",
  ".ttf": "font/ttf",
  ".eot": "application/vnd.ms-fontobject",
};

/**
 * Get content type for a file path.
 */
function getContentType(filePath: string): string {
  const ext = path.extname(filePath).toLowerCase();
  return CONTENT_TYPES[ext] || "application/octet-stream";
}

/**
 * Load portal assets from the embedded web dist directory.
 * Assets are expected to be copied to the Lambda bundle at build time.
 */
export function loadPortalAssets(): PortalAssets | undefined {
  // Assets directory relative to the handler
  const assetsDir = path.join(import.meta.dirname, "web-dist");

  // Check if assets directory exists
  if (!fs.existsSync(assetsDir)) {
    console.log("[portal-assets] Web assets directory not found:", assetsDir);
    return undefined;
  }

  try {
    // Read index.html
    const indexHtmlPath = path.join(assetsDir, "index.html");
    if (!fs.existsSync(indexHtmlPath)) {
      console.log("[portal-assets] index.html not found");
      return undefined;
    }
    const indexHtml = fs.readFileSync(indexHtmlPath, "utf-8");

    // Read static assets from assets/ subdirectory
    const assets = new Map<string, { content: Uint8Array; contentType: string }>();
    const assetsSubDir = path.join(assetsDir, "assets");

    if (fs.existsSync(assetsSubDir)) {
      const files = fs.readdirSync(assetsSubDir);
      for (const file of files) {
        const filePath = path.join(assetsSubDir, file);
        const stat = fs.statSync(filePath);
        if (stat.isFile()) {
          const content = fs.readFileSync(filePath);
          const relativePath = `assets/${file}`;
          assets.set(relativePath, {
            content: new Uint8Array(content),
            contentType: getContentType(file),
          });
        }
      }
    }

    console.log(
      "[portal-assets] Loaded",
      assets.size,
      "static assets from",
      assetsDir
    );

    return { indexHtml, assets };
  } catch (err) {
    console.error("[portal-assets] Failed to load assets:", err);
    return undefined;
  }
}
