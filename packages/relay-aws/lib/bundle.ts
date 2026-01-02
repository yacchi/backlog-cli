/**
 * Bundle creation for portal configuration distribution.
 */

import JSZip from "jszip";
import YAML from "yaml";
import type { TenantConfig } from "@backlog-cli/relay-core";

const MANIFEST_NAME = "manifest.yaml";
const MANIFEST_SIG_NAME = "manifest.yaml.sig";
const BUNDLE_EXPIRY_DAYS = 30;

/**
 * JWK structure.
 */
interface JWK {
  kty: string;
  crv: string;
  kid: string;
  x?: string;
  d?: string;
}

/**
 * JWKS structure.
 */
interface JWKS {
  keys: JWK[];
}

/**
 * Relay bundle key reference.
 */
interface RelayBundleKey {
  key_id: string;
  thumbprint: string;
}

/**
 * Relay bundle manifest.
 */
interface RelayBundleManifest {
  version: number;
  relay_url: string;
  allowed_domain: string;
  issued_at: string;
  expires_at: string;
  bundle_token: string;
  relay_keys: RelayBundleKey[];
  files: Array<{ name: string; sha256: string }>;
}

/**
 * JWS signature structure.
 */
interface JWSSignature {
  protected: string;
  signature: string;
}

/**
 * JWS structure.
 */
interface JWS {
  payload: string;
  signatures: JWSSignature[];
}

/**
 * Extended tenant config with JWK fields.
 */
interface ExtendedTenantConfig extends TenantConfig {
  jwks?: string;
  activeKeys?: string;
}

/**
 * Base64URL encode.
 */
