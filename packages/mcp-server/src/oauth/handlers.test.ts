import { describe, it, expect, vi, beforeEach } from "vitest";
import { createMcpApp } from "../index.js";
import { generateKey, exportKey, decrypt } from "../crypto/jwe.js";
import type { McpServerConfig } from "../config/schema.js";

const testKey = generateKey();

function makeConfig(overrides?: Partial<McpServerConfig>): McpServerConfig {
    return {
        base_url: "https://mcp.example.com",
        relay_url: "https://relay.example.com",
        token_key: exportKey(testKey),
        backlog_app: { client_id: "test-client-id" },
        tenants: {
            "mycompany.backlog.jp": {
                cli_access: { allow: ["*"], deny: [] },
            },
        },
        ...overrides,
    };
}

describe("Well-known endpoints", () => {
    const app = createMcpApp({ config: makeConfig() });

    it("GET /.well-known/oauth-protected-resource", async () => {
        const res = await app.request("/.well-known/oauth-protected-resource");
        expect(res.status).toBe(200);
        const body = await res.json();
        expect(body.resource).toBe("https://mcp.example.com/mcp");
        expect(body.authorization_servers).toEqual(["https://mcp.example.com"]);
    });

    it("GET /.well-known/oauth-authorization-server", async () => {
        const res = await app.request(
            "/.well-known/oauth-authorization-server",
        );
        expect(res.status).toBe(200);
        const body = await res.json();
        expect(body.issuer).toBe("https://mcp.example.com");
        expect(body.authorization_endpoint).toBe(
            "https://mcp.example.com/mcp/authorize",
        );
        expect(body.token_endpoint).toBe(
            "https://mcp.example.com/mcp/token",
        );
        expect(body.registration_endpoint).toBe(
            "https://mcp.example.com/mcp/register",
        );
        expect(body.code_challenge_methods_supported).toContain("S256");
        expect(body.token_endpoint_auth_methods_supported).toContain("none");
    });
});

describe("POST /mcp/register (DCR)", () => {
    const app = createMcpApp({ config: makeConfig() });

    it("registers a client with valid redirect_uris", async () => {
        const res = await app.request("/mcp/register", {
            method: "POST",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify({
                redirect_uris: ["https://claude.ai/oauth/callback"],
                client_name: "Claude Desktop",
            }),
        });
        expect(res.status).toBe(201);
        const body = await res.json();
        expect(body.client_id).toBeTruthy();
        expect(body.redirect_uris).toEqual([
            "https://claude.ai/oauth/callback",
        ]);
        expect(body.client_name).toBe("Claude Desktop");
        expect(body.token_endpoint_auth_method).toBe("none");

        // client_id is a valid JWE that can be decrypted
        const payload = await decrypt(body.client_id, testKey);
        expect(payload.redirect_uris).toEqual([
            "https://claude.ai/oauth/callback",
        ]);
    });

    it("rejects missing redirect_uris", async () => {
        const res = await app.request("/mcp/register", {
            method: "POST",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify({ client_name: "Test" }),
        });
        expect(res.status).toBe(400);
        const body = await res.json();
        expect(body.error).toBe("invalid_client_metadata");
    });

    it("rejects invalid redirect_uri", async () => {
        const res = await app.request("/mcp/register", {
            method: "POST",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify({
                redirect_uris: ["not-a-url"],
            }),
        });
        expect(res.status).toBe(400);
        expect((await res.json()).error).toBe("invalid_redirect_uri");
    });
});

