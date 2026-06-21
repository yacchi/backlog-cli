import { describe, it, expect } from "vitest";
import { seal, open, deriveEncKey, DecryptError } from "./secret.js";

const D1 = Buffer.from("private-scalar-material-one").toString("base64url");
const D2 = Buffer.from("private-scalar-material-two").toString("base64url");

describe("deriveEncKey", () => {
    it("derives a 32-byte key deterministically", async () => {
        const k1 = await deriveEncKey(D1);
        const k2 = await deriveEncKey(D1);
        expect(k1.length).toBe(32);
        expect(Buffer.from(k1).equals(Buffer.from(k2))).toBe(true);
    });

    it("different d yields different key", async () => {
        const k1 = await deriveEncKey(D1);
        const k2 = await deriveEncKey(D2);
        expect(Buffer.from(k1).equals(Buffer.from(k2))).toBe(false);
    });
});

describe("seal/open", () => {
    it("roundtrips a secret value", async () => {
        const key = await deriveEncKey(D1);
        const jwe = await seal("backlog-access-token-xyz", key, "kid-1", "x.backlog.jp", "at");
        const out = await open(jwe, () => key);
        expect(out).toBe("backlog-access-token-xyz");
    });

    it("produces JWE compact (5 segments), not plaintext", async () => {
        const key = await deriveEncKey(D1);
        const jwe = await seal("secret", key, "kid-1", "x.backlog.jp", "rt");
        expect(jwe.split(".").length).toBe(5);
        expect(jwe).not.toContain("secret");
    });

    it("fails with DecryptError on wrong key", async () => {
        const jwe = await seal("secret", await deriveEncKey(D1), "kid-1", "x.backlog.jp", "at");
        const wrongKey = await deriveEncKey(D2);
        await expect(open(jwe, () => wrongKey)).rejects.toBeInstanceOf(DecryptError);
    });

    it("fails with DecryptError on unknown kid", async () => {
        const key = await deriveEncKey(D1);
        const jwe = await seal("secret", key, "kid-1", "x.backlog.jp", "at");
        await expect(open(jwe, () => undefined)).rejects.toBeInstanceOf(DecryptError);
    });

    it("fails with DecryptError on tampered ciphertext", async () => {
        const key = await deriveEncKey(D1);
        const jwe = await seal("secret", key, "kid-1", "x.backlog.jp", "at");
        const parts = jwe.split(".");
        parts[3] = parts[3].slice(0, -4) + "AAAA";
        await expect(open(parts.join("."), () => key)).rejects.toBeInstanceOf(DecryptError);
    });

    it("resolves the key by kid from the header", async () => {
        const key1 = await deriveEncKey(D1);
        const key2 = await deriveEncKey(D2);
        const jwe = await seal("secret", key2, "kid-2", "x.backlog.jp", "at");
        const resolver = (kid: string) => (kid === "kid-2" ? key2 : key1);
        expect(await open(jwe, resolver)).toBe("secret");
    });

    it("validates sp/use when expected and rejects mismatch", async () => {
        const key = await deriveEncKey(D1);
        const jwe = await seal("secret", key, "kid-1", "x.backlog.jp", "at");
        expect(await open(jwe, () => key, { sp: "x.backlog.jp", use: "at" })).toBe("secret");
        await expect(open(jwe, () => key, { sp: "evil.backlog.jp" })).rejects.toBeInstanceOf(DecryptError);
        await expect(open(jwe, () => key, { use: "rt" })).rejects.toBeInstanceOf(DecryptError);
    });
});
