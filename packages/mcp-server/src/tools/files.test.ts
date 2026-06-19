import { describe, it, expect, afterEach } from "vitest";
import { readFileSync, writeFileSync, existsSync } from "node:fs";
import { join } from "node:path";
import { gzipSync } from "node:zlib";
import { materializeFiles, substituteFileRefs, createOutputDir, collectOutputFiles } from "./files.js";

describe("substituteFileRefs", () => {
    it("replaces $file[N] with paths", () => {
        const result = substituteFileRefs(
            'issue create --body-file $file[0] --attach $file[1]',
            ["/tmp/body.md", "/tmp/image.png"],
        );
        expect(result).toBe("issue create --body-file /tmp/body.md --attach /tmp/image.png");
    });

    it("leaves unresolved refs unchanged", () => {
        const result = substituteFileRefs(
            'issue create --body-file $file[5]',
            ["/tmp/body.md"],
        );
        expect(result).toBe("issue create --body-file $file[5]");
    });

    it("handles no refs", () => {
        const result = substituteFileRefs("issue list", ["/tmp/f"]);
        expect(result).toBe("issue list");
    });
});

describe("materializeFiles", () => {
    const cleanups: Array<() => void> = [];
    afterEach(() => {
        cleanups.forEach((fn) => fn());
        cleanups.length = 0;
    });

    it("decodes base64 (default encoding)", () => {
        const content = Buffer.from("hello world").toString("base64");
        const m = materializeFiles([{ content }]);
        cleanups.push(m.cleanup);

        expect(m.paths).toHaveLength(1);
        expect(readFileSync(m.paths[0], "utf8")).toBe("hello world");
    });

    it("decodes utf8 encoding", () => {
        const m = materializeFiles([{ content: "日本語テスト", encoding: "utf8" }]);
        cleanups.push(m.cleanup);

        expect(readFileSync(m.paths[0], "utf8")).toBe("日本語テスト");
    });

    it("decodes gzip+base64 encoding", () => {
        const original = "compressed content test";
        const compressed = gzipSync(Buffer.from(original));
        const content = compressed.toString("base64");

        const m = materializeFiles([{ content, encoding: "gzip+base64" }]);
        cleanups.push(m.cleanup);

        expect(readFileSync(m.paths[0], "utf8")).toBe(original);
    });

    it("uses name hint for filename", () => {
        const content = Buffer.from("data").toString("base64");
        const m = materializeFiles([{ content, name: "report.md" }]);
        cleanups.push(m.cleanup);

        expect(m.paths[0]).toContain("report.md");
    });

    it("sanitizes path traversal in name", () => {
        const content = Buffer.from("data").toString("base64");
        const m = materializeFiles([{ content, name: "../../../etc/passwd" }]);
        cleanups.push(m.cleanup);

        expect(m.paths[0]).not.toContain("..");
    });

    it("cleans up temp files", () => {
        const content = Buffer.from("temp").toString("base64");
        const m = materializeFiles([{ content }]);
        const path = m.paths[0];

        expect(existsSync(path)).toBe(true);
        m.cleanup();
        expect(existsSync(path)).toBe(false);
    });
});

describe("createOutputDir", () => {
    const cleanups: Array<() => void> = [];
    afterEach(() => {
        cleanups.forEach((fn) => fn());
        cleanups.length = 0;
    });

    it("creates a temp directory", () => {
        const dir = createOutputDir();
        cleanups.push(dir.cleanup);
        expect(existsSync(dir.path)).toBe(true);
    });

    it("cleans up on cleanup()", () => {
        const dir = createOutputDir();
        const path = dir.path;
        expect(existsSync(path)).toBe(true);
        dir.cleanup();
        expect(existsSync(path)).toBe(false);
    });
});

describe("collectOutputFiles", () => {
    const cleanups: Array<() => void> = [];
    afterEach(() => {
        cleanups.forEach((fn) => fn());
        cleanups.length = 0;
    });

    it("returns empty array for empty dir", () => {
        const dir = createOutputDir();
        cleanups.push(dir.cleanup);
        expect(collectOutputFiles(dir.path)).toEqual([]);
    });

    it("collects files with correct mime types and paths", () => {
        const dir = createOutputDir();
        cleanups.push(dir.cleanup);

        writeFileSync(join(dir.path, "photo.png"), Buffer.from("PNG data"));
        writeFileSync(join(dir.path, "report.pdf"), Buffer.from("PDF data"));

        const files = collectOutputFiles(dir.path);
        expect(files).toHaveLength(2);

        const png = files.find((f) => f.name === "photo.png");
        expect(png).toBeDefined();
        expect(png!.mimeType).toBe("image/png");
        expect(png!.path).toBe("/photo.png");
        expect(Buffer.from(png!.data, "base64").toString()).toBe("PNG data");

        const pdf = files.find((f) => f.name === "report.pdf");
        expect(pdf).toBeDefined();
        expect(pdf!.mimeType).toBe("application/pdf");
        expect(pdf!.path).toBe("/report.pdf");
    });

    it("walks nested directories", () => {
        const dir = createOutputDir();
        cleanups.push(dir.cleanup);

        const { mkdirSync } = require("node:fs");
        const nested = join(dir.path, "private", "tmp", "scratchpad");
        mkdirSync(nested, { recursive: true });
        writeFileSync(join(nested, "img1.png"), Buffer.from("img1"));
        writeFileSync(join(nested, "img2.png"), Buffer.from("img2"));

        const files = collectOutputFiles(dir.path);
        expect(files).toHaveLength(2);

        const img1 = files.find((f) => f.name === "img1.png");
        expect(img1).toBeDefined();
        expect(img1!.path).toBe("/private/tmp/scratchpad/img1.png");

        const img2 = files.find((f) => f.name === "img2.png");
        expect(img2).toBeDefined();
        expect(img2!.path).toBe("/private/tmp/scratchpad/img2.png");
    });

    it("returns empty array for nonexistent dir", () => {
        expect(collectOutputFiles("/nonexistent/path")).toEqual([]);
    });
});
