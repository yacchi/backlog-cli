import { describe, it, expect } from "vitest";
import { createMcpApp } from "../index.js";
import { generateKey, exportKey, encryptToken } from "../crypto/jwe.js";
import type { McpServerConfig } from "../config/schema.js";
import type { TokenPayload } from "../crypto/jwe.js";

const testKey = generateKey();

function makeConfig(): McpServerConfig {
    return {
        base_url: "https://mcp.example.com",
        relay_url: "https://relay.example.com",
        token_key: exportKey(testKey),
        backlog_apps: [{ domain: "backlog.jp", client_id: "test-client-id" }],
        tenants: {
            "mycompany.backlog.jp": {
                cli_access: {
                    allow: ["issue *", "project *", "wiki *"],
                    deny: ["* --delete", "auth *", "config *"],
                },
            },
        },
    };
}

async function makeAccessToken(): Promise<string> {
    const now = Math.floor(Date.now() / 1000);
    return encryptToken(
        {
            bl_access_token: "test-backlog-token",
            bl_expires_at: now + 3600,
            space: "mycompany",
            domain: "backlog.jp",
            iat: now,
            exp: now + 3600,
        },
        testKey,
    );
}

async function jsonRpcRequest(
    app: ReturnType<typeof createMcpApp>,
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
    const app = createMcpApp({ config: makeConfig() });

    it("returns server info and capabilities", async () => {
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

    it("does not include instructions in initialize", async () => {
        const res = await jsonRpcRequest(app, "initialize", {
            protocolVersion: "2025-03-26",
            capabilities: {},
            clientInfo: { name: "test", version: "1.0" },
        });
        expect(res.result.instructions).toBeUndefined();
    });
});

describe("MCP transport — tools/list", () => {
    it("lists query and mutation tools when script disabled", async () => {
        const app = createMcpApp({ config: makeConfig() });
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

    it("lists all tools when script enabled", async () => {
        const config = makeConfig();
        config.tenants["mycompany.backlog.jp"].script = {
            enabled: true,
            max_cli_calls: 20,
            timeout_ms: 30000,
        };
        const app = createMcpApp({ config });
        const res = await jsonRpcRequest(app, "tools/list");
        const tools = res.result.tools;
        const names = tools.map((t: { name: string }) => t.name);
        expect(names).toContain("backlog_help");
        expect(names).toContain("who");
        expect(names).toContain("backlog_query");
        expect(names).toContain("backlog_mutate");
        expect(names).toContain("backlog_query_script");
        expect(names).toContain("backlog_mutate_script");
    });
});

describe("MCP transport — auth", () => {
    const app = createMcpApp({ config: makeConfig() });

    it("rejects request without Bearer token", async () => {
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
        const now = Math.floor(Date.now() / 1000);
        const expired = await encryptToken(
            {
                bl_access_token: "expired",
                space: "test",
                domain: "backlog.jp",
                iat: now - 7200,
                exp: now - 3600,
            },
            testKey,
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
    const app = createMcpApp({ config: makeConfig() });

    it("rejects missing args", async () => {
        const res = await jsonRpcRequest(app, "tools/call", {
            name: "backlog",
            arguments: {},
        });
        expect(res.error).toBeDefined();
        expect(res.error.code).toBe(-32602);
    });

    it("rejects unknown tool", async () => {
        const res = await jsonRpcRequest(app, "tools/call", {
            name: "nonexistent",
            arguments: {},
        });
        expect(res.error).toBeDefined();
    });
});

describe("MCP transport — prompts", () => {
    const app = createMcpApp({ config: makeConfig() });

    it("lists prompts", async () => {
        const res = await jsonRpcRequest(app, "prompts/list");
        expect(res.result.prompts).toHaveLength(1);
        expect(res.result.prompts[0].name).toBe("backlog-cli-reference");
    });

    it("gets prompt content", async () => {
        const res = await jsonRpcRequest(app, "prompts/get", {
            name: "backlog-cli-reference",
        });
        expect(res.result.messages).toHaveLength(1);
        expect(res.result.messages[0].content.text).toContain("Backlog CLI Reference");
        expect(res.result.messages[0].content.text).toContain("mycompany");
    });

    it("rejects unknown prompt", async () => {
        const res = await jsonRpcRequest(app, "prompts/get", {
            name: "nonexistent",
        });
        expect(res.error).toBeDefined();
    });
});

describe("MCP transport — tools/call backlog_help", () => {
    const app = createMcpApp({ config: makeConfig() });

    it("returns full CLI reference without command arg", async () => {
        const res = await jsonRpcRequest(app, "tools/call", {
            name: "backlog_help",
            arguments: {},
        });
        expect(res.result.content[0].text).toContain("Backlog CLI Reference");
        expect(res.result.content[0].text).toContain("mycompany");
        expect(res.result.content[0].text).toContain("backlog.jp");
    });

    it("returns filtered section for specific command", async () => {
        const res = await jsonRpcRequest(app, "tools/call", {
            name: "backlog_help",
            arguments: { command: "issue" },
        });
        expect(res.result.content[0].text).toContain("issue");
        expect(res.result.isError).toBeUndefined();
    });

    it("falls back to full reference for unknown command", async () => {
        const res = await jsonRpcRequest(app, "tools/call", {
            name: "backlog_help",
            arguments: { command: "nonexistent_command" },
        });
        expect(res.result.content[0].text).toContain("Backlog CLI Reference");
    });
});

describe("MCP transport — ping", () => {
    const app = createMcpApp({ config: makeConfig() });

    it("responds to ping", async () => {
        const res = await jsonRpcRequest(app, "ping");
        expect(res.result).toEqual({});
    });
});
