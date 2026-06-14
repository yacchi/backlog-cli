import { describe, it, expect } from "vitest";
import { createMcpApp } from "../index.js";
import { loadSigningKeys, sign, verify, spaceKey } from "./jwt.js";
import { seal, open } from "./secret.js";
import { generateKeyPair, exportJWK } from "jose";
import type { McpServerConfig } from "../config/schema.js";

const SPACE = "mycompany.backlog.jp";
const RAW_AT = "RAW-BACKLOG-ACCESS-TOKEN";
const RAW_RT = "RAW-BACKLOG-REFRESH-TOKEN";

async function makeJwks(): Promise<string> {
    const { privateKey } = await generateKeyPair("EdDSA", { crv: "Ed25519", extractable: true });
    const privJwk = await exportJWK(privateKey);
    return JSON.stringify({ keys: [{ ...privJwk, kid: "test-key-1", kty: "OKP", crv: "Ed25519" }] });
}

function makeConfig(jwks: string): McpServerConfig {
    return {
        base_url: "https://mcp.example.com",
        relay_url: "https://relay.example.com",
        jwks,
        backlog_app: { client_id: "test-client-id" },
        spaces: [{ pattern: "mycompany\\.backlog\\.jp", writable: true }],
        default_spaces: [SPACE],
    };
}

/** Build a server-style authorization code carrying sealed Backlog tokens. */
async function makeSealedCode(jwks: string): Promise<string> {
    const keys = await loadSigningKeys(jwks);
    const encKey = keys.encKeys.get(keys.signingKid)!;
    const now = Math.floor(Date.now() / 1000);
    return sign(
        {
            [spaceKey(SPACE)]: {
                at: await seal(RAW_AT, encKey, keys.signingKid, SPACE, "at"),
                rt: await seal(RAW_RT, encKey, keys.signingKid, SPACE, "rt"),
                exp: now + 3600,
            },
            space: SPACE,
            iat: now,
            exp: now + 300,
        },
        keys.signingKey,
        keys.signingKid,
    );
}

describe("token encryption — code exchange", () => {
    it("issues tokens that do not leak the raw Backlog tokens in plaintext", async () => {
        const jwks = await makeJwks();
        const app = await createMcpApp({ config: makeConfig(jwks) });
        const code = await makeSealedCode(jwks);

        const res = await app.request("/mcp/token", {
            method: "POST",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify({ grant_type: "authorization_code", code }),
        });
        expect(res.status).toBe(200);
        const body = await res.json();

        // The raw Backlog tokens must never appear in the issued JWTs.
        expect(body.access_token).not.toContain(RAW_AT);
        expect(body.refresh_token).not.toContain(RAW_RT);

        const keys = await loadSigningKeys(jwks);

        // Access token carries a sealed `at` (JWE compact = 5 segments).
        const accessPayload = await verify(body.access_token, keys.verifyKeys);
        const sealedAt = (accessPayload[spaceKey(SPACE)] as { at: string }).at;
        expect(sealedAt.split(".").length).toBe(5);
        const opened = await open(sealedAt, (kid) => keys.encKeys.get(kid), { sp: SPACE, use: "at" });
        expect(opened).toBe(RAW_AT);

        // Refresh token carries a sealed `rt`.
        const refreshPayload = await verify(body.refresh_token, keys.verifyKeys);
        const sealedRt = (refreshPayload[spaceKey(SPACE)] as { rt: string }).rt;
        expect(sealedRt.split(".").length).toBe(5);
        const openedRt = await open(sealedRt, (kid) => keys.encKeys.get(kid), { sp: SPACE, use: "rt" });
        expect(openedRt).toBe(RAW_RT);
    });

    it("rejects a code whose sealed tokens were encrypted under an unknown key", async () => {
        const jwks = await makeJwks();
        const otherJwks = await makeJwks();
        const app = await createMcpApp({ config: makeConfig(jwks) });

        // Code signed by the server's key but sealed with a foreign enc key:
        // exchange still succeeds (copy-forward), but the issued access token
        // cannot later be opened — so it is effectively unusable → re-auth.
        const serverKeys = await loadSigningKeys(jwks);
        const foreignKeys = await loadSigningKeys(otherJwks);
        const now = Math.floor(Date.now() / 1000);
        const code = await sign(
            {
                [spaceKey(SPACE)]: {
                    at: await seal(RAW_AT, foreignKeys.encKeys.get(foreignKeys.signingKid)!, serverKeys.signingKid, SPACE, "at"),
                    rt: await seal(RAW_RT, foreignKeys.encKeys.get(foreignKeys.signingKid)!, serverKeys.signingKid, SPACE, "rt"),
                    exp: now + 3600,
                },
                space: SPACE,
                iat: now,
                exp: now + 300,
            },
            serverKeys.signingKey,
            serverKeys.signingKid,
        );

        const res = await app.request("/mcp/token", {
            method: "POST",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify({ grant_type: "authorization_code", code }),
        });
        const body = await res.json();
        const accessPayload = await verify(body.access_token, serverKeys.verifyKeys);
        const sealedAt = (accessPayload[spaceKey(SPACE)] as { at: string }).at;
        await expect(
            open(sealedAt, (kid) => serverKeys.encKeys.get(kid), { sp: SPACE, use: "at" }),
        ).rejects.toThrow();
    });
});
