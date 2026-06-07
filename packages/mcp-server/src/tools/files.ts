import { mkdtempSync, writeFileSync, rmSync } from "node:fs";
import { join } from "node:path";
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
