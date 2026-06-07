import { describe, it, expect } from "vitest";
import { checkCliAccess, isReadOnlyCommand } from "./cli-access.js";

describe("checkCliAccess (deprecated — access control moved to CLI)", () => {
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

describe("isReadOnlyCommand (deprecated — access control moved to CLI)", () => {
    it("allows list and view subcommands", () => {
        expect(isReadOnlyCommand("issue list --project PROJ")).toBe(true);
        expect(isReadOnlyCommand("issue view PROJ-42 --json")).toBe(true);
        expect(isReadOnlyCommand("project list")).toBe(true);
        expect(isReadOnlyCommand("wiki view 123")).toBe(true);
        expect(isReadOnlyCommand("notification list -L 10")).toBe(true);
        expect(isReadOnlyCommand("activity list --project PROJ")).toBe(true);
    });

    it("allows standalone read-only commands", () => {
        expect(isReadOnlyCommand("whoami")).toBe(true);
        expect(isReadOnlyCommand("whoami --json")).toBe(true);
    });

    it("rejects create, edit, delete subcommands", () => {
        expect(isReadOnlyCommand("issue create --project PROJ -t test")).toBe(false);
        expect(isReadOnlyCommand("issue edit PROJ-42 --status 2")).toBe(false);
        expect(isReadOnlyCommand("issue delete PROJ-42")).toBe(false);
        expect(isReadOnlyCommand("wiki create --project PROJ --name test")).toBe(false);
    });

    it("allows api command without -X (defaults to GET)", () => {
        expect(isReadOnlyCommand("api /api/v2/space")).toBe(true);
        expect(isReadOnlyCommand("api /api/v2/issues/count -f projectId[]=123")).toBe(true);
    });

    it("allows api command with GET method", () => {
        expect(isReadOnlyCommand("api /api/v2/space -X GET")).toBe(true);
    });

    it("rejects api command with write methods", () => {
        expect(isReadOnlyCommand("api /api/v2/issues -X POST -f summary=test")).toBe(false);
        expect(isReadOnlyCommand("api /api/v2/issues/1 -X PUT -f summary=test")).toBe(false);
        expect(isReadOnlyCommand("api /api/v2/issues/1 -X DELETE")).toBe(false);
        expect(isReadOnlyCommand("api /api/v2/issues/1 -X PATCH -f summary=test")).toBe(false);
    });
});
