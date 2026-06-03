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

    it("handles escaped characters", () => {
        expect(parseArgs("issue create --summary Hello\\ World")).toEqual([
            "issue", "create", "--summary", "Hello World",
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
