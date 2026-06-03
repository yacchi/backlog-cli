import { describe, it, expect } from "vitest";
import {
    encryptToken,
    decryptToken,
    generateKey,
    importKey,
    exportKey,
    type TokenPayload,
} from "./jwe.js";

describe("JWE token encryption", () => {
    const key = generateKey();

    const accessPayload: TokenPayload = {
        bl_access_token: "backlog-access-token-xyz",
        bl_expires_at: Math.floor(Date.now() / 1000) + 3600,
        space: "mycompany",
        domain: "backlog.jp",
        iat: Math.floor(Date.now() / 1000),
        exp: Math.floor(Date.now() / 1000) + 3600,
    };

    const refreshPayload: TokenPayload = {
        bl_refresh_token: "backlog-refresh-token-abc",
        space: "mycompany",
        domain: "backlog.jp",
        iat: Math.floor(Date.now() / 1000),
    };

    it("encrypt → decrypt roundtrip for access token", async () => {
        const jwe = await encryptToken(accessPayload, key);
        const decrypted = await decryptToken(jwe, key);

        expect(decrypted.bl_access_token).toBe("backlog-access-token-xyz");
        expect(decrypted.space).toBe("mycompany");
        expect(decrypted.domain).toBe("backlog.jp");
        expect(decrypted.bl_expires_at).toBe(accessPayload.bl_expires_at);
    });

    it("encrypt → decrypt roundtrip for refresh token", async () => {
        const jwe = await encryptToken(refreshPayload, key);
        const decrypted = await decryptToken(jwe, key);

        expect(decrypted.bl_refresh_token).toBe("backlog-refresh-token-abc");
        expect(decrypted.space).toBe("mycompany");
        expect(decrypted.exp).toBeUndefined();
    });

    it("decrypt fails with wrong key", async () => {
        const jwe = await encryptToken(accessPayload, key);
        const wrongKey = generateKey();

        await expect(decryptToken(jwe, wrongKey)).rejects.toThrow();
    });

    it("decrypt fails with tampered JWE", async () => {
        const jwe = await encryptToken(accessPayload, key);
        const parts = jwe.split(".");
        parts[3] = parts[3].slice(0, -4) + "XXXX";
        const tampered = parts.join(".");

        await expect(decryptToken(tampered, key)).rejects.toThrow();
    });

    it("decrypt rejects expired token", async () => {
        const expired: TokenPayload = {
            ...accessPayload,
            exp: Math.floor(Date.now() / 1000) - 60,
        };
        const jwe = await encryptToken(expired, key);

        await expect(decryptToken(jwe, key)).rejects.toThrow("token expired");
    });

    it("decrypt accepts token without exp", async () => {
        const jwe = await encryptToken(refreshPayload, key);
        const decrypted = await decryptToken(jwe, key);

        expect(decrypted.bl_refresh_token).toBe("backlog-refresh-token-abc");
    });
});

describe("key management", () => {
    it("generateKey returns 32 bytes", () => {
        const key = generateKey();
        expect(key).toBeInstanceOf(Uint8Array);
        expect(key.length).toBe(32);
    });

    it("exportKey → importKey roundtrip", () => {
        const key = generateKey();
        const exported = exportKey(key);
        const imported = importKey(exported);

        expect(imported).toEqual(key);
    });

    it("importKey rejects wrong length", () => {
        const short = btoa("tooshort").replace(/=+$/, "");
        expect(() => importKey(short)).toThrow("invalid key length");
    });
});
