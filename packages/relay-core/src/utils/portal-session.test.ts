import { describe, it, expect } from "vitest";
import {
    createPortalSessionToken,
    verifyPortalSessionToken,
    encryptRefreshToken,
    decryptRefreshToken,
} from "./portal-session.js";
import {
    base64UrlEncode,
    deriveEd25519PublicKey,
} from "./crypto.js";

async function makeTestJWKS(): Promise<string> {
    const seed = crypto.getRandomValues(new Uint8Array(32));
    const pubBytes = await deriveEd25519PublicKey(seed);
    const jwks = {
        keys: [
            {
                kty: "OKP",
                crv: "Ed25519",
                kid: "test-kid-1",
                d: base64UrlEncode(seed),
                x: base64UrlEncode(pubBytes),
            },
        ],
    };
    return JSON.stringify(jwks);
}

describe("portal session token", () => {
    it("round-trips create → verify", async () => {
        const jwksJson = await makeTestJWKS();
        const token = await createPortalSessionToken(
            { userId: "u1", name: "Test", email: "test@example.com" },
            "tenant1",
            "space.backlog.jp",
            jwksJson,
        );
        const claims = await verifyPortalSessionToken(token, jwksJson);
        expect(claims.sub).toBe("u1");
        expect(claims.name).toBe("Test");
        expect(claims.tenant).toBe("tenant1");
        expect(claims.space).toBe("space.backlog.jp");
        expect(claims.role).toBe(0);
        expect(claims.auth_time).toBeGreaterThan(0);
    });

    it("preserves roleType and authTime", async () => {
        const jwksJson = await makeTestJWKS();
        const authTime = 1700000000;
        const token = await createPortalSessionToken(
            { userId: "u1", name: "Admin", email: "admin@example.com", roleType: 1 },
            "tenant1",
            "space.backlog.jp",
            jwksJson,
            authTime,
        );
        const claims = await verifyPortalSessionToken(token, jwksJson);
        expect(claims.role).toBe(1);
        expect(claims.auth_time).toBe(authTime);
    });
});

describe("refresh token encryption", () => {
    it("round-trips encrypt → decrypt", async () => {
        const jwksJson = await makeTestJWKS();
        const sealed = await encryptRefreshToken(
            "rt_abc123",
            "space.backlog.jp",
            "tenant1",
            jwksJson,
        );
        const result = await decryptRefreshToken(sealed, jwksJson);
        expect(result.refreshToken).toBe("rt_abc123");
        expect(result.space).toBe("space.backlog.jp");
        expect(result.tenant).toBe("tenant1");
    });

    it("produces different ciphertexts for the same input (unique IV)", async () => {
        const jwksJson = await makeTestJWKS();
        const a = await encryptRefreshToken("rt_x", "s.backlog.jp", "t1", jwksJson);
        const b = await encryptRefreshToken("rt_x", "s.backlog.jp", "t1", jwksJson);
        expect(a).not.toBe(b);
    });

    it("produces JWE compact format (5 segments)", async () => {
        const jwksJson = await makeTestJWKS();
        const sealed = await encryptRefreshToken("rt_x", "s.backlog.jp", "t1", jwksJson);
        expect(sealed.split(".").length).toBe(5);
    });

    it("fails to decrypt with a different key", async () => {
        const jwks1 = await makeTestJWKS();
        const jwks2 = await makeTestJWKS();
        const sealed = await encryptRefreshToken("rt_y", "s.backlog.jp", "t1", jwks1);
        await expect(decryptRefreshToken(sealed, jwks2)).rejects.toThrow();
    });
});
