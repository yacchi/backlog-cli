import { describe, it, expect } from "vitest";
import { createMcpApp } from "../index.js";
import { loadSigningKeys, signToken } from "../crypto/jwt.js";
import { seal } from "../crypto/secret.js";
import { generateKeyPair, exportJWK } from "jose";
import type { McpServerConfig } from "../config/schema.js";
import { readJson } from "../test-support/http.js";

/**
 * Loose shape of a JSON-RPC response body as read back in these tests.
 * `result` and `error` are both typed as always-present so call sites don't
 * need `!` on every access — at runtime only one of the two is ever set,
 * and each test only ever reads the one that applies to its scenario.
 */
interface JsonRpcTestResponse<T = Record<string, unknown>> {
    result: T;
    error: {
        code: number;
        message: string;
        data: { supported?: string[]; requested?: string };
    };
}

interface InitializeResult {
    protocolVersion: string;
    serverInfo: { name: string };
    capabilities: { tools?: unknown; prompts?: unknown };
    instructions: string;
}

interface ToolsListResult {
    tools: Array<{ name: string }>;
}

interface PromptsListResult {
    prompts: Array<{ name: string }>;
}

interface PromptsGetResult {
    messages: Array<{ content: { text: string } }>;
}

interface ToolCallResult {
    content: Array<{ text: string }>;
    isError?: boolean;
}

interface DiscoverResult {
    resultType: string;
    supportedVersions: string[];
    capabilities: { tools?: unknown };
    instructions: string;
    _meta: Record<string, { name: string }>;
    cacheScope: string;
    ttlMs: number;
}

interface ModernListResult {
    resultType: string;
    ttlMs: number;
    cacheScope: string;
    _meta: Record<string, { name: string }>;
    tools: unknown[];
}

let testJwksJson: string;
let testKid: string;

