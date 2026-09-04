import { describe, it, expect } from "vitest";
import { createMcpApp } from "../index.js";
import { s256Challenge } from "./handlers.js";
import { verify, loadSigningKeys, sign, spaceKey, setSpaceAccess, setSpaceRefresh } from "../crypto/jwt.js";
import { generateKeyPair, exportJWK } from "jose";
import type { McpServerConfig } from "../config/schema.js";
import { readJson } from "../test-support/http.js";

interface ClientRegistrationResponse {
    client_id: string;
    client_name?: string;
    redirect_uris?: string[];
    token_endpoint_auth_method?: string;
}

interface TokenResponse {
    access_token: string;
    refresh_token: string;
    expires_in: number;
}

let testJwksJson: string;

const VERIFIER = "test-code-verifier-0123456789-abcdefghijklmnopqrstuv";
const CLIENT_ID = "test-client-id-jwt";
const REDIRECT_URI = "https://client.example.com/cb";

/** PKCE / client binding claims that /mcp/authorize puts into every code. */
async function codeBinding(): Promise<Record<string, unknown>> {
    return {
        code_challenge: await s256Challenge(VERIFIER),
        code_challenge_method: "S256",
        client_id: CLIENT_ID,
        redirect_uri: REDIRECT_URI,
    };
}

async function initTestKeys() {
    if (testJwksJson) return;
    const { privateKey } = await generateKeyPair("EdDSA", { crv: "Ed25519", extractable: true });
    const privJwk = await exportJWK(privateKey);
    const jwks = { keys: [{ ...privJwk, kid: "test-key-1", kty: "OKP", crv: "Ed25519" }] };
    testJwksJson = JSON.stringify(jwks);
}

function makeConfig(overrides?: Partial<McpServerConfig>): McpServerConfig {
    return {
        base_url: "https://mcp.example.com",
        relay_url: "https://relay.example.com",
        jwks: testJwksJson,
        backlog_app: { client_id: "test-client-id" },
        spaces: [
            { pattern: "mycompany\\.backlog\\.jp", writable: true },
        ],
        default_spaces: ["mycompany.backlog.jp"],
        ...overrides,
    };
}

