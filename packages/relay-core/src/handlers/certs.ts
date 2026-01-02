/**
 * Certificate handlers for JWKS distribution.
 */

import { Hono } from "hono";
import type { RelayConfig, TenantConfig } from "../config/types.js";

/**
 * JWK structure for certificates.
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
 * Derive Ed25519 public key from seed.
 * Note: This requires Web Crypto API with Ed25519 support or a polyfill.
 */
async function deriveEd25519PublicKey(_seed: Uint8Array): Promise<Uint8Array> {
  // Web Crypto API doesn't support Ed25519 key derivation in all environments
  // For now, we throw an error and expect the JWKS to already have public keys
  throw new Error(
    "Ed25519 public key derivation not supported. Ensure JWKS contains public keys."
  );
}

/**
 * Base64URL decode.
 */
function base64UrlDecode(str: string): Uint8Array {
  // Add padding if needed
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
 * Base64URL encode.
 */
function base64UrlEncode(bytes: Uint8Array): string {
  let binary = "";
  for (let i = 0; i < bytes.length; i++) {
    binary += String.fromCharCode(bytes[i]);
  }
  return btoa(binary).replace(/\+/g, "-").replace(/\//g, "_").replace(/=+$/, "");
}

/**
 * Compute SHA256 hash and return first 16 bytes as hex.
 */
async function computeETag(data: Uint8Array): Promise<string> {
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
 * Redact private keys from JWKS.
 */
async function redactJWKSPrivateKeys(jwksJson: string): Promise<string> {
  const jwks: JWKS = JSON.parse(jwksJson);

  if (!jwks.keys || jwks.keys.length === 0) {
    throw new Error("invalid jwks: keys is missing or empty");
  }

  for (const key of jwks.keys) {
    if (key.d) {
      // If we have a private key but no public key, we need to derive it
      // For now, just remove the private key and hope X is already present
      if (!key.x) {
        // Try to derive public key from seed (not supported in all environments)
        try {
          const seed = base64UrlDecode(key.d);
          const pubKey = await deriveEd25519PublicKey(seed);
          key.x = base64UrlEncode(pubKey);
        } catch {
          throw new Error(
            `Cannot derive public key for ${key.kid}. Ensure JWKS contains public keys.`
          );
        }
      }
      delete key.d;
    }
  }

  return JSON.stringify(jwks);
}

/**
 * Find tenant by allowed domain.
 */
function findTenant(
  tenants: TenantConfig[] | undefined,
  domain: string
): TenantConfig | undefined {
  return tenants?.find(
    (t) => t.allowedDomain.toLowerCase() === domain.toLowerCase()
  );
}

/**
 * Create certificate handlers.
 */
export function createCertsHandlers(config: RelayConfig): Hono {
  const app = new Hono();

  /**
   * GET /relay/certs/:domain - Get public JWKS for a tenant.
   */
  app.get("/relay/certs/:domain", async (c) => {
    const domain = c.req.param("domain")?.trim();
    if (!domain) {
      return c.text("domain is required", 400);
    }

    const tenant = findTenant(config.tenants, domain);
    if (!tenant) {
      return c.text("tenant not found", 404);
    }

    // Get JWKS from tenant config (extended tenant config would have jwks field)
    const jwks = (tenant as TenantConfig & { jwks?: string }).jwks;
    if (!jwks) {
      return c.text("tenant jwks is empty", 500);
    }

    try {
      const redacted = await redactJWKSPrivateKeys(jwks);
      const encoder = new TextEncoder();
      const data = encoder.encode(redacted);
      const etag = await computeETag(data);

      // Check If-None-Match
      const ifNoneMatch = c.req.header("If-None-Match");
      if (ifNoneMatch === etag) {
        return new Response(null, { status: 304 });
      }

      return c.json(JSON.parse(redacted), 200, {
        ETag: etag,
        "Cache-Control": "public, max-age=3600",
      });
    } catch (err) {
      return c.text(`failed to parse jwks: ${(err as Error).message}`, 500);
    }
  });

  return app;
}
