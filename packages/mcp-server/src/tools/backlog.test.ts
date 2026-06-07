import { describe, it, expect } from "vitest";
import { parseArgs } from "./backlog.js";

describe("parseArgs", () => {
    it("splits simple args", () => {
        expect(parseArgs("issue list --project PROJ")).toEqual([
            "issue", "list", "--project", "PROJ",
        ]);
    });

    it("handles double quotes", () => {
        expect(parseArgs('issue create --summary "Hello World"')).toEqual([
            "issue", "create", "--summary", "Hello World",
        ]);
    });

    it("handles single quotes", () => {
        expect(parseArgs("issue list --jq '.[] | select(.x)'")).toEqual([
            "issue", "list", "--jq", ".[] | select(.x)",
        ]);
    });

    it("handles escaped characters outside quotes", () => {
        expect(parseArgs("issue create --summary Hello\\ World")).toEqual([
            "issue", "create", "--summary", "Hello World",
        ]);
    });

    it("interprets escape sequences inside double quotes", () => {
        // MCP JSON delivers literal backslash + n as two chars; parseArgs converts to newline
        const input = 'issue create -b "line1\\nline2"';
        expect(parseArgs(input)).toEqual([
            "issue", "create", "-b", "line1\nline2",
        ]);
    });

    it("handles escaped double quote inside double quotes", () => {
        expect(parseArgs('issue create -b "say \\"hello\\""')).toEqual([
            "issue", "create", "-b", 'say "hello"',
        ]);
    });

    it("preserves unknown escapes inside double quotes", () => {
        const input = 'issue create -b "path\\x"';
        expect(parseArgs(input)).toEqual([
            "issue", "create", "-b", "path\\x",
        ]);
    });

    it("preserves all characters inside single quotes", () => {
        expect(parseArgs("issue create -b 'line1\\nline2'")).toEqual([
            "issue", "create", "-b", "line1\\nline2",
        ]);
    });

    it("handles empty input", () => {
        expect(parseArgs("")).toEqual([]);
    });

    it("handles multiple spaces", () => {
        expect(parseArgs("issue   list   --project   PROJ")).toEqual([
            "issue", "list", "--project", "PROJ",
        ]);
    });
});
