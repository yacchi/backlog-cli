// Tests for the MCP_AUTH_BYPASS_TOKEN test/dev escape hatch in jwtAuth.
//
// The bypass lets the transport + sandbox layers be exercised end-to-end without
// minting a signed JWT. It MUST only ever be enabled in local testing.

import { describe, it, expect, afterEach } from "vitest";
import { createMcpApp } from "../index.js";
import { generateKeyPair, exportJWK } from "jose";
import type { McpServerConfig } from "../config/schema.js";

let jwksJson: string;

async function makeConfig(): Promise<McpServerConfig> {
    if (!jwksJson) {
        const { privateKey } = await generateKeyPair("EdDSA", { crv: "Ed25519", extractable: true });
        const privJwk = await exportJWK(privateKey);
        jwksJson = JSON.stringify({ keys: [{ ...privJwk, kid: "k1", kty: "OKP", crv: "Ed25519" }] });
    }
    return {
        base_url: "https://mcp.example.com",
        relay_url: "https://relay.example.com",
        jwks: jwksJson,
        backlog_app: { client_id: "test-client-id" },
        spaces: [{ pattern: "mycompany\\.backlog\\.jp", writable: true }],
        default_spaces: ["mycompany.backlog.jp"],
    };
}

function initialize(app: Awaited<ReturnType<typeof createMcpApp>>, withHeader: boolean) {
    return app.request("/mcp", {
        method: "POST",
        headers: {
            "Content-Type": "application/json",
            ...(withHeader ? { Authorization: "Bearer anything" } : {}),
        },
        body: JSON.stringify({ jsonrpc: "2.0", id: 1, method: "initialize", params: {} }),
    });
}

afterEach(() => {
    delete process.env.MCP_AUTH_BYPASS_TOKEN;
});

describe("MCP_AUTH_BYPASS_TOKEN", () => {
    it("rejects unauthenticated requests when not set", async () => {
        const app = await createMcpApp({ config: await makeConfig() });
        const res = await initialize(app, false);
        expect(res.status).toBe(401);
    });

    it("accepts requests with no valid JWT when set", async () => {
        process.env.MCP_AUTH_BYPASS_TOKEN = JSON.stringify({
            space: "mycompany.backlog.jp",
            bl_access_token: "bypass-token",
            iat: 1700000000,
        });
        const app = await createMcpApp({ config: await makeConfig() });

        // Even with NO Authorization header, the request is authenticated.
        const res = await initialize(app, false);
        expect(res.status).toBe(200);
        const body = await res.json() as { result?: { serverInfo?: unknown } };
        expect(body.result?.serverInfo).toBeDefined();
    });

    it("rejects a payload missing 'space'", async () => {
        process.env.MCP_AUTH_BYPASS_TOKEN = JSON.stringify({ bl_access_token: "x" });
        await expect(createMcpApp({ config: await makeConfig() })).rejects.toThrow(/space/);
    });

    it("rejects invalid JSON", async () => {
        process.env.MCP_AUTH_BYPASS_TOKEN = "{not json";
        await expect(createMcpApp({ config: await makeConfig() })).rejects.toThrow(/valid JSON/);
    });
});
