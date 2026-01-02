/**
 * Relay info handlers for signed relay information.
 */

import { Hono } from "hono";
import type { RelayConfig, TenantConfig } from "../config/types.js";
import { extractRequestContext } from "../utils/request.js";

const RELAY_INFO_VERSION = 1;
const RELAY_INFO_DEFAULT_TTL = 600;

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
 * Relay info payload.
 */
interface RelayInfoPayload {
  version: number;
  relay_url: string;
  allowed_domain: string;
  space: string;
  domain: string;
  issued_at: string;
  expires_at: string;
  update_before?: string;
}

/**
 * Relay info signature.
 */
interface RelayInfoSignature {
  protected: string;
  signature: string;
}

/**
 * Relay info response.
 */
interface RelayInfoResponse {
  payload: string;
  signatures: RelayInfoSignature[];
  payload_decoded: RelayInfoPayload;
}

/**
 * Protected header.
 */
interface ProtectedHeader {
  alg: string;
  kid: string;
}

/**
 * Extended tenant config with JWK fields.
 */
interface ExtendedTenantConfig extends TenantConfig {
  jwks?: string;
  activeKeys?: string;
  infoTtl?: number;
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
 * Compute ETag from JWKS and activeKeys.
 */
async function computeInfoETag(
  jwks: string,
  activeKeys: string
): Promise<string> {
  const data = new TextEncoder().encode(jwks + "|" + activeKeys);
  const hashBuffer = await crypto.subtle.digest("SHA-256", data);
  const hashArray = Array.from(new Uint8Array(hashBuffer));
  return (
    '"' +
    hashArray
      .slice(0, 16)
      .map((b) => b.toString(16).padStart(2, "0"))
      .join("") +
    '"'
  );
}

/**
 * Split domain into space and backlog domain.
 */
function splitDomain(domain: string): { space: string; backlogDomain: string } {
  const parts = domain.split(".");
  if (parts.length < 3) {
    return { space: "", backlogDomain: domain };
  }
  const space = parts[0];
  const backlogDomain = parts.slice(1).join(".");
  return { space, backlogDomain };
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
 * Find tenant by allowed domain.
 */
function findTenant(
  tenants: TenantConfig[] | undefined,
  domain: string
): ExtendedTenantConfig | undefined {
  return tenants?.find(
    (t) => t.allowedDomain.toLowerCase() === domain.toLowerCase()
  ) as ExtendedTenantConfig | undefined;
}

/**
 * Build relay URL from config and request context.
 */
function buildRelayUrl(
  baseUrl: string | undefined,
  reqBaseUrl: string
): string {
  return baseUrl || reqBaseUrl;
}

/**
 * Sign payload with Ed25519 key using Web Crypto API.
 * Uses PKCS8 format to import the private key seed.
 */
async function signEd25519(
  seed: Uint8Array,
  data: Uint8Array
): Promise<Uint8Array> {
  // Ed25519 PKCS8 format header
  const pkcs8Header = new Uint8Array([
    0x30, 0x2e, // SEQUENCE, length 46
    0x02, 0x01, 0x00, // INTEGER 0 (version)
    0x30, 0x05, // SEQUENCE, length 5
    0x06, 0x03, 0x2b, 0x65, 0x70, // OID 1.3.101.112 (Ed25519)
    0x04, 0x22, // OCTET STRING, length 34
    0x04, 0x20, // OCTET STRING, length 32 (the seed)
  ]);

  const pkcs8Key = new Uint8Array(pkcs8Header.length + seed.length);
  pkcs8Key.set(pkcs8Header);
  pkcs8Key.set(seed, pkcs8Header.length);

  // Import as PKCS8 private key
  const privateKey = await crypto.subtle.importKey(
    "pkcs8",
    pkcs8Key,
    { name: "Ed25519" },
    false,
    ["sign"]
  );

  // Sign the data
  const signature = await crypto.subtle.sign("Ed25519", privateKey, data);
  return new Uint8Array(signature);
}

/**
 * Build signatures for relay info.
 */
async function buildRelayInfoSignatures(
  payloadB64: string,
  jwksJson: string,
  activeKeys: string
): Promise<RelayInfoSignature[]> {
  const activeKeyIds = splitKeyList(activeKeys);
  if (activeKeyIds.length === 0) {
    throw new Error("active_keys is empty");
  }

  const jwks: JWKS = JSON.parse(jwksJson);
  const jwkByKid = new Map<string, JWK>();
  for (const key of jwks.keys) {
    if (key.kid) {
      jwkByKid.set(key.kid, key);
    }
  }

  const signatures: RelayInfoSignature[] = [];

  for (const kid of activeKeyIds) {
    const jwk = jwkByKid.get(kid);
    if (!jwk) {
      throw new Error(`jwks missing key: ${kid}`);
    }
    if (!jwk.d) {
      throw new Error(`jwk ${kid} missing private key`);
    }

    const protectedHeader: ProtectedHeader = {
      alg: "EdDSA",
      kid,
    };
    const protectedB64 = base64UrlEncode(JSON.stringify(protectedHeader));
    const signingInput = protectedB64 + "." + payloadB64;

    // Get private key seed
    const seed = base64UrlDecode(jwk.d);

    try {
      const sigBytes = await signEd25519(
        seed,
        new TextEncoder().encode(signingInput)
      );
      signatures.push({
        protected: protectedB64,
        signature: base64UrlEncode(sigBytes),
      });
    } catch {
      // Ed25519 signing not supported in this environment
      // Return empty signatures for now
      throw new Error(
        "Ed25519 signing not supported. Relay info endpoint requires Ed25519 support."
      );
    }
  }

  return signatures;
}

/**
 * Create relay info handlers.
 */
export function createInfoHandlers(config: RelayConfig): Hono {
  const app = new Hono();

  /**
   * GET /v1/relay/tenants/:domain/info - Get signed relay info.
   */
  app.get("/v1/relay/tenants/:domain/info", async (c) => {
    const allowedDomain = c.req.param("domain")?.trim();
    if (!allowedDomain) {
      return c.text("domain is required", 400);
    }

    const tenant = findTenant(config.tenants, allowedDomain);
    if (!tenant) {
      return c.text("tenant not found", 404);
    }

    const jwks = tenant.jwks;
    const activeKeys = tenant.activeKeys;
    if (!jwks || !activeKeys) {
      return c.text("tenant not configured for info endpoint", 500);
    }

    // Compute ETag
    const etag = await computeInfoETag(jwks, activeKeys);

    // Check If-None-Match
    const ifNoneMatch = c.req.header("If-None-Match");
    if (ifNoneMatch === etag) {
      return new Response(null, { status: 304 });
    }

    const reqCtx = extractRequestContext(c);
    const issuedAt = new Date();
    const ttl = tenant.infoTtl || RELAY_INFO_DEFAULT_TTL;
    const expiresAt = new Date(issuedAt.getTime() + ttl * 1000);

    const { space, backlogDomain } = splitDomain(tenant.allowedDomain);
    const payload: RelayInfoPayload = {
      version: RELAY_INFO_VERSION,
      relay_url: buildRelayUrl(config.server.baseUrl, reqCtx.baseUrl),
      allowed_domain: tenant.allowedDomain,
      space,
      domain: backlogDomain,
      issued_at: issuedAt.toISOString(),
      expires_at: expiresAt.toISOString(),
    };

    const payloadJson = JSON.stringify(payload);
    const payloadB64 = base64UrlEncode(payloadJson);

    try {
      const signatures = await buildRelayInfoSignatures(
        payloadB64,
        jwks,
        activeKeys
      );

      const response: RelayInfoResponse = {
        payload: payloadB64,
        signatures,
        payload_decoded: payload,
      };

      return c.json(response, 200, {
        ETag: etag,
        "Cache-Control": `public, max-age=${Math.floor(ttl / 2)}`,
      });
    } catch (err) {
      return c.text(`failed to sign payload: ${(err as Error).message}`, 500);
    }
  });

  return app;
}
