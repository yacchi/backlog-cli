import { CompactEncrypt, compactDecrypt } from "jose";
import { hkdfSync } from "node:crypto";

const ENC = "A256GCM";
/** HKDF info label — bump the version suffix if the enc algorithm changes. */
const HKDF_INFO = "backlog-mcp:token-enc:A256GCM:v1";

export type TokenUse = "at" | "rt";

/**
 * Raised when a sealed value cannot be opened (wrong/unknown key, tampering,
 * legacy plaintext, or header mismatch). Callers must map this to a
 * re-authentication response (401 / invalid_grant), never a 500.
 */
export class DecryptError extends Error {
    constructor(message: string, options?: { cause?: unknown }) {
        super(message, options);
        this.name = "DecryptError";
    }
}

/**
 * Derive a 32-byte AES-256-GCM key from an Ed25519 signing key's private
 * scalar (the JWK "d" value, base64url). The encryption key is bound to the
 * same key material (and therefore the same `kid`) as the signing key, so no
 * separate secret needs to be provisioned or rotated.
 */
export function deriveEncKey(dBase64url: string): Uint8Array {
    const ikm = Buffer.from(dBase64url, "base64url");
    const derived = hkdfSync("sha256", ikm, new Uint8Array(0), HKDF_INFO, 32);
    return new Uint8Array(derived as ArrayBuffer);
}

/**
 * Encrypt a secret value (raw Backlog access/refresh token) into a JWE compact
 * string. `sp` (space) and `use` go into the protected header so they are
 * covered by the AEAD tag, binding the ciphertext to its slot.
 */
export async function seal(
    plain: string,
    key: Uint8Array,
    kid: string,
    sp: string,
    use: TokenUse,
): Promise<string> {
    return new CompactEncrypt(new TextEncoder().encode(plain))
        .setProtectedHeader({ alg: "dir", enc: ENC, kid, sp, use })
        .encrypt(key);
}

/**
 * Decrypt a JWE compact string produced by {@link seal}. The key is resolved
 * by the header `kid`. When `expected` is provided, the `sp`/`use` header
 * params must match. Any failure throws {@link DecryptError}.
 */
export async function open(
    jwe: string,
    keyForKid: (kid: string) => Uint8Array | undefined,
    expected?: { sp?: string; use?: TokenUse },
): Promise<string> {
    let plaintext: Uint8Array;
    let header: Record<string, unknown>;
    try {
        const result = await compactDecrypt(jwe, (protectedHeader) => {
            const kid = protectedHeader.kid;
            if (!kid) throw new DecryptError("JWE missing kid header");
            const key = keyForKid(kid);
            if (!key) throw new DecryptError(`unknown enc kid: ${kid}`);
            return key;
        });
        plaintext = result.plaintext;
        header = result.protectedHeader as Record<string, unknown>;
    } catch (err) {
        if (err instanceof DecryptError) throw err;
        throw new DecryptError("failed to decrypt token", { cause: err });
    }

    if (expected?.sp !== undefined && header.sp !== expected.sp) {
        throw new DecryptError("JWE sp mismatch");
    }
    if (expected?.use !== undefined && header.use !== expected.use) {
        throw new DecryptError("JWE use mismatch");
    }

    return new TextDecoder().decode(plaintext);
}