function base64UrlEncode(data: Uint8Array | string): string {
  const bytes =
    typeof data === "string" ? new TextEncoder().encode(data) : data;
  let binary = "";
  for (let i = 0; i < bytes.length; i++) {
    binary += String.fromCharCode(bytes[i]);
  }
  return btoa(binary).replace(/\+/g, "-").replace(/\//g, "_").replace(/=+$/, "");
}

/**
 * Base64URL decode.
 */
function base64UrlDecode(str: string): Uint8Array {
  const padded = str + "=".repeat((4 - (str.length % 4)) % 4);
  const base64 = padded.replace(/-/g, "+").replace(/_/g, "/");
  const binary = atob(base64);
  const bytes = new Uint8Array(binary.length);
  for (let i = 0; i < binary.length; i++) {
    bytes[i] = binary.charCodeAt(i);
  }
  return bytes;
}

/**
 * Generate random bytes.
 */
function randomBytes(length: number): Uint8Array {
  const bytes = new Uint8Array(length);
  crypto.getRandomValues(bytes);
  return bytes;
}

/**
 * Calculate JWK thumbprint (RFC 7638).
 * Uses canonical JSON format: {"crv":"Ed25519","kty":"OKP","x":"..."}
 */
async function jwkThumbprint(jwk: JWK): Promise<string> {
  if (jwk.kty !== "OKP" || jwk.crv !== "Ed25519") {
    throw new Error(`Unsupported JWK: kty=${jwk.kty} crv=${jwk.crv}`);
  }
  if (!jwk.x) {
    throw new Error("JWK missing x parameter");
  }

  // Canonical JSON format per RFC 7638
  const canonical = JSON.stringify({
    crv: jwk.crv,
    kty: jwk.kty,
    x: jwk.x,
  });

  const hashBuffer = await crypto.subtle.digest(
    "SHA-256",
    new TextEncoder().encode(canonical)
  );
  return base64UrlEncode(new Uint8Array(hashBuffer));
}

/**
 * Derive Ed25519 public key from private key seed.
 */
async function deriveEd25519PublicKey(seed: Uint8Array): Promise<Uint8Array> {
  const pkcs8Header = new Uint8Array([
    0x30, 0x2e, 0x02, 0x01, 0x00, 0x30, 0x05,
    0x06, 0x03, 0x2b, 0x65, 0x70, 0x04, 0x22, 0x04, 0x20,
  ]);

  const pkcs8Key = new Uint8Array(pkcs8Header.length + seed.length);
  pkcs8Key.set(pkcs8Header);
  pkcs8Key.set(seed, pkcs8Header.length);

  const privateKey = await crypto.subtle.importKey(
    "pkcs8",
    pkcs8Key,
    { name: "Ed25519" },
    true,
    ["sign"]
  );

  const exportedJwk = await crypto.subtle.exportKey("jwk", privateKey);
  if (!exportedJwk.x) {
    throw new Error("Failed to derive public key");
  }

  return base64UrlDecode(exportedJwk.x);
}

/**
 * Sign data with Ed25519.
 */
async function signEd25519(
  seed: Uint8Array,
  data: Uint8Array
): Promise<Uint8Array> {
  const pkcs8Header = new Uint8Array([
    0x30, 0x2e, 0x02, 0x01, 0x00, 0x30, 0x05,
    0x06, 0x03, 0x2b, 0x65, 0x70, 0x04, 0x22, 0x04, 0x20,
  ]);

  const pkcs8Key = new Uint8Array(pkcs8Header.length + seed.length);
  pkcs8Key.set(pkcs8Header);
  pkcs8Key.set(seed, pkcs8Header.length);

  const privateKey = await crypto.subtle.importKey(
    "pkcs8",
    pkcs8Key,
    { name: "Ed25519" },
    false,
    ["sign"]
  );

  // Create a new ArrayBuffer copy to satisfy TypeScript's stricter type checking
  const dataBuffer = new ArrayBuffer(data.byteLength);
  new Uint8Array(dataBuffer).set(data);
  const signature = await crypto.subtle.sign("Ed25519", privateKey, dataBuffer);
  return new Uint8Array(signature);
}

/**
 * Split comma-separated key list.
 */
function splitKeyList(value: string): string[] {
  return value
    .split(",")
    .map((s) => s.trim())
    .filter((s) => s !== "");
}

/**
 * Normalize JWKS by deriving public keys from private keys.
 */
async function normalizeJWKS(jwks: JWKS): Promise<Map<string, JWK>> {
  const jwkByKid = new Map<string, JWK>();

  for (const key of jwks.keys) {
    const normalized = { ...key };

    // If we have private key but no public key, derive it
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
 * Generate bundle token JWT.
 */
async function generateBundleToken(
  allowedDomain: string,
  jwkByKid: Map<string, JWK>,
  activeKeys: string[],
  now: Date
): Promise<string> {
  if (activeKeys.length === 0) {
    throw new Error("No active keys for signing");
  }

  const kid = activeKeys[0];
  const jwk = jwkByKid.get(kid);
  if (!jwk) {
    throw new Error(`JWKS missing key for token signing: ${kid}`);
  }
  if (!jwk.d) {
    throw new Error(`JWKS key ${kid} missing private key`);
  }

  // Generate JTI
  const jtiBytes = randomBytes(16);
  const jti = base64UrlEncode(jtiBytes);

  // Header
  const header = {
    alg: "EdDSA",
    typ: "JWT",
    kid,
  };

  // Claims
  const claims = {
    sub: allowedDomain,
    iat: Math.floor(now.getTime() / 1000),
    nbf: Math.floor(now.getTime() / 1000),
    jti,
  };

  const headerB64 = base64UrlEncode(JSON.stringify(header));
  const claimsB64 = base64UrlEncode(JSON.stringify(claims));
  const signingInput = headerB64 + "." + claimsB64;

  const seed = base64UrlDecode(jwk.d);
  const signature = await signEd25519(
    seed,
    new TextEncoder().encode(signingInput)
  );
  const signatureB64 = base64UrlEncode(signature);

  return signingInput + "." + signatureB64;
}

/**
 * Build manifest relay keys with thumbprints.
 */
async function buildManifestRelayKeys(
  jwkByKid: Map<string, JWK>,
  activeKeys: string[]
): Promise<RelayBundleKey[]> {
  const keys: RelayBundleKey[] = [];

  for (const kid of activeKeys) {
    const jwk = jwkByKid.get(kid);
    if (!jwk) {
      throw new Error(`JWKS missing key: ${kid}`);
    }
    const thumbprint = await jwkThumbprint(jwk);
    keys.push({
      key_id: kid,
      thumbprint,
    });
  }

  return keys;
}

/**
 * Sign manifest as JWS.
 */
async function signManifest(
  manifestBytes: Uint8Array,
  jwkByKid: Map<string, JWK>,
  activeKeys: string[]
): Promise<string> {
  const payload = base64UrlEncode(manifestBytes);
  const signatures: JWSSignature[] = [];

  for (const kid of activeKeys) {
    const jwk = jwkByKid.get(kid);
    if (!jwk) {
      throw new Error(`JWKS missing key for signing: ${kid}`);
    }
    if (!jwk.d) {
      throw new Error(`JWKS key ${kid} missing private key`);
    }

    const protectedHeader = { alg: "EdDSA", kid };
    const protectedB64 = base64UrlEncode(JSON.stringify(protectedHeader));
    const signingInput = protectedB64 + "." + payload;

    const seed = base64UrlDecode(jwk.d);
    const signature = await signEd25519(
      seed,
      new TextEncoder().encode(signingInput)
    );

    signatures.push({
      protected: protectedB64,
      signature: base64UrlEncode(signature),
    });
  }

  const jws: JWS = { payload, signatures };
  return JSON.stringify(jws, null, 2);
}

/**
 * Create a configuration bundle for a tenant.
 */
export async function createBundle(
  tenant: TenantConfig,
  allowedDomain: string,
  relayUrl: string
): Promise<Uint8Array> {
  const extendedTenant = tenant as ExtendedTenantConfig;

  if (!extendedTenant.jwks) {
    throw new Error("Tenant JWKS is not configured");
  }
  if (!extendedTenant.activeKeys) {
    throw new Error("Tenant active keys are not configured");
  }

  const now = new Date();
  const expiresAt = new Date(
    now.getTime() + BUNDLE_EXPIRY_DAYS * 24 * 60 * 60 * 1000
  );

  // Parse and normalize JWKS
  const jwks: JWKS = JSON.parse(extendedTenant.jwks);
  const jwkByKid = await normalizeJWKS(jwks);

  // Get active key IDs
  const activeKeyIds = splitKeyList(extendedTenant.activeKeys);
  if (activeKeyIds.length === 0) {
    throw new Error("No active keys for tenant");
  }

  // Build manifest relay keys with thumbprints
  const relayKeys = await buildManifestRelayKeys(jwkByKid, activeKeyIds);

  // Generate bundle token
  const bundleToken = await generateBundleToken(
    allowedDomain,
    jwkByKid,
    activeKeyIds,
    now
  );

  // Create manifest
  const manifest: RelayBundleManifest = {
    version: 1,
    relay_url: relayUrl,
    allowed_domain: allowedDomain,
    issued_at: now.toISOString(),
    expires_at: expiresAt.toISOString(),
    bundle_token: bundleToken,
    relay_keys: relayKeys,
    files: [],
  };

  // Serialize manifest to YAML
  const manifestYaml = YAML.stringify(manifest);
  const manifestBytes = new TextEncoder().encode(manifestYaml);

  // Sign manifest
  const jwsJson = await signManifest(manifestBytes, jwkByKid, activeKeyIds);
  const jwsBytes = new TextEncoder().encode(jwsJson);

  // Create ZIP
  const zip = new JSZip();
  zip.file(MANIFEST_NAME, manifestBytes);
  zip.file(MANIFEST_SIG_NAME, jwsBytes);

  const zipData = await zip.generateAsync({
    type: "uint8array",
    compression: "DEFLATE",
    compressionOptions: { level: 6 },
  });

  return zipData;
}
