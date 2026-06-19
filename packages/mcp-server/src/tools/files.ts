import { mkdtempSync, writeFileSync, readFileSync, readdirSync, statSync, rmSync } from "node:fs";
import { join, extname, relative, basename } from "node:path";
import { tmpdir } from "node:os";
import { gunzipSync } from "node:zlib";
import type { ScriptFile } from "../transport/handlers.js";

export interface MaterializedFiles {
    paths: string[];
    cleanup: () => void;
}

export function materializeFiles(files: ScriptFile[]): MaterializedFiles {
    const dir = mkdtempSync(join(tmpdir(), "mcp-files-"));
    const paths: string[] = [];

    for (let i = 0; i < files.length; i++) {
        const f = files[i];
        const name = sanitizeFilename(f.name) ?? `file-${i}.dat`;
        const path = join(dir, name);
        const buf = decodeFileContent(f.content, f.encoding);
        writeFileSync(path, buf);
        paths.push(path);
    }

    return {
        paths,
        cleanup: () => {
            try { rmSync(dir, { recursive: true, force: true }); } catch { /* best effort */ }
        },
    };
}

const FILE_REF_PATTERN = /\$file\[(\d+)]/g;

export function substituteFileRefs(args: string, filePaths: string[]): string {
    return args.replace(FILE_REF_PATTERN, (match, indexStr) => {
        const idx = parseInt(indexStr, 10);
        if (idx >= 0 && idx < filePaths.length) {
            return filePaths[idx];
        }
        return match;
    });
}

function decodeFileContent(content: string, encoding?: string): Buffer {
    switch (encoding) {
        case "gzip+base64": {
            const compressed = Buffer.from(content, "base64");
            return gunzipSync(compressed);
        }
        case "utf8":
            return Buffer.from(content, "utf8");
        case "base64":
        default:
            return Buffer.from(content, "base64");
    }
}

function sanitizeFilename(name?: string): string | undefined {
    if (!name) return undefined;
    const base = name.replace(/[/\\]/g, "_").replace(/\.\./g, "_");
    return base || undefined;
}

// --- Output file capture ---

export interface OutputDir {
    path: string;
    cleanup: () => void;
}

export function createOutputDir(): OutputDir {
    const path = mkdtempSync(join(tmpdir(), "mcp-output-"));
    return {
        path,
        cleanup: () => {
            try { rmSync(path, { recursive: true, force: true }); } catch { /* best effort */ }
        },
    };
}

export interface CollectedFile {
    name: string;
    path: string;
    data: string;
    mimeType: string;
    size: number;
}

const MIME_TYPES: Record<string, string> = {
    ".png": "image/png",
    ".jpg": "image/jpeg",
    ".jpeg": "image/jpeg",
    ".gif": "image/gif",
    ".webp": "image/webp",
    ".svg": "image/svg+xml",
    ".bmp": "image/bmp",
    ".ico": "image/x-icon",
    ".pdf": "application/pdf",
    ".txt": "text/plain",
    ".csv": "text/csv",
    ".json": "application/json",
    ".xml": "application/xml",
    ".html": "text/html",
    ".md": "text/markdown",
    ".zip": "application/zip",
    ".gz": "application/gzip",
    ".tar": "application/x-tar",
    ".xlsx": "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
    ".xls": "application/vnd.ms-excel",
    ".docx": "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
    ".doc": "application/msword",
    ".pptx": "application/vnd.openxmlformats-officedocument.presentationml.presentation",
    ".ppt": "application/vnd.ms-powerpoint",
};

function getMimeType(filename: string): string {
    const ext = extname(filename).toLowerCase();
    return MIME_TYPES[ext] ?? "application/octet-stream";
}

export function collectOutputFiles(dirPath: string): CollectedFile[] {
    try {
        return walkDir(dirPath, dirPath);
    } catch {
        return [];
    }
}

function walkDir(currentPath: string, rootPath: string): CollectedFile[] {
    const entries = readdirSync(currentPath, { withFileTypes: true });
    const files: CollectedFile[] = [];
    for (const entry of entries) {
        const fullPath = join(currentPath, entry.name);
        if (entry.isDirectory()) {
            files.push(...walkDir(fullPath, rootPath));
        } else if (entry.isFile()) {
            const relPath = relative(rootPath, fullPath);
            const data = readFileSync(fullPath);
            files.push({
                name: basename(fullPath),
                path: `/${relPath}`,
                data: data.toString("base64"),
                mimeType: getMimeType(entry.name),
                size: statSync(fullPath).size,
            });
        }
    }
    return files;
}
