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
 * Derive Ed25519 public key from private key seed using Web Crypto API.
 * Uses PKCS8 format to import the private key, then exports to get the public key.
 */
async function deriveEd25519PublicKey(seed: Uint8Array): Promise<Uint8Array> {
  // Ed25519 PKCS8 format:
  // SEQUENCE {
  //   INTEGER 0 (version)
  //   SEQUENCE { OID 1.3.101.112 (Ed25519) }
  //   OCTET STRING { OCTET STRING { 32-byte seed } }
  // }
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

  try {
    // Import as PKCS8 private key
    const privateKey = await crypto.subtle.importKey(
      "pkcs8",
      pkcs8Key,
      { name: "Ed25519" },
      true,
      ["sign"]
    );

    // Export as JWK to get the public key (x)
    const exportedJwk = await crypto.subtle.exportKey("jwk", privateKey);
    if (!exportedJwk.x) {
      throw new Error("Failed to derive public key from exported JWK");
    }

    return base64UrlDecode(exportedJwk.x);
  } catch (e) {
    throw new Error(
      `Ed25519 public key derivation failed: ${(e as Error).message}. Ensure JWKS contains public keys (x field).`
    );
  }
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
   * GET /v1/relay/tenants/:domain/certs - Get public JWKS for a tenant.
   */
  app.get("/v1/relay/tenants/:domain/certs", async (c) => {
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
