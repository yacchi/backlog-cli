import { describe, it, expect } from "vitest";
import {
    signToken,
    verifyToken,
    sign,
    verify,
    loadSigningKeys,
    type TokenPayload,
} from "./jwt.js";
import { exportJWK, generateKeyPair } from "jose";

async function makeTestJWKS(): Promise<{ jwksJson: string; kid: string }> {
    const { publicKey, privateKey } = await generateKeyPair("EdDSA", { crv: "Ed25519", extractable: true });
    const privJwk = await exportJWK(privateKey);
    const kid = "test-key-1";
    const jwks = { keys: [{ ...privJwk, kid, kty: "OKP", crv: "Ed25519" }] };
    return { jwksJson: JSON.stringify(jwks), kid };
}

describe("JWT sign/verify", () => {
    it("sign → verify roundtrip for access token", async () => {
        const { jwksJson } = await makeTestJWKS();
        const keys = await loadSigningKeys(jwksJson);
        const now = Math.floor(Date.now() / 1000);

        const payload: TokenPayload = {
            bl_access_token: "backlog-access-token-xyz",
            bl_expires_at: now + 3600,
            space: "mycompany",
            domain: "backlog.jp",
            iat: now,
            exp: now + 3600,
        };

        const jwt = await signToken(payload, keys.signingKey, keys.signingKid);
        const verified = await verifyToken(jwt, keys.verifyKeys);

        expect(verified.bl_access_token).toBe("backlog-access-token-xyz");
        expect(verified.space).toBe("mycompany");
        expect(verified.domain).toBe("backlog.jp");
    });

    it("sign → verify roundtrip for refresh token (no exp)", async () => {
        const { jwksJson } = await makeTestJWKS();
        const keys = await loadSigningKeys(jwksJson);
        const now = Math.floor(Date.now() / 1000);

        const payload: TokenPayload = {
            bl_refresh_token: "backlog-refresh-token-abc",
            space: "mycompany",
            domain: "backlog.jp",
            iat: now,
        };

        const jwt = await signToken(payload, keys.signingKey, keys.signingKid);
        const verified = await verifyToken(jwt, keys.verifyKeys);

        expect(verified.bl_refresh_token).toBe("backlog-refresh-token-abc");
        expect(verified.exp).toBeUndefined();
    });

    it("verify fails with wrong key", async () => {
        const { jwksJson: jwks1 } = await makeTestJWKS();
        const { jwksJson: jwks2 } = await makeTestJWKS();
        const keys1 = await loadSigningKeys(jwks1);
        const keys2 = await loadSigningKeys(jwks2);
        const now = Math.floor(Date.now() / 1000);

        const jwt = await signToken(
            { bl_access_token: "test", space: "s", domain: "d", iat: now, exp: now + 3600 },
            keys1.signingKey,
            keys1.signingKid,
        );

        await expect(verifyToken(jwt, keys2.verifyKeys)).rejects.toThrow();
    });

    it("verify fails with tampered JWT", async () => {
        const { jwksJson } = await makeTestJWKS();
        const keys = await loadSigningKeys(jwksJson);
        const now = Math.floor(Date.now() / 1000);

        const jwt = await signToken(
            { bl_access_token: "test", space: "s", domain: "d", iat: now, exp: now + 3600 },
            keys.signingKey,
            keys.signingKid,
        );

        const parts = jwt.split(".");
        parts[1] = parts[1].slice(0, -4) + "XXXX";
        const tampered = parts.join(".");

        await expect(verifyToken(tampered, keys.verifyKeys)).rejects.toThrow();
    });

    it("verify rejects expired token", async () => {
        const { jwksJson } = await makeTestJWKS();
        const keys = await loadSigningKeys(jwksJson);
        const now = Math.floor(Date.now() / 1000);

        const jwt = await signToken(
            { bl_access_token: "test", space: "s", domain: "d", iat: now - 120, exp: now - 60 },
            keys.signingKey,
            keys.signingKid,
        );

        await expect(verifyToken(jwt, keys.verifyKeys)).rejects.toThrow();
    });
});

describe("loadSigningKeys", () => {
    it("loads keys from JWKS JSON", async () => {
        const { jwksJson } = await makeTestJWKS();
        const keys = await loadSigningKeys(jwksJson);

        expect(keys.signingKey).toBeDefined();
        expect(keys.signingKid).toBe("test-key-1");
        expect(keys.verifyKeys.size).toBe(1);
        expect(keys.verifyKeys.has("test-key-1")).toBe(true);
    });

    it("throws on empty JWKS", async () => {
        await expect(loadSigningKeys(JSON.stringify({ keys: [] }))).rejects.toThrow("JWKS has no keys");
    });
});
