import { describe, it, expect } from "vitest";
import { createMcpApp } from "../index.js";
import { loadSigningKeys, signToken } from "../crypto/jwt.js";
import { generateKeyPair, exportJWK } from "jose";
import type { McpServerConfig } from "../config/schema.js";
import type { TokenPayload } from "../crypto/jwt.js";

let testJwksJson: string;
let testKid: string;

async function initTestKeys() {
    if (testJwksJson) return;
    const { publicKey, privateKey } = await generateKeyPair("EdDSA", { crv: "Ed25519", extractable: true });
    const privJwk = await exportJWK(privateKey);
    testKid = "test-key-1";
    const jwks = { keys: [{ ...privJwk, kid: testKid, kty: "OKP", crv: "Ed25519" }] };
    testJwksJson = JSON.stringify(jwks);
}

function makeConfig(): McpServerConfig {
    return {
        base_url: "https://mcp.example.com",
        relay_url: "https://relay.example.com",
        jwks: testJwksJson,
        backlog_app: { client_id: "test-client-id" },
        spaces: [
            { pattern: "mycompany\\.backlog\\.jp", writable: true },
        ],
        default_spaces: ["mycompany.backlog.jp"],
    };
}

async function makeAccessToken(): Promise<string> {
    const keys = await loadSigningKeys(testJwksJson);
    const now = Math.floor(Date.now() / 1000);
    return signToken(
        {
            bl_access_token: "test-backlog-token",
            bl_expires_at: now + 3600,
            space: "mycompany",
            domain: "backlog.jp",
            iat: now,
            exp: now + 3600,
        },
        keys.signingKey,
        keys.signingKid,
    );
}

async function jsonRpcRequest(
    app: Awaited<ReturnType<typeof createMcpApp>>,
    method: string,
    params?: Record<string, unknown>,
    token?: string,
) {
    const accessToken = token ?? (await makeAccessToken());
    const res = await app.request("/mcp", {
        method: "POST",
        headers: {
            "Content-Type": "application/json",
            Authorization: `Bearer ${accessToken}`,
        },
        body: JSON.stringify({
            jsonrpc: "2.0",
            id: 1,
            method,
            params,
        }),
    });
    return res.json();
}

describe("MCP transport — initialize", () => {
    it("returns server info and capabilities", async () => {
        await initTestKeys();
        const app = await createMcpApp({ config: makeConfig() });
        const res = await jsonRpcRequest(app, "initialize", {
            protocolVersion: "2025-03-26",
            capabilities: {},
            clientInfo: { name: "test", version: "1.0" },
        });
        expect(res.result.protocolVersion).toBe("2025-03-26");
        expect(res.result.serverInfo.name).toBe("backlog-mcp-server");
        expect(res.result.capabilities.tools).toBeDefined();
        expect(res.result.capabilities.prompts).toBeDefined();
    });

    it("includes instructions in initialize", async () => {
        await initTestKeys();
        const app = await createMcpApp({ config: makeConfig() });
        const res = await jsonRpcRequest(app, "initialize", {
            protocolVersion: "2025-03-26",
            capabilities: {},
            clientInfo: { name: "test", version: "1.0" },
        });
        expect(res.result.instructions).toBeTypeOf("string");
        expect(res.result.instructions).toContain("Prefer local CLI");
    });
});

describe("MCP transport — tools/list", () => {
    it("lists query and mutation tools (no script tools without sandbox)", async () => {
        await initTestKeys();
        const app = await createMcpApp({ config: makeConfig() });
        const res = await jsonRpcRequest(app, "tools/list");
        const tools = res.result.tools;
        const names = tools.map((t: { name: string }) => t.name);
        expect(names).toContain("backlog_help");
        expect(names).toContain("who");
        expect(names).toContain("backlog_query");
        expect(names).toContain("backlog_mutate");
        expect(names).not.toContain("backlog_query_script");
        expect(names).not.toContain("backlog_mutate_script");
    });
});

describe("MCP transport — auth", () => {
    it("rejects request without Bearer token", async () => {
        await initTestKeys();
        const app = await createMcpApp({ config: makeConfig() });
        const res = await app.request("/mcp", {
            method: "POST",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify({
                jsonrpc: "2.0",
                id: 1,
                method: "ping",
            }),
        });
        expect(res.status).toBe(401);
    });

    it("rejects expired token", async () => {
        await initTestKeys();
        const app = await createMcpApp({ config: makeConfig() });
        const keys = await loadSigningKeys(testJwksJson);
        const now = Math.floor(Date.now() / 1000);
        const expired = await signToken(
            {
                bl_access_token: "expired",
                space: "mycompany",
                domain: "backlog.jp",
                iat: now - 7200,
                exp: now - 3600,
            },
            keys.signingKey,
            keys.signingKid,
        );
        const res = await app.request("/mcp", {
            method: "POST",
            headers: {
                "Content-Type": "application/json",
                Authorization: `Bearer ${expired}`,
            },
            body: JSON.stringify({
                jsonrpc: "2.0",
                id: 1,
                method: "ping",
            }),
        });
        expect(res.status).toBe(401);
    });
});

describe("MCP transport — tools/call backlog", () => {
    it("rejects missing args", async () => {
        await initTestKeys();
        const app = await createMcpApp({ config: makeConfig() });
        const res = await jsonRpcRequest(app, "tools/call", {
            name: "backlog",
            arguments: {},
        });
        expect(res.error).toBeDefined();
        expect(res.error.code).toBe(-32602);
    });

    it("rejects unknown tool", async () => {
        await initTestKeys();
        const app = await createMcpApp({ config: makeConfig() });
        const res = await jsonRpcRequest(app, "tools/call", {
            name: "nonexistent",
            arguments: {},
        });
        expect(res.error).toBeDefined();
    });
});

