/**
 * Shared Ed25519 / base64url / JWKS utilities.
 *
 * Used by bundle.ts, info.ts, and portal-session.ts.
 */

export interface JWK {
  kty: string;
  crv: string;
  kid: string;
  x?: string;
  d?: string;
}

export interface JWKS {
  keys: JWK[];
}

export function base64UrlEncode(data: Uint8Array | string): string {
  const bytes =
    typeof data === "string" ? new TextEncoder().encode(data) : data;
  let binary = "";
  for (let i = 0; i < bytes.length; i++) {
    binary += String.fromCharCode(bytes[i]);
  }
  return btoa(binary).replace(/\+/g, "-").replace(/\//g, "_").replace(/=+$/, "");
}

export function base64UrlDecode(str: string): Uint8Array {
  const padded = str + "=".repeat((4 - (str.length % 4)) % 4);
  const base64 = padded.replace(/-/g, "+").replace(/_/g, "/");
  const binary = atob(base64);
  const bytes = new Uint8Array(binary.length);
  for (let i = 0; i < binary.length; i++) {
    bytes[i] = binary.charCodeAt(i);
  }
  return bytes;
}

export function randomBytes(length: number): Uint8Array {
  const bytes = new Uint8Array(length);
  crypto.getRandomValues(bytes);
  return bytes;
}

/**
 * Import Ed25519 private key from raw 32-byte seed in PKCS8 format.
 */
function buildPkcs8Key(seed: Uint8Array): Uint8Array {
  const pkcs8Header = new Uint8Array([
    0x30, 0x2e, 0x02, 0x01, 0x00, 0x30, 0x05,
    0x06, 0x03, 0x2b, 0x65, 0x70, 0x04, 0x22, 0x04, 0x20,
  ]);
  const pkcs8Key = new Uint8Array(pkcs8Header.length + seed.length);
  pkcs8Key.set(pkcs8Header);
  pkcs8Key.set(seed, pkcs8Header.length);
  return pkcs8Key;
}

export async function signEd25519(
  seed: Uint8Array,
  data: Uint8Array,
): Promise<Uint8Array> {
  const privateKey = await crypto.subtle.importKey(
    "pkcs8",
    buildPkcs8Key(seed),
    { name: "Ed25519" },
    false,
    ["sign"],
  );
  const dataBuffer = new ArrayBuffer(data.byteLength);
  new Uint8Array(dataBuffer).set(data);
  const signature = await crypto.subtle.sign("Ed25519", privateKey, dataBuffer);
  return new Uint8Array(signature);
}

export async function verifyEd25519(
  publicKeyBytes: Uint8Array,
  signature: Uint8Array,
  data: Uint8Array,
): Promise<boolean> {
  const publicKey = await crypto.subtle.importKey(
    "raw",
    publicKeyBytes,
    { name: "Ed25519" },
    false,
    ["verify"],
  );
  return crypto.subtle.verify("Ed25519", publicKey, signature, data);
}

export async function deriveEd25519PublicKey(seed: Uint8Array): Promise<Uint8Array> {
  const privateKey = await crypto.subtle.importKey(
    "pkcs8",
    buildPkcs8Key(seed),
    { name: "Ed25519" },
    true,
    ["sign"],
  );
  const exportedJwk = await crypto.subtle.exportKey("jwk", privateKey);
  if (!exportedJwk.x) {
    throw new Error("Failed to derive public key");
  }
  return base64UrlDecode(exportedJwk.x);
}

/**
 * Calculate JWK thumbprint (RFC 7638) for OKP/Ed25519.
 */
export async function jwkThumbprint(jwk: JWK): Promise<string> {
  if (jwk.kty !== "OKP" || jwk.crv !== "Ed25519") {
    throw new Error(`Unsupported JWK: kty=${jwk.kty} crv=${jwk.crv}`);
  }
  if (!jwk.x) {
    throw new Error("JWK missing x parameter");
  }
  const canonical = JSON.stringify({ crv: jwk.crv, kty: jwk.kty, x: jwk.x });
  const hashBuffer = await crypto.subtle.digest(
    "SHA-256",
    new TextEncoder().encode(canonical),
  );
  return base64UrlEncode(new Uint8Array(hashBuffer));
}

/**
 * Normalize JWKS by deriving public keys from private keys where missing.
 */
export async function normalizeJWKS(jwks: JWKS): Promise<Map<string, JWK>> {
  const jwkByKid = new Map<string, JWK>();
  for (const key of jwks.keys) {
    const normalized = { ...key };
    if (normalized.d && !normalized.x) {
      const seed = base64UrlDecode(normalized.d);
      const pubKey = await deriveEd25519PublicKey(seed);
      normalized.x = base64UrlEncode(pubKey);
    }
    if (normalized.kid) {
      jwkByKid.set(normalized.kid, normalized);
    }
  }
  return jwkByKid;
}

/**
 * Get the first active signing key from JWKS.
 */
export function getFirstSigningKey(
  jwks: JWKS,
  jwkByKid: Map<string, JWK>,
): { kid: string; jwk: JWK } {
  const kid = jwks.keys
    .map((k) => k.kid)
    .filter((kid): kid is string => !!kid)[0];
  if (!kid) {
    throw new Error("No keys with kid in JWKS");
  }
  const jwk = jwkByKid.get(kid);
  if (!jwk) {
    throw new Error(`JWKS missing key: ${kid}`);
  }
  if (!jwk.d) {
    throw new Error(`JWKS key ${kid} missing private key`);
  }
  return { kid, jwk };
}

// --- JWE token encryption (mirrors mcp-server/src/crypto/secret.ts) ---

import { CompactEncrypt, compactDecrypt } from "jose";

const ENC = "A256GCM";
const HKDF_INFO = "backlog-mcp:token-enc:A256GCM:v1";

export type TokenUse = "at" | "rt" | "dl";

export class DecryptError extends Error {
  constructor(message: string, options?: { cause?: unknown }) {
    super(message, options);
    this.name = "DecryptError";
  }
}

/**
 * Derive a 32-byte AES-256-GCM key from an Ed25519 JWK "d" value (base64url).
 * Uses Web Crypto HKDF (cross-runtime) instead of node:crypto hkdfSync.
 */
export async function deriveEncKey(dBase64url: string): Promise<Uint8Array> {
  const ikm = base64UrlDecode(dBase64url);
  const baseKey = await crypto.subtle.importKey("raw", ikm, "HKDF", false, [
    "deriveBits",
  ]);
  const derived = await crypto.subtle.deriveBits(
    {
      name: "HKDF",
      hash: "SHA-256",
      salt: new Uint8Array(0),
      info: new TextEncoder().encode(HKDF_INFO),
    },
    baseKey,
    256,
  );
  return new Uint8Array(derived);
}

/**
 * Encrypt a secret value into a JWE compact string.
 * Same format as mcp-server's seal — sp/use are AEAD-bound via the protected header.
 */
export async function seal(
  plain: string,
  key: Uint8Array,
  kid: string,
  sp: string,
  use: TokenUse,
): Promise<string> {
  return new CompactEncrypt(new TextEncoder().encode(plain))
    .setProtectedHeader({ alg: "dir", enc: ENC, kid, sp, use } as Parameters<CompactEncrypt["setProtectedHeader"]>[0])
    .encrypt(key);
}

/**
 * Decrypt a JWE compact string produced by {@link seal}.
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
