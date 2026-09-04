import { describe, it, expect } from "vitest";
import {
    signToken,
    verifyToken,
    sign,
    loadSigningKeys,
    spaceKey,
    setSpaceAccess,
    setSpaceRefresh,
    listSpaceEntries,
    type TokenPayload,
    type SpaceAccessEntry,
    type SpaceRefreshEntry,
} from "./jwt.js";
import { exportJWK, generateKeyPair } from "jose";

async function makeTestJWKS(): Promise<{ jwksJson: string; kid: string }> {
    const { privateKey } = await generateKeyPair("EdDSA", { crv: "Ed25519", extractable: true });
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

        const space = "mycompany.backlog.jp";
        const payload: Record<string, unknown> = {
            space,
            iat: now,
            exp: now + 3600,
        };
        setSpaceAccess(payload, space, "backlog-access-token-xyz", now + 3600);

        const jwt = await sign(payload, keys.signingKey, keys.signingKid);
        const verified = await verifyToken(jwt, keys.verifyKeys);

        const entries = listSpaceEntries(verified);
        expect(entries).toHaveLength(1);
        expect(entries[0][0]).toBe(space);
        expect((entries[0][1] as SpaceAccessEntry).at).toBe("backlog-access-token-xyz");
        expect(verified.space).toBe(space);
    });

    it("sign → verify roundtrip for refresh token (no exp)", async () => {
        const { jwksJson } = await makeTestJWKS();
        const keys = await loadSigningKeys(jwksJson);
        const now = Math.floor(Date.now() / 1000);

        const space = "mycompany.backlog.jp";
        const payload: Record<string, unknown> = {
            space,
            iat: now,
        };
        setSpaceRefresh(payload, space, "backlog-refresh-token-abc");

        const jwt = await sign(payload, keys.signingKey, keys.signingKid);
        const verified = await verifyToken(jwt, keys.verifyKeys);

        const entries = listSpaceEntries(verified);
        expect(entries).toHaveLength(1);
        expect((entries[0][1] as SpaceRefreshEntry).rt).toBe("backlog-refresh-token-abc");
        expect(verified.exp).toBeUndefined();
    });

    it("verifyToken normalizes Gen 1 bl_access_token to space:* entry", async () => {
        const { jwksJson } = await makeTestJWKS();
        const keys = await loadSigningKeys(jwksJson);
        const now = Math.floor(Date.now() / 1000);

        const payload: TokenPayload = {
            bl_access_token: "legacy-at",
            bl_expires_at: now + 3600,
            space: "legacy.backlog.jp",
            iat: now,
            exp: now + 3600,
        };

        const jwt = await signToken(payload, keys.signingKey, keys.signingKid);
        const verified = await verifyToken(jwt, keys.verifyKeys);

        expect(verified.bl_access_token).toBeUndefined();
        const entries = listSpaceEntries(verified);
        expect(entries).toHaveLength(1);
        expect(entries[0][0]).toBe("legacy.backlog.jp");
        expect((entries[0][1] as SpaceAccessEntry).at).toBe("legacy-at");
    });

    it("setSpaceAccess and setSpaceRefresh merge into one entry", async () => {
        const now = Math.floor(Date.now() / 1000);
        const space = "mycompany.backlog.jp";
        const payload: Record<string, unknown> = { space, iat: now };

        setSpaceAccess(payload, space, "at-1", now + 3600);
        setSpaceRefresh(payload, space, "rt-1");

        const entry = payload[spaceKey(space)] as SpaceAccessEntry & SpaceRefreshEntry;
        expect(entry.at).toBe("at-1");
        expect(entry.exp).toBe(now + 3600);
        expect(entry.rt).toBe("rt-1");

        // 逆順でも同じ結果になること
        const reversed: Record<string, unknown> = { space, iat: now };
        setSpaceRefresh(reversed, space, "rt-2");
        setSpaceAccess(reversed, space, "at-2", now + 60);
        const rev = reversed[spaceKey(space)] as SpaceAccessEntry & SpaceRefreshEntry;
        expect(rev.at).toBe("at-2");
        expect(rev.rt).toBe("rt-2");
        expect(rev.exp).toBe(now + 60);
    });

    it("verifyToken normalizes Gen 1 bl_refresh_token to space:* entry", async () => {
        const { jwksJson } = await makeTestJWKS();
        const keys = await loadSigningKeys(jwksJson);
        const now = Math.floor(Date.now() / 1000);

        const payload: TokenPayload = {
            bl_refresh_token: "legacy-rt",
            space: "legacy.backlog.jp",
            iat: now,
        };

        const jwt = await signToken(payload, keys.signingKey, keys.signingKid);
        const verified = await verifyToken(jwt, keys.verifyKeys);

        expect(verified.bl_refresh_token).toBeUndefined();
        const entries = listSpaceEntries(verified);
        expect(entries).toHaveLength(1);
        expect((entries[0][1] as SpaceRefreshEntry).rt).toBe("legacy-rt");
    });

    it("verifyToken keeps both at and rt when migrating legacy spaces array", async () => {
        const { jwksJson } = await makeTestJWKS();
        const keys = await loadSigningKeys(jwksJson);
        const now = Math.floor(Date.now() / 1000);

        const payload: TokenPayload = {
            spaces: [
                {
                    space: "one.backlog.jp",
                    bl_access_token: "at-one",
                    bl_refresh_token: "rt-one",
                    bl_expires_at: now + 3600,
                },
                {
                    space: "two.backlog.jp",
                    bl_access_token: "at-two",
                    bl_refresh_token: "rt-two",
                    bl_expires_at: now + 1800,
                },
            ],
            space: "one.backlog.jp",
            iat: now,
        };

        const jwt = await signToken(payload, keys.signingKey, keys.signingKid);
        const verified = await verifyToken(jwt, keys.verifyKeys);

        expect(verified.spaces).toBeUndefined();
        const entries = new Map(listSpaceEntries(verified));
        expect(entries.size).toBe(2);
        for (const [domain, at, rt, exp] of [
            ["one.backlog.jp", "at-one", "rt-one", now + 3600],
            ["two.backlog.jp", "at-two", "rt-two", now + 1800],
        ] as const) {
            const entry = entries.get(domain) as SpaceAccessEntry & SpaceRefreshEntry;
            expect(entry.at).toBe(at);
            expect(entry.rt).toBe(rt);
            expect(entry.exp).toBe(exp);
        }
    });

    it("verifyToken migrates legacy spaces array with refresh token only", async () => {
        const { jwksJson } = await makeTestJWKS();
        const keys = await loadSigningKeys(jwksJson);
        const now = Math.floor(Date.now() / 1000);

        const payload: TokenPayload = {
            spaces: [{ space: "only.backlog.jp", bl_refresh_token: "rt-only" } as never],
            space: "only.backlog.jp",
            iat: now,
        };

        const jwt = await signToken(payload, keys.signingKey, keys.signingKid);
        const verified = await verifyToken(jwt, keys.verifyKeys);

        const entries = listSpaceEntries(verified);
        expect(entries).toHaveLength(1);
        const entry = entries[0][1] as SpaceAccessEntry & SpaceRefreshEntry;
        expect(entry.rt).toBe("rt-only");
        expect(entry.at).toBeUndefined();
    });

    it("verify fails with wrong key", async () => {
        const { jwksJson: jwks1 } = await makeTestJWKS();
        const { jwksJson: jwks2 } = await makeTestJWKS();
        const keys1 = await loadSigningKeys(jwks1);
        const keys2 = await loadSigningKeys(jwks2);
        const now = Math.floor(Date.now() / 1000);

        const jwt = await signToken(
            { bl_access_token: "test", space: "s.d", iat: now, exp: now + 3600 },
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
            { bl_access_token: "test", space: "s.d", iat: now, exp: now + 3600 },
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
            { bl_access_token: "test", space: "s.d", iat: now - 120, exp: now - 60 },
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

    it("derives an enc key per kid that has a private scalar", async () => {
        const { jwksJson } = await makeTestJWKS();
        const keys = await loadSigningKeys(jwksJson);

        const enc = keys.encKeys.get("test-key-1");
        expect(enc).toBeDefined();
        expect(enc!.length).toBe(32);
    });

    it("does not derive an enc key for public-only (retired) keys", async () => {
        const { jwksJson } = await makeTestJWKS();
        const jwks = JSON.parse(jwksJson);
        const { x, kty, crv } = jwks.keys[0];
        jwks.keys.push({ kid: "retired-pub-only", kty, crv, x });

        const keys = await loadSigningKeys(JSON.stringify(jwks));
        expect(keys.verifyKeys.has("retired-pub-only")).toBe(true);
        expect(keys.encKeys.has("retired-pub-only")).toBe(false);
    });
});
