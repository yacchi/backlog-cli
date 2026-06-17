/**
 * Bundle creation utilities for portal configuration distribution.
 */

import { zipSync } from "fflate";
import type { TenantConfig } from "../config/types.js";
import {
  type JWK,
  type JWKS,
  base64UrlEncode,
  base64UrlDecode,
  randomBytes,
  signEd25519,
  jwkThumbprint,
  normalizeJWKS,
  getFirstSigningKey,
} from "./crypto.js";

const MANIFEST_NAME = "manifest.yaml";
const MANIFEST_SIG_NAME = "manifest.yaml.sig";
const BUNDLE_EXPIRY_DAYS = 30;

/**
 * Relay bundle key reference.
 */
interface RelayBundleKey {
  key_id: string;
  thumbprint: string;
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
 * User who issued a bundle or provisioning token.
 */
export interface IssuedByInfo {
  user_id: string;
  name: string;
  email: string;
}

/**
 * Simple YAML stringifier for manifest.
 * Handles basic types: string, number, boolean, array, object.
 */
function stringifyYaml(obj: unknown, indent = 0): string {
  const spaces = "  ".repeat(indent);

  if (obj === null || obj === undefined) {
    return "null";
  }

  if (typeof obj === "string") {
    // Quote strings that contain special characters
    if (/[:\-#\[\]{}"'\n]/.test(obj) || obj === "") {
      return JSON.stringify(obj);
    }
    return obj;
  }

  if (typeof obj === "number" || typeof obj === "boolean") {
    return String(obj);
  }

  if (Array.isArray(obj)) {
    if (obj.length === 0) return "[]";
    return obj
      .map((item) => {
        const value = stringifyYaml(item, indent + 1);
        if (typeof item === "object" && item !== null && !Array.isArray(item)) {
          return `${spaces}- ${value.trimStart()}`;
        }
        return `${spaces}- ${value}`;
      })
      .join("\n");
  }

  if (typeof obj === "object") {
    const entries = Object.entries(obj);
    if (entries.length === 0) return "{}";
    return entries
      .map(([key, value]) => {
        if (typeof value === "object" && value !== null) {
          if (Array.isArray(value) && value.length === 0) {
            return `${spaces}${key}: []`;
          }
          if (
            !Array.isArray(value) &&
            Object.keys(value as object).length === 0
          ) {
            return `${spaces}${key}: {}`;
          }
          return `${spaces}${key}:\n${stringifyYaml(value, indent + 1)}`;
        }
        return `${spaces}${key}: ${stringifyYaml(value, indent)}`;
      })
      .join("\n");
  }

  return String(obj);
}


/**
 * Generate bundle token JWT.
 */
async function generateBundleToken(
  name: string,
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
    sub: name,
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
 * Provisioning token expiry in seconds (15 minutes).
 */
const PROVISIONING_TOKEN_EXPIRY_SECONDS = 15 * 60;

/**
 * Generate a provisioning token JWT for CLI setup.
 *
 * The token is self-contained: it carries relay_url and the bundle name
 * so the CLI can decode it (unverified) to discover where to fetch the bundle,
 * then verify the signature via JWKS before proceeding.
 * スペース非依存のため space/domain は持たない（スペースはログイン時に選択する）。
 */
export async function generateProvisioningToken(
  tenant: TenantConfig,
  name: string,
  relayUrl: string,
  jwksJson?: string,
  issuedBy?: IssuedByInfo,
): Promise<string> {
  const jwksStr = jwksJson ?? (tenant as TenantConfig & { jwks?: string }).jwks;
  if (!jwksStr) {
    throw new Error("JWKS is not configured");
  }

  const jwks: JWKS = JSON.parse(jwksStr);
  const jwkByKid = await normalizeJWKS(jwks);
  const { kid, jwk } = getFirstSigningKey(jwks, jwkByKid);

  const now = Math.floor(Date.now() / 1000);
  const jti = base64UrlEncode(randomBytes(16));

  const header = { alg: "EdDSA", typ: "JWT", kid };

  const claims: Record<string, unknown> = {
    sub: name,
    relay_url: relayUrl,
    name,
    purpose: "provision",
    iat: now,
    nbf: now,
    exp: now + PROVISIONING_TOKEN_EXPIRY_SECONDS,
    jti,
  };
  if (issuedBy) {
    claims.issued_by = issuedBy;
  }

  const headerB64 = base64UrlEncode(JSON.stringify(header));
  const claimsB64 = base64UrlEncode(JSON.stringify(claims));
  const signingInput = headerB64 + "." + claimsB64;

  const seed = base64UrlDecode(jwk.d!);
  const signature = await signEd25519(
    seed,
    new TextEncoder().encode(signingInput),
  );

  return signingInput + "." + base64UrlEncode(signature);
}

/**
 * Create a configuration bundle for a tenant.
 */
export async function createBundle(
  tenant: TenantConfig,
  name: string,
  relayUrl: string,
  jwksJson?: string,
  issuedBy?: IssuedByInfo,
): Promise<Uint8Array> {
  const jwksStr = jwksJson ?? (tenant as TenantConfig & { jwks?: string }).jwks;
  if (!jwksStr) {
    throw new Error("JWKS is not configured");
  }

  const now = new Date();
  const expiresAt = new Date(
    now.getTime() + BUNDLE_EXPIRY_DAYS * 24 * 60 * 60 * 1000
  );

  // Parse and normalize JWKS
  const jwks: JWKS = JSON.parse(jwksStr);
  const jwkByKid = await normalizeJWKS(jwks);

  // Use keys[0] as signing key
  const activeKeyIds = jwks.keys
    .map((k) => k.kid)
    .filter((kid): kid is string => !!kid)
    .slice(0, 1);
  if (activeKeyIds.length === 0) {
    throw new Error("No keys with kid in JWKS");
  }

  // Build manifest relay keys with thumbprints
  const relayKeys = await buildManifestRelayKeys(jwkByKid, activeKeyIds);

  // Generate bundle token
  const bundleToken = await generateBundleToken(
    name,
    jwkByKid,
    activeKeyIds,
    now
  );

  // Create manifest
  const manifest: Record<string, unknown> = {
    version: 2,
    name,
    relay_url: relayUrl,
    issued_at: now.toISOString(),
    expires_at: expiresAt.toISOString(),
    bundle_token: bundleToken,
    relay_keys: relayKeys,
    files: [],
  };
  if (issuedBy) {
    manifest.issued_by = issuedBy;
  }

  // Serialize manifest to YAML (simple format, no external dependency)
  const manifestYaml = stringifyYaml(manifest);
  const manifestBytes = new TextEncoder().encode(manifestYaml);

  // Sign manifest
  const jwsJson = await signManifest(manifestBytes, jwkByKid, activeKeyIds);
  const jwsBytes = new TextEncoder().encode(jwsJson);

  // Create ZIP using fflate (ESM native)
  const zipData = zipSync(
    {
      [MANIFEST_NAME]: manifestBytes,
      [MANIFEST_SIG_NAME]: jwsBytes,
    },
    { level: 6 }
  );

  return zipData;
}