describe("Well-known endpoints", () => {
    it("GET /.well-known/oauth-protected-resource", async () => {
        await initTestKeys();
        const app = await createMcpApp({ config: makeConfig() });
        const res = await app.request("/.well-known/oauth-protected-resource");
        expect(res.status).toBe(200);
        const body = await readJson(res);
        expect(body.resource).toBe("https://mcp.example.com/mcp");
        expect(body.authorization_servers).toEqual(["https://mcp.example.com"]);
    });

    it("GET /.well-known/oauth-authorization-server", async () => {
        await initTestKeys();
        const app = await createMcpApp({ config: makeConfig() });
        const res = await app.request(
            "/.well-known/oauth-authorization-server",
        );
        expect(res.status).toBe(200);
        const body = await readJson(res);
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

    it("advertises CIMD support and the iss parameter", async () => {
        await initTestKeys();
        const app = await createMcpApp({ config: makeConfig() });
        const res = await app.request("/.well-known/oauth-authorization-server");
        const body = await readJson(res);
        expect(body.client_id_metadata_document_supported).toBe(true);
        expect(body.authorization_response_iss_parameter_supported).toBe(true);
        // DCR stays available for clients that do not implement CIMD yet.
        expect(body.registration_endpoint).toBeTruthy();
    });

    it("reports CIMD as unsupported when disabled", async () => {
        await initTestKeys();
        const app = await createMcpApp({
            config: makeConfig({ cimd: { enabled: false, allowed_hosts: [] } }),
        });
        const res = await app.request("/.well-known/oauth-authorization-server");
        expect((await readJson(res)).client_id_metadata_document_supported).toBe(false);
    });
});

describe("GET /mcp/authorize with a Client ID Metadata Document", () => {
    const CLIENT_ID = "https://app.example.com/oauth/client.json";

    function authorizeParams(overrides?: Record<string, string>): URLSearchParams {
        return new URLSearchParams({
            client_id: CLIENT_ID,
            redirect_uri: "https://app.example.com/cb",
            response_type: "code",
            state: "cimd-state",
            code_challenge: "E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM",
            code_challenge_method: "S256",
            scope: "backlog:mycompany.backlog.jp",
            ...overrides,
        });
    }

    function stubResolver(metadata: Record<string, unknown>) {
        return {
            resolve: async () => metadata as never,
        };
    }

    it("accepts a URL client_id whose document lists the redirect_uri", async () => {
        await initTestKeys();
        const app = await createMcpApp({
            config: makeConfig(),
            clientMetadataResolver: stubResolver({
                client_id: CLIENT_ID,
                client_name: "Example MCP Client",
                redirect_uris: ["https://app.example.com/cb"],
            }),
        });
        const res = await app.request(`/mcp/authorize?${authorizeParams()}`);
        expect(res.status).toBe(200);
        expect(await res.text()).toContain("mycompany.backlog.jp");
    });

    it("rejects a redirect_uri absent from the document", async () => {
        await initTestKeys();
        const app = await createMcpApp({
            config: makeConfig(),
            clientMetadataResolver: stubResolver({
                client_id: CLIENT_ID,
                client_name: "Example MCP Client",
                redirect_uris: ["https://app.example.com/other"],
            }),
        });
        const res = await app.request(`/mcp/authorize?${authorizeParams()}`);
        expect(res.status).toBe(400);
        expect((await readJson(res)).error).toBe("invalid_redirect_uri");
    });

    it("rejects a URL client_id when the document cannot be resolved", async () => {
        await initTestKeys();
        const app = await createMcpApp({
            config: makeConfig(),
            clientMetadataResolver: {
                resolve: async () => {
                    throw new Error("fetch failed");
                },
            },
        });
        const res = await app.request(`/mcp/authorize?${authorizeParams()}`);
        expect(res.status).toBe(400);
        expect((await readJson(res)).error).toBe("invalid_client");
    });

    it("rejects a URL client_id when CIMD is disabled", async () => {
        await initTestKeys();
        const app = await createMcpApp({
            config: makeConfig({ cimd: { enabled: false, allowed_hosts: [] } }),
            clientMetadataResolver: stubResolver({
                client_id: CLIENT_ID,
                client_name: "Example MCP Client",
                redirect_uris: ["https://app.example.com/cb"],
            }),
        });
        const res = await app.request(`/mcp/authorize?${authorizeParams()}`);
        expect(res.status).toBe(400);
        expect((await readJson(res)).error).toBe("invalid_client");
    });
});

describe("POST /mcp/register (DCR)", () => {
    it("registers a client with valid redirect_uris", async () => {
        await initTestKeys();
        const app = await createMcpApp({ config: makeConfig() });
        const res = await app.request("/mcp/register", {
            method: "POST",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify({
                redirect_uris: ["https://claude.ai/oauth/callback"],
                client_name: "Claude Desktop",
            }),
        });
        expect(res.status).toBe(201);
        const body = await readJson<ClientRegistrationResponse>(res);
        expect(body.client_id).toBeTruthy();
        expect(body.redirect_uris).toEqual([
            "https://claude.ai/oauth/callback",
        ]);
        expect(body.client_name).toBe("Claude Desktop");
        expect(body.token_endpoint_auth_method).toBe("none");

        const keys = await loadSigningKeys(testJwksJson);
        const payload = await verify(body.client_id, keys.verifyKeys);
        expect(payload.redirect_uris).toEqual([
            "https://claude.ai/oauth/callback",
        ]);
    });

    it("rejects missing redirect_uris", async () => {
        await initTestKeys();
        const app = await createMcpApp({ config: makeConfig() });
        const res = await app.request("/mcp/register", {
            method: "POST",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify({ client_name: "Test" }),
        });
        expect(res.status).toBe(400);
        const body = await readJson(res);
        expect(body.error).toBe("invalid_client_metadata");
    });

    it("rejects invalid redirect_uri", async () => {
        await initTestKeys();
        const app = await createMcpApp({ config: makeConfig() });
        const res = await app.request("/mcp/register", {
            method: "POST",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify({
                redirect_uris: ["not-a-url"],
            }),
        });
        expect(res.status).toBe(400);
        expect((await readJson(res)).error).toBe("invalid_redirect_uri");
    });
});

describe("GET /mcp/authorize", () => {
    it("rejects missing parameters", async () => {
        await initTestKeys();
        const app = await createMcpApp({ config: makeConfig() });
        const res = await app.request("/mcp/authorize?client_id=x");
        expect(res.status).toBe(400);
    });

    it("rejects unsupported response_type", async () => {
        await initTestKeys();
        const app = await createMcpApp({ config: makeConfig() });
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
        expect((await readJson(res)).error).toBe("unsupported_response_type");
    });

    it("rejects missing code_challenge", async () => {
        await initTestKeys();
        const app = await createMcpApp({ config: makeConfig() });
        const regRes = await app.request("/mcp/register", {
            method: "POST",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify({
                redirect_uris: ["https://example.com/cb"],
            }),
        });
        const { client_id } = await readJson<ClientRegistrationResponse>(regRes);

        const params = new URLSearchParams({
            client_id,
            redirect_uri: "https://example.com/cb",
            response_type: "code",
            state: "abc",
        });
        const res = await app.request(`/mcp/authorize?${params}`);
        expect(res.status).toBe(400);
        expect((await readJson(res)).error).toBe("invalid_request");
    });

    it("renders auth page with valid params and scope", async () => {
        await initTestKeys();
        const app = await createMcpApp({ config: makeConfig() });
        const regRes = await app.request("/mcp/register", {
            method: "POST",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify({
                redirect_uris: ["https://example.com/cb"],
            }),
        });
        const { client_id } = await readJson<ClientRegistrationResponse>(regRes);

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
        expect(html).toContain("Backlog スペースの認証");
    });

    it("rejects mismatched redirect_uri", async () => {
        await initTestKeys();
        const app = await createMcpApp({ config: makeConfig() });
        const regRes = await app.request("/mcp/register", {
            method: "POST",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify({
                redirect_uris: ["https://example.com/cb"],
            }),
        });
        const { client_id } = await readJson<ClientRegistrationResponse>(regRes);

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
        expect((await readJson(res)).error).toBe("invalid_redirect_uri");
    });
});

describe("POST /mcp/token", () => {
    it("rejects unsupported grant_type", async () => {
        await initTestKeys();
        const app = await createMcpApp({ config: makeConfig() });
        const res = await app.request("/mcp/token", {
            method: "POST",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify({ grant_type: "client_credentials" }),
        });
        expect(res.status).toBe(400);
        expect((await readJson(res)).error).toBe("unsupported_grant_type");
    });

    it("rejects missing code in authorization_code grant", async () => {
        await initTestKeys();
        const app = await createMcpApp({ config: makeConfig() });
        const res = await app.request("/mcp/token", {
            method: "POST",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify({ grant_type: "authorization_code" }),
        });
        expect(res.status).toBe(400);
        expect((await readJson(res)).error).toBe("invalid_request");
    });

    it("rejects invalid code", async () => {
        await initTestKeys();
        const app = await createMcpApp({ config: makeConfig() });
        const res = await app.request("/mcp/token", {
            method: "POST",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify({
                grant_type: "authorization_code",
                code: "invalid-jwt",
            }),
        });
        expect(res.status).toBe(400);
        expect((await readJson(res)).error).toBe("invalid_grant");
    });

    it("rejects missing refresh_token in refresh_token grant", async () => {
        await initTestKeys();
        const app = await createMcpApp({ config: makeConfig() });
        const res = await app.request("/mcp/token", {
            method: "POST",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify({ grant_type: "refresh_token" }),
        });
        expect(res.status).toBe(400);
        expect((await readJson(res)).error).toBe("invalid_request");
    });

    it("exchanges a single-space code that carries both at and rt", async () => {
        await initTestKeys();
        const space = "mycompany.backlog.jp";
        const keys = await loadSigningKeys(testJwksJson);
        const now = Math.floor(Date.now() / 1000);

        // /mcp/authorize の単一スペースフローと同じ手順でコードを組み立てる。
        // setSpaceAccess → setSpaceRefresh の順で同一キーへ書くため、
        // 上書き実装だと at/exp が失われ exp が NaN になって 500 になる。
        const codePayload: Record<string, unknown> = {
            space,
            ...(await codeBinding()),
            iat: now,
            exp: now + 300,
        };
        setSpaceAccess(codePayload, space, "sealed-at", now + 3600);
        setSpaceRefresh(codePayload, space, "sealed-rt");
        const code = await sign(codePayload, keys.signingKey, keys.signingKid);

        const app = await createMcpApp({
            config: makeConfig({ audit: { collect_user_info: false } }),
        });
        const res = await app.request("/mcp/token", {
            method: "POST",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify({
                grant_type: "authorization_code",
                code,
                code_verifier: VERIFIER,
                client_id: CLIENT_ID,
                redirect_uri: REDIRECT_URI,
            }),
        });

        expect(res.status).toBe(200);
        const body = await readJson<TokenResponse>(res);
        expect(Number.isFinite(body.expires_in)).toBe(true);
        expect(body.expires_in).toBeGreaterThan(0);

        const accessPayload = await verify(body.access_token, keys.verifyKeys);
        expect((accessPayload[spaceKey(space)] as { at: string }).at).toBe("sealed-at");
        expect(Number.isFinite(accessPayload.exp as number)).toBe(true);

        const refreshPayload = await verify(body.refresh_token, keys.verifyKeys);
        expect((refreshPayload[spaceKey(space)] as { rt: string }).rt).toBe("sealed-rt");
    });

    it("rejects a code exchange without code_verifier (PKCE)", async () => {
        await initTestKeys();
        const space = "mycompany.backlog.jp";
        const keys = await loadSigningKeys(testJwksJson);
        const now = Math.floor(Date.now() / 1000);

        const codePayload: Record<string, unknown> = {
            space,
            ...(await codeBinding()),
            iat: now,
            exp: now + 300,
        };
        setSpaceAccess(codePayload, space, "sealed-at", now + 3600);
        setSpaceRefresh(codePayload, space, "sealed-rt");
        const code = await sign(codePayload, keys.signingKey, keys.signingKid);

        const app = await createMcpApp({ config: makeConfig() });
        const res = await app.request("/mcp/token", {
            method: "POST",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify({ grant_type: "authorization_code", code, client_id: CLIENT_ID }),
        });
        expect(res.status).toBe(400);
        const body = await readJson(res);
        expect(body.error).toBe("invalid_grant");
        expect(body.error_description).toContain("code_verifier");
    });

    it("rejects a code exchange with a wrong code_verifier", async () => {
        await initTestKeys();
        const space = "mycompany.backlog.jp";
        const keys = await loadSigningKeys(testJwksJson);
        const now = Math.floor(Date.now() / 1000);

        const codePayload: Record<string, unknown> = {
            space,
            ...(await codeBinding()),
            iat: now,
            exp: now + 300,
        };
        setSpaceAccess(codePayload, space, "sealed-at", now + 3600);
        setSpaceRefresh(codePayload, space, "sealed-rt");
        const code = await sign(codePayload, keys.signingKey, keys.signingKid);

        const app = await createMcpApp({ config: makeConfig() });
        const res = await app.request("/mcp/token", {
            method: "POST",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify({
                grant_type: "authorization_code",
                code,
                code_verifier: "an-entirely-different-verifier-0123456789abcdef",
                client_id: CLIENT_ID,
            }),
        });
        expect(res.status).toBe(400);
        expect((await readJson(res)).error).toBe("invalid_grant");
    });

    it("rejects a code that carries no PKCE challenge at all", async () => {
        await initTestKeys();
        const space = "mycompany.backlog.jp";
        const keys = await loadSigningKeys(testJwksJson);
        const now = Math.floor(Date.now() / 1000);

        // 旧サーバーが発行した PKCE 非バインドの code。ダウングレードさせず拒否する。
        const codePayload: Record<string, unknown> = { space, iat: now, exp: now + 300 };
        setSpaceAccess(codePayload, space, "sealed-at", now + 3600);
        setSpaceRefresh(codePayload, space, "sealed-rt");
        const code = await sign(codePayload, keys.signingKey, keys.signingKid);

        const app = await createMcpApp({ config: makeConfig() });
        const res = await app.request("/mcp/token", {
            method: "POST",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify({ grant_type: "authorization_code", code, code_verifier: VERIFIER }),
        });
        expect(res.status).toBe(400);
        expect((await readJson(res)).error).toBe("invalid_grant");
    });

    it("rejects a code redeemed by a different client_id", async () => {
        await initTestKeys();
        const space = "mycompany.backlog.jp";
        const keys = await loadSigningKeys(testJwksJson);
        const now = Math.floor(Date.now() / 1000);

        const codePayload: Record<string, unknown> = {
            space,
            ...(await codeBinding()),
            iat: now,
            exp: now + 300,
        };
        setSpaceAccess(codePayload, space, "sealed-at", now + 3600);
        setSpaceRefresh(codePayload, space, "sealed-rt");
        const code = await sign(codePayload, keys.signingKey, keys.signingKid);

        const app = await createMcpApp({ config: makeConfig() });
        const res = await app.request("/mcp/token", {
            method: "POST",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify({
                grant_type: "authorization_code",
                code,
                code_verifier: VERIFIER,
                client_id: "someone-elses-client-id",
            }),
        });
        expect(res.status).toBe(400);
        const body = await readJson(res);
        expect(body.error).toBe("invalid_grant");
        expect(body.error_description).toContain("client_id");
    });

    it("rejects a code redeemed against a different redirect_uri", async () => {
        await initTestKeys();
        const space = "mycompany.backlog.jp";
        const keys = await loadSigningKeys(testJwksJson);
        const now = Math.floor(Date.now() / 1000);

        const codePayload: Record<string, unknown> = {
            space,
            ...(await codeBinding()),
            iat: now,
            exp: now + 300,
        };
        setSpaceAccess(codePayload, space, "sealed-at", now + 3600);
        setSpaceRefresh(codePayload, space, "sealed-rt");
        const code = await sign(codePayload, keys.signingKey, keys.signingKid);

        const app = await createMcpApp({ config: makeConfig() });
        const res = await app.request("/mcp/token", {
            method: "POST",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify({
                grant_type: "authorization_code",
                code,
                code_verifier: VERIFIER,
                client_id: CLIENT_ID,
                redirect_uri: "https://attacker.example.com/cb",
            }),
        });
        expect(res.status).toBe(400);
        expect((await readJson(res)).error_description).toContain("redirect_uri");
    });

    it("rejects a code whose space entry is missing rt", async () => {
        await initTestKeys();
        const space = "mycompany.backlog.jp";
        const keys = await loadSigningKeys(testJwksJson);
        const now = Math.floor(Date.now() / 1000);

        const code = await sign(
            {
                [spaceKey(space)]: { at: "sealed-at", exp: now + 3600 },
                space,
                ...(await codeBinding()),
                iat: now,
                exp: now + 300,
            },
            keys.signingKey,
            keys.signingKid,
        );

        const app = await createMcpApp({ config: makeConfig() });
        const res = await app.request("/mcp/token", {
            method: "POST",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify({ grant_type: "authorization_code", code }),
        });
        expect(res.status).toBe(400);
        expect((await readJson(res)).error).toBe("invalid_grant");
    });

    it("rejects a code whose space entry has a non-numeric exp", async () => {
        await initTestKeys();
        const space = "mycompany.backlog.jp";
        const keys = await loadSigningKeys(testJwksJson);
        const now = Math.floor(Date.now() / 1000);

        const code = await sign(
            {
                [spaceKey(space)]: { at: "sealed-at", rt: "sealed-rt", exp: "soon" },
                space,
                ...(await codeBinding()),
                iat: now,
                exp: now + 300,
            },
            keys.signingKey,
            keys.signingKid,
        );

        const app = await createMcpApp({ config: makeConfig() });
        const res = await app.request("/mcp/token", {
            method: "POST",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify({ grant_type: "authorization_code", code }),
        });
        expect(res.status).toBe(400);
        expect((await readJson(res)).error).toBe("invalid_grant");
    });

    it("rejects a refresh token whose space entry is missing rt", async () => {
        await initTestKeys();
        const space = "mycompany.backlog.jp";
        const keys = await loadSigningKeys(testJwksJson);
        const now = Math.floor(Date.now() / 1000);

        const refreshToken = await sign(
            { [spaceKey(space)]: { at: "sealed-at", exp: now + 3600 }, space, iat: now },
            keys.signingKey,
            keys.signingKid,
        );

        const app = await createMcpApp({ config: makeConfig() });
        const res = await app.request("/mcp/token", {
            method: "POST",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify({ grant_type: "refresh_token", refresh_token: refreshToken }),
        });
        expect(res.status).toBe(400);
        expect((await readJson(res)).error).toBe("invalid_grant");
    });

    it("accepts application/x-www-form-urlencoded", async () => {
        await initTestKeys();
        const app = await createMcpApp({ config: makeConfig() });
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
        expect((await readJson(res)).error).toBe("invalid_request");
    });
});

describe("GET /health", () => {
    it("returns ok", async () => {
        await initTestKeys();
        const app = await createMcpApp({ config: makeConfig() });
        const res = await app.request("/health");
        expect(res.status).toBe(200);
        expect(await readJson(res)).toEqual({ status: "ok" });
    });
});

describe("space cookie session binding", () => {
    async function getAuthorizeSession(app: Awaited<ReturnType<typeof createMcpApp>>, state: string): Promise<string> {
        const regRes = await app.request("/mcp/register", {
            method: "POST",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify({ redirect_uris: ["https://example.com/cb"] }),
        });
        const { client_id } = (await readJson(regRes)) as { client_id: string };
        const params = new URLSearchParams({
            client_id,
            redirect_uri: "https://example.com/cb",
            response_type: "code",
            state,
            code_challenge: "E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM",
            code_challenge_method: "S256",
            scope: "backlog:mycompany.backlog.jp",
        });
        const res = await app.request(`/mcp/authorize?${params}`);
        const html = await res.text();
        const m = html.match(/const SESSION = "([^"]+)"/);
        if (!m) throw new Error("session not found in auth page");
        return m[1];
    }

    async function craftSpaceCookie(space: string, sid: string): Promise<{ name: string; value: string }> {
        const { spaceCookieName } = await import("./handlers.js");
        const { sign, loadSigningKeys } = await import("../crypto/jwt.js");
        const keys = await loadSigningKeys(testJwksJson);
        const now = Math.floor(Date.now() / 1000);
        const value = await sign(
            {
                space,
                bl_access_token: "bl-at",
                bl_refresh_token: "bl-rt",
                bl_expires_at: now + 3600,
                sid,
            },
            keys.signingKey,
            keys.signingKid,
        );
        return { name: spaceCookieName(space, sid), value };
    }

    it("status: session B cannot see a space cookie authenticated under session A", async () => {
        await initTestKeys();
        const { sessionFingerprint } = await import("./handlers.js");
        const app = await createMcpApp({ config: makeConfig() });

        const sessionA = await getAuthorizeSession(app, "state-A");
        const sessionB = await getAuthorizeSession(app, "state-B");
        expect(sessionA).not.toBe(sessionB);

        const sidA = await sessionFingerprint(sessionA);
        const space = "mycompany.backlog.jp";
        const cookie = await craftSpaceCookie(space, sidA);

        // Session A sees its own authenticated space.
        const resA = await app.request(
            `/mcp/authorize/status?session=${encodeURIComponent(sessionA)}&spaces=${encodeURIComponent(space)}`,
            { headers: { Cookie: `${cookie.name}=${cookie.value}` } },
        );
        const bodyA = (await resA.json()) as { spaces: Array<{ space: string; authenticated: boolean }> };
        expect(bodyA.spaces[0].authenticated).toBe(true);

        // Session B (same browser, same cookie jar) must NOT observe it.
        const resB = await app.request(
            `/mcp/authorize/status?session=${encodeURIComponent(sessionB)}&spaces=${encodeURIComponent(space)}`,
            { headers: { Cookie: `${cookie.name}=${cookie.value}` } },
        );
        const bodyB = (await resB.json()) as { spaces: Array<{ space: string; authenticated: boolean }> };
        expect(bodyB.spaces[0].authenticated).toBe(false);
    });

    it("complete: session B cannot mint a code from session A's space cookie", async () => {
        await initTestKeys();
        const { sessionFingerprint } = await import("./handlers.js");
        const app = await createMcpApp({ config: makeConfig() });

        const sessionA = await getAuthorizeSession(app, "state-A");
        const sessionB = await getAuthorizeSession(app, "state-B");

        const sidA = await sessionFingerprint(sessionA);
        const space = "mycompany.backlog.jp";
        const cookie = await craftSpaceCookie(space, sidA);

        // Session B tries to complete using A's cookie → no usable token → error.
        const resB = await app.request("/mcp/authorize/complete", {
            method: "POST",
            headers: {
                "Content-Type": "application/x-www-form-urlencoded",
                Cookie: `${cookie.name}=${cookie.value}`,
            },
            body: new URLSearchParams({ session: sessionB, spaces: space }).toString(),
        });
        // Returns the "no authenticated space" error page (400), never a redirect with a code.
        expect(resB.status).toBe(400);
        expect(resB.headers.get("location")).toBeNull();

        // Session A completes successfully with its own cookie.
        const resA = await app.request("/mcp/authorize/complete", {
            method: "POST",
            headers: {
                "Content-Type": "application/x-www-form-urlencoded",
                Cookie: `${cookie.name}=${cookie.value}`,
            },
            body: new URLSearchParams({ session: sessionA, spaces: space }).toString(),
        });
        expect(resA.status).toBe(302);
        const loc = resA.headers.get("location");
        expect(loc).toBeTruthy();
        expect(loc).toContain("code=");
        expect(loc).toContain("state=state-A");
    });

    // RFC 9207 (MCP SEP-2468): the authorization response must identify the
    // issuer so clients can detect mix-up attacks before redeeming the code.
    it("complete: authorization response carries the iss parameter", async () => {
        await initTestKeys();
        const { sessionFingerprint } = await import("./handlers.js");
        const app = await createMcpApp({ config: makeConfig() });

        const session = await getAuthorizeSession(app, "state-iss");
        const sid = await sessionFingerprint(session);
        const space = "mycompany.backlog.jp";
        const cookie = await craftSpaceCookie(space, sid);

        const res = await app.request("/mcp/authorize/complete", {
            method: "POST",
            headers: {
                "Content-Type": "application/x-www-form-urlencoded",
                Cookie: `${cookie.name}=${cookie.value}`,
            },
            body: new URLSearchParams({ session, spaces: space }).toString(),
        });
        expect(res.status).toBe(302);
        const loc = new URL(res.headers.get("location")!);
        expect(loc.searchParams.get("iss")).toBe("https://mcp.example.com");
    });
});

describe("parseScopes with space patterns", () => {
    it("filters scopes by space patterns", async () => {
        const { parseScopes } = await import("./handlers.js");
        await initTestKeys();
        const config = makeConfig();
        const result = parseScopes("backlog:mycompany.backlog.jp backlog:rogue.backlog.com", config);
        expect(result).toHaveLength(1);
        expect(result[0]).toEqual({ space: "mycompany.backlog.jp" });
    });

    it("falls back to default_spaces when no scope", async () => {
        const { parseScopes } = await import("./handlers.js");
        await initTestKeys();
        const config = makeConfig();
        const result = parseScopes(undefined, config);
        expect(result).toHaveLength(1);
        expect(result[0]).toEqual({ space: "mycompany.backlog.jp" });
    });

    it("returns empty when scope is invalid and no default_spaces", async () => {
        const { parseScopes } = await import("./handlers.js");
        await initTestKeys();
        const config = makeConfig({ default_spaces: [] });
        const result = parseScopes("backlog:rogue.backlog.com", config);
        expect(result).toHaveLength(0);
    });
});
