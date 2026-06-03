import { describe, it, expect } from "vitest";
import { checkCliAccess } from "./cli-access.js";

describe("checkCliAccess", () => {
    const access = {
        allow: ["issue *", "project *", "wiki *", "api *"],
        deny: ["* --delete", "auth *", "config *"],
    };

    it("allows matching commands", () => {
        expect(checkCliAccess("issue list --project PROJ", access)).toBe(true);
        expect(checkCliAccess("project list --json id,name", access)).toBe(true);
        expect(checkCliAccess("wiki view 123", access)).toBe(true);
        expect(checkCliAccess("api /api/v2/space", access)).toBe(true);
    });

    it("denies matching deny patterns", () => {
        expect(checkCliAccess("issue list --delete", access)).toBe(false);
        expect(checkCliAccess("auth login", access)).toBe(false);
        expect(checkCliAccess("config set key val", access)).toBe(false);
    });

    it("denies unmatched commands", () => {
        expect(checkCliAccess("notification list", access)).toBe(false);
    });

    it("deny takes precedence over allow", () => {
        expect(checkCliAccess("auth list", access)).toBe(false);
    });

    it("wildcard-only allow permits everything", () => {
        const permissive = { allow: ["*"], deny: [] };
        expect(checkCliAccess("anything goes here", permissive)).toBe(true);
    });
});