describe("MCP transport — prompts", () => {
    it("lists prompts", async () => {
        await initTestKeys();
        const app = await createMcpApp({ config: makeConfig() });
        const res = await jsonRpcRequest(app, "prompts/list");
        expect(res.result.prompts).toHaveLength(1);
        expect(res.result.prompts[0].name).toBe("backlog-cli-reference");
    });

    it("gets prompt content", async () => {
        await initTestKeys();
        const app = await createMcpApp({ config: makeConfig() });
        const res = await jsonRpcRequest(app, "prompts/get", {
            name: "backlog-cli-reference",
        });
        expect(res.result.messages).toHaveLength(1);
        expect(res.result.messages[0].content.text).toContain("Backlog CLI Reference");
        expect(res.result.messages[0].content.text).toContain("mycompany");
    });

    it("rejects unknown prompt", async () => {
        await initTestKeys();
        const app = await createMcpApp({ config: makeConfig() });
        const res = await jsonRpcRequest(app, "prompts/get", {
            name: "nonexistent",
        });
        expect(res.error).toBeDefined();
    });
});

describe("MCP transport — tools/call backlog_help", () => {
    it("returns full CLI reference without command arg", async () => {
        await initTestKeys();
        const app = await createMcpApp({ config: makeConfig() });
        const res = await jsonRpcRequest(app, "tools/call", {
            name: "backlog_help",
            arguments: {},
        });
        expect(res.result.content[0].text).toContain("Backlog CLI Reference");
        expect(res.result.content[0].text).toContain("mycompany");
        expect(res.result.content[0].text).toContain("backlog.jp");
    });

    it("returns filtered section for specific command", async () => {
        await initTestKeys();
        const app = await createMcpApp({ config: makeConfig() });
        const res = await jsonRpcRequest(app, "tools/call", {
            name: "backlog_help",
            arguments: { command: "issue" },
        });
        expect(res.result.content[0].text).toContain("issue");
        expect(res.result.isError).toBeUndefined();
    });

    it("falls back to full reference for unknown command", async () => {
        await initTestKeys();
        const app = await createMcpApp({ config: makeConfig() });
        const res = await jsonRpcRequest(app, "tools/call", {
            name: "backlog_help",
            arguments: { command: "nonexistent_command" },
        });
        expect(res.result.content[0].text).toContain("Backlog CLI Reference");
    });
});

describe("MCP transport — ping", () => {
    it("responds to ping", async () => {
        await initTestKeys();
        const app = await createMcpApp({ config: makeConfig() });
        const res = await jsonRpcRequest(app, "ping");
        expect(res.result).toEqual({});
    });
});

describe("MCP transport — space access control", () => {
    it("rejects requests from disallowed primary space", async () => {
        await initTestKeys();
        const app = await createMcpApp({ config: makeConfig() });
        const keys = await loadSigningKeys(testJwksJson);
        const now = Math.floor(Date.now() / 1000);
        const token = await signToken(
            {
                bl_access_token: "token-for-unknown",
                bl_expires_at: now + 3600,
                space: "unknown",
                domain: "backlog.jp",
                iat: now,
                exp: now + 3600,
            },
            keys.signingKey,
            keys.signingKid,
        );
        const res = await jsonRpcRequest(app, "initialize", {
            protocolVersion: "2025-03-26",
            capabilities: {},
            clientInfo: { name: "test", version: "1.0" },
        }, token);
        expect(res.error).toBeDefined();
        expect(res.error.message).toContain("unknown.backlog.jp");
    });

    it("rejects tool calls targeting disallowed space", async () => {
        await initTestKeys();
        const app = await createMcpApp({ config: makeConfig() });
        const keys = await loadSigningKeys(testJwksJson);
        const now = Math.floor(Date.now() / 1000);
        const token = await signToken(
            {
                bl_access_token: "primary-token",
                bl_expires_at: now + 3600,
                space: "mycompany",
                domain: "backlog.jp",
                spaces: [
                    { space: "mycompany", domain: "backlog.jp", bl_access_token: "primary-token", bl_refresh_token: "r1", bl_expires_at: now + 3600 },
                    { space: "rogue", domain: "backlog.jp", bl_access_token: "rogue-token", bl_refresh_token: "r2", bl_expires_at: now + 3600 },
                ],
                iat: now,
                exp: now + 3600,
            },
            keys.signingKey,
            keys.signingKid,
        );
        const res = await jsonRpcRequest(app, "tools/call", {
            name: "backlog_query",
            arguments: { args: "issue list -p ALL", space: "rogue.backlog.jp" },
        }, token);
        expect(res.result.isError).toBe(true);
        expect(res.result.content[0].text).toContain("rogue.backlog.jp");
        expect(res.result.content[0].text).toContain("許可されていません");
    });

    it("rejects mutation on read-only space", async () => {
        await initTestKeys();
        const config = makeConfig();
        config.spaces = [
            { pattern: "mycompany\\.backlog\\.jp", writable: false },
        ];
        const app = await createMcpApp({ config });
        const res = await jsonRpcRequest(app, "tools/call", {
            name: "backlog_mutate",
            arguments: { args: "issue create -p PROJ -t test" },
        });
        expect(res.result.isError).toBe(true);
        expect(res.result.content[0].text).toContain("読み取り専用");
    });
});
