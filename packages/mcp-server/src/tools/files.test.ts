import { describe, it, expect, afterEach } from "vitest";
import { readFileSync, existsSync } from "node:fs";
import { gzipSync } from "node:zlib";
import { materializeFiles, substituteFileRefs } from "./files.js";

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
