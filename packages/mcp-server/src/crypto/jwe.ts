import { CompactEncrypt, compactDecrypt } from "jose";

export interface TokenPayload {
    bl_access_token?: string;
    bl_refresh_token?: string;
    bl_expires_at?: number;
    space: string;
    domain: string;
    iat: number;
    exp?: number;
}

const ALG = "dir";
const ENC = "A256GCM";

export async function encryptToken(
    payload: TokenPayload,
    key: Uint8Array,
): Promise<string> {
    const encoder = new TextEncoder();
    const jwe = await new CompactEncrypt(encoder.encode(JSON.stringify(payload)))
        .setProtectedHeader({ alg: ALG, enc: ENC })
        .encrypt(key);
    return jwe;
}

export async function decryptToken(
    jwe: string,
    key: Uint8Array,
): Promise<TokenPayload> {
    const { plaintext } = await compactDecrypt(jwe, key, {
        keyManagementAlgorithms: [ALG],
        contentEncryptionAlgorithms: [ENC],
    });
    const payload = JSON.parse(new TextDecoder().decode(plaintext)) as TokenPayload;

    if (payload.exp != null && payload.exp < Math.floor(Date.now() / 1000)) {
        throw new Error("token expired");
    }

    return payload;
}

export function generateKey(): Uint8Array {
    const key = new Uint8Array(32);
    crypto.getRandomValues(key);
    return key;
}

export function importKey(base64url: string): Uint8Array {
    const padded = base64url.replace(/-/g, "+").replace(/_/g, "/");
    const binary = atob(padded);
    const bytes = new Uint8Array(binary.length);
    for (let i = 0; i < binary.length; i++) {
        bytes[i] = binary.charCodeAt(i);
    }
    if (bytes.length !== 32) {
        throw new Error(`invalid key length: expected 32 bytes, got ${bytes.length}`);
    }
    return bytes;
}

export function exportKey(key: Uint8Array): string {
    let binary = "";
    for (const byte of key) {
        binary += String.fromCharCode(byte);
    }
    return btoa(binary).replace(/\+/g, "-").replace(/\//g, "_").replace(/=+$/, "");
}