describe("GET /mcp/authorize", () => {
    const app = createMcpApp({ config: makeConfig() });

    it("rejects missing parameters", async () => {
        const res = await app.request("/mcp/authorize?client_id=x");
        expect(res.status).toBe(400);
    });

    it("rejects unsupported response_type", async () => {
        const params = new URLSearchParams({
            client_id: "x",
            redirect_uri: "https://example.com/cb",
            response_type: "token",
            state: "abc",
            code_challenge: "challenge",
            code_challenge_method: "S256",
        });
        const res = await app.request(`/mcp/authorize?${params}`);
        expect(res.status).toBe(400);
        expect((await res.json()).error).toBe("unsupported_response_type");
    });

    it("rejects missing code_challenge", async () => {
        // First register a client to get a valid client_id
        const regRes = await app.request("/mcp/register", {
            method: "POST",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify({
                redirect_uris: ["https://example.com/cb"],
            }),
        });
        const { client_id } = await regRes.json();

        const params = new URLSearchParams({
            client_id,
            redirect_uri: "https://example.com/cb",
            response_type: "code",
            state: "abc",
        });
        const res = await app.request(`/mcp/authorize?${params}`);
        expect(res.status).toBe(400);
        expect((await res.json()).error).toBe("invalid_request");
    });

    it("redirects to Backlog with valid params and scope", async () => {
        const regRes = await app.request("/mcp/register", {
            method: "POST",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify({
                redirect_uris: ["https://example.com/cb"],
            }),
        });
        const { client_id } = await regRes.json();

        const params = new URLSearchParams({
            client_id,
            redirect_uri: "https://example.com/cb",
            response_type: "code",
            state: "test-state",
            code_challenge: "E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM",
            code_challenge_method: "S256",
            scope: "backlog:mycompany.backlog.jp",
        });
        const res = await app.request(`/mcp/authorize?${params}`);
        expect(res.status).toBe(200);

        const html = await res.text();
        expect(html).toContain("mycompany.backlog.jp");
        expect(html).toContain("Authenticate Backlog Spaces");
    });

    it("rejects mismatched redirect_uri", async () => {
        const regRes = await app.request("/mcp/register", {
            method: "POST",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify({
                redirect_uris: ["https://example.com/cb"],
            }),
        });
        const { client_id } = await regRes.json();

        const params = new URLSearchParams({
            client_id,
            redirect_uri: "https://evil.com/cb",
            response_type: "code",
            state: "abc",
            code_challenge: "challenge",
            code_challenge_method: "S256",
            scope: "backlog:mycompany.backlog.jp",
        });
        const res = await app.request(`/mcp/authorize?${params}`);
        expect(res.status).toBe(400);
        expect((await res.json()).error).toBe("invalid_redirect_uri");
    });
});

describe("POST /mcp/token", () => {
    const app = createMcpApp({ config: makeConfig() });

    it("rejects unsupported grant_type", async () => {
        const res = await app.request("/mcp/token", {
            method: "POST",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify({ grant_type: "client_credentials" }),
        });
        expect(res.status).toBe(400);
        expect((await res.json()).error).toBe("unsupported_grant_type");
    });

    it("rejects missing code in authorization_code grant", async () => {
        const res = await app.request("/mcp/token", {
            method: "POST",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify({ grant_type: "authorization_code" }),
        });
        expect(res.status).toBe(400);
        expect((await res.json()).error).toBe("invalid_request");
    });

    it("rejects invalid code", async () => {
        const res = await app.request("/mcp/token", {
            method: "POST",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify({
                grant_type: "authorization_code",
                code: "invalid-jwe",
            }),
        });
        expect(res.status).toBe(400);
        expect((await res.json()).error).toBe("invalid_grant");
    });

    it("rejects missing refresh_token in refresh_token grant", async () => {
        const res = await app.request("/mcp/token", {
            method: "POST",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify({ grant_type: "refresh_token" }),
        });
        expect(res.status).toBe(400);
        expect((await res.json()).error).toBe("invalid_request");
    });

    it("accepts application/x-www-form-urlencoded", async () => {
        const res = await app.request("/mcp/token", {
            method: "POST",
            headers: {
                "Content-Type": "application/x-www-form-urlencoded",
            },
            body: new URLSearchParams({
                grant_type: "authorization_code",
            }).toString(),
        });
        expect(res.status).toBe(400);
        expect((await res.json()).error).toBe("invalid_request");
    });
});

describe("GET /health", () => {
    const app = createMcpApp({ config: makeConfig() });

    it("returns ok", async () => {
        const res = await app.request("/health");
        expect(res.status).toBe(200);
        expect(await res.json()).toEqual({ status: "ok" });
    });
});