async function initTestKeys() {
    if (testJwksJson) return;
    const { privateKey } = await generateKeyPair("EdDSA", { crv: "Ed25519", extractable: true });
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

/** Seal a raw Backlog access token the way the server does before issuing it. */
async function sealAt(domain: string, value: string): Promise<string> {
    const keys = await loadSigningKeys(testJwksJson);
    const sealKey = keys.encKeys.get(keys.signingKid)!;
    return seal(value, sealKey, keys.signingKid, domain, "at");
}

async function makeAccessToken(): Promise<string> {
    const keys = await loadSigningKeys(testJwksJson);
    const now = Math.floor(Date.now() / 1000);
    return signToken(
        {
            bl_access_token: await sealAt("mycompany.backlog.jp", "test-backlog-token"),
            bl_expires_at: now + 3600,
            space: "mycompany.backlog.jp",
            iat: now,
            exp: now + 3600,
        },
        keys.signingKey,
        keys.signingKid,
    );
}

async function jsonRpcRequest<T = Record<string, unknown>>(
    app: Awaited<ReturnType<typeof createMcpApp>>,
    method: string,
    params?: Record<string, unknown>,
    token?: string,
): Promise<JsonRpcTestResponse<T>> {
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
    return readJson<JsonRpcTestResponse<T>>(res);
}

describe("MCP transport — initialize", () => {
    it("returns server info and capabilities", async () => {
        await initTestKeys();
        const app = await createMcpApp({ config: makeConfig() });
        const res = await jsonRpcRequest<InitializeResult>(app, "initialize", {
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
        const res = await jsonRpcRequest<InitializeResult>(app, "initialize", {
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
        const res = await jsonRpcRequest<ToolsListResult>(app, "tools/list");
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
                space: "mycompany.backlog.jp",
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
        const res = await jsonRpcRequest<PromptsListResult>(app, "prompts/list");
        expect(res.result.prompts).toHaveLength(1);
        expect(res.result.prompts[0].name).toBe("backlog-cli-reference");
    });

    it("gets prompt content", async () => {
        await initTestKeys();
        const app = await createMcpApp({ config: makeConfig() });
        const res = await jsonRpcRequest<PromptsGetResult>(app, "prompts/get", {
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
        const res = await jsonRpcRequest<ToolCallResult>(app, "tools/call", {
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
        const res = await jsonRpcRequest<ToolCallResult>(app, "tools/call", {
            name: "backlog_help",
            arguments: { command: "issue" },
        });
        expect(res.result.content[0].text).toContain("issue");
        expect(res.result.isError).toBeUndefined();
    });

    it("falls back to full reference for unknown command", async () => {
        await initTestKeys();
        const app = await createMcpApp({ config: makeConfig() });
        const res = await jsonRpcRequest<ToolCallResult>(app, "tools/call", {
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
                space: "unknown.backlog.jp",
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
                bl_access_token: await sealAt("mycompany.backlog.jp", "primary-token"),
                bl_expires_at: now + 3600,
                space: "mycompany.backlog.jp",
                spaces: [
                    { space: "mycompany.backlog.jp", bl_access_token: await sealAt("mycompany.backlog.jp", "primary-token"), bl_refresh_token: "r1", bl_expires_at: now + 3600 },
                    { space: "rogue.backlog.jp", bl_access_token: await sealAt("rogue.backlog.jp", "rogue-token"), bl_refresh_token: "r2", bl_expires_at: now + 3600 },
                ],
                iat: now,
                exp: now + 3600,
            },
            keys.signingKey,
            keys.signingKid,
        );
        const res = await jsonRpcRequest<ToolCallResult>(app, "tools/call", {
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
        const res = await jsonRpcRequest<ToolCallResult>(app, "tools/call", {
            name: "backlog_mutate",
            arguments: { args: "issue create -p PROJ -t test" },
        });
        expect(res.result.isError).toBe(true);
        expect(res.result.content[0].text).toContain("読み取り専用");
    });
});

/** POST /mcp with explicit control over headers and body (protocol-version tests). */
async function rawRequest<T = Record<string, unknown>>(
    app: Awaited<ReturnType<typeof createMcpApp>>,
    body: Record<string, unknown>,
    headers: Record<string, string> = {},
): Promise<{ status: number; body: JsonRpcTestResponse<T> }> {
    const accessToken = await makeAccessToken();
    const res = await app.request("/mcp", {
        method: "POST",
        headers: {
            "Content-Type": "application/json",
            Authorization: `Bearer ${accessToken}`,
            ...headers,
        },
        body: JSON.stringify({ jsonrpc: "2.0", id: 1, ...body }),
    });
    return { status: res.status, body: await readJson<JsonRpcTestResponse<T>>(res) };
}

const MODERN = "2026-07-28";

describe("MCP transport — protocol version negotiation", () => {
    it("echoes the legacy version requested by the client", async () => {
        await initTestKeys();
        const app = await createMcpApp({ config: makeConfig() });
        for (const version of ["2025-03-26", "2025-06-18", "2025-11-25"]) {
            const res = await jsonRpcRequest<InitializeResult>(app, "initialize", {
                protocolVersion: version,
                capabilities: {},
                clientInfo: { name: "test", version: "1.0" },
            });
            expect(res.result.protocolVersion).toBe(version);
        }
    });

    it("falls back to the newest legacy version for an unknown initialize version", async () => {
        await initTestKeys();
        const app = await createMcpApp({ config: makeConfig() });
        const res = await jsonRpcRequest<InitializeResult>(app, "initialize", {
            protocolVersion: "1900-01-01",
            capabilities: {},
            clientInfo: { name: "test", version: "1.0" },
        });
        expect(res.result.protocolVersion).toBe("2025-11-25");
    });

    it("rejects an unsupported protocol version with UnsupportedProtocolVersionError", async () => {
        await initTestKeys();
        const app = await createMcpApp({ config: makeConfig() });
        const { status, body } = await rawRequest(
            app,
            { method: "tools/list", params: {} },
            { "MCP-Protocol-Version": "1900-01-01", "Mcp-Method": "tools/list" },
        );
        expect(status).toBe(400);
        expect(body.error.code).toBe(-32022);
        expect(body.error.data.supported).toContain(MODERN);
        expect(body.error.data.requested).toBe("1900-01-01");
    });

    it("rejects a header/_meta protocol version mismatch with HeaderMismatch", async () => {
        await initTestKeys();
        const app = await createMcpApp({ config: makeConfig() });
        const { status, body } = await rawRequest(
            app,
            {
                method: "tools/list",
                params: { _meta: { "io.modelcontextprotocol/protocolVersion": MODERN } },
            },
            { "MCP-Protocol-Version": "2025-11-25", "Mcp-Method": "tools/list" },
        );
        expect(status).toBe(400);
        expect(body.error.code).toBe(-32020);
    });

    it("rejects an Mcp-Method header that does not match the body", async () => {
        await initTestKeys();
        const app = await createMcpApp({ config: makeConfig() });
        const { status, body } = await rawRequest(
            app,
            { method: "tools/list", params: {} },
            { "MCP-Protocol-Version": MODERN, "Mcp-Method": "prompts/list" },
        );
        expect(status).toBe(400);
        expect(body.error.code).toBe(-32020);
    });

    it("rejects an Mcp-Name header that does not match the body", async () => {
        await initTestKeys();
        const app = await createMcpApp({ config: makeConfig() });
        const { status, body } = await rawRequest(
            app,
            { method: "prompts/get", params: { name: "unknown-prompt" } },
            { "MCP-Protocol-Version": MODERN, "Mcp-Method": "prompts/get", "Mcp-Name": "other" },
        );
        expect(status).toBe(400);
        expect(body.error.code).toBe(-32020);
    });

    it("accepts a base64-encoded Mcp-Name matching the body", async () => {
        await initTestKeys();
        const app = await createMcpApp({ config: makeConfig() });
        const encoded = `=?base64?${btoa("unknown-prompt")}?=`;
        const { status, body } = await rawRequest(
            app,
            { method: "prompts/get", params: { name: "unknown-prompt" } },
            { "MCP-Protocol-Version": MODERN, "Mcp-Method": "prompts/get", "Mcp-Name": encoded },
        );
        // Header validation passed, so the request reached dispatch (which then
        // rejects the unknown prompt name with -32602, not a header error).
        expect(status).toBe(200);
        expect(body.error.code).toBe(-32602);
    });

    it("returns 404 with -32601 for an unknown method on a modern request", async () => {
        await initTestKeys();
        const app = await createMcpApp({ config: makeConfig() });
        const { status, body } = await rawRequest(
            app,
            { method: "resources/list", params: {} },
            { "MCP-Protocol-Version": MODERN, "Mcp-Method": "resources/list" },
        );
        expect(status).toBe(404);
        expect(body.error.code).toBe(-32601);
    });
});

describe("MCP transport — server/discover", () => {
    it("advertises supported versions, capabilities and identity", async () => {
        await initTestKeys();
        const app = await createMcpApp({ config: makeConfig() });
        const { status, body } = await rawRequest<DiscoverResult>(
            app,
            {
                method: "server/discover",
                params: { _meta: { "io.modelcontextprotocol/protocolVersion": MODERN } },
            },
            { "MCP-Protocol-Version": MODERN, "Mcp-Method": "server/discover" },
        );
        expect(status).toBe(200);
        expect(body.result.resultType).toBe("complete");
        expect(body.result.supportedVersions).toContain(MODERN);
        expect(body.result.supportedVersions).toContain("2025-03-26");
        expect(body.result.capabilities.tools).toBeDefined();
        expect(body.result.instructions).toContain("Prefer local CLI");
        expect(body.result._meta["io.modelcontextprotocol/serverInfo"].name).toBe("backlog-mcp-server");
        expect(body.result.cacheScope).toBe("public");
        expect(body.result.ttlMs).toBeGreaterThan(0);
    });

    it("is reachable without a protocol version declaration", async () => {
        await initTestKeys();
        const app = await createMcpApp({ config: makeConfig() });
        const { status, body } = await rawRequest<DiscoverResult>(app, { method: "server/discover" });
        expect(status).toBe(200);
        expect(body.result.supportedVersions).toContain(MODERN);
    });
});

describe("MCP transport — modern result envelope", () => {
    it("adds resultType, cache hints and serverInfo to modern list results", async () => {
        await initTestKeys();
        const app = await createMcpApp({ config: makeConfig() });
        const { body } = await rawRequest<ModernListResult>(
            app,
            { method: "tools/list", params: {} },
            { "MCP-Protocol-Version": MODERN, "Mcp-Method": "tools/list" },
        );
        expect(body.result.resultType).toBe("complete");
        expect(body.result.ttlMs).toBeGreaterThan(0);
        expect(body.result.cacheScope).toBe("private");
        expect(body.result._meta["io.modelcontextprotocol/serverInfo"].name).toBe("backlog-mcp-server");
        expect(body.result.tools.length).toBeGreaterThan(0);
    });

    it("leaves legacy results untouched", async () => {
        await initTestKeys();
        const app = await createMcpApp({ config: makeConfig() });
        const res = await jsonRpcRequest(app, "tools/list");
        expect(res.result.resultType).toBeUndefined();
        expect(res.result.ttlMs).toBeUndefined();
        expect(res.result._meta).toBeUndefined();
    });
});
