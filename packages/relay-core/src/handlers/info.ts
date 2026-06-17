/**
 * Relay info handlers for signed relay information.
 */

import { Hono } from "hono";
import type { RelayConfig, TenantConfig } from "../config/types.js";
import { extractRequestContext } from "../utils/request.js";
import {
  type JWKS,
  base64UrlEncode,
  base64UrlDecode,
  signEd25519,
} from "../utils/crypto.js";

const RELAY_INFO_VERSION = 1;
const RELAY_INFO_DEFAULT_TTL = 600;

/**
 * Relay info payload.
 */
interface RelayInfoPayload {
  version: number;
  name: string;
  relay_url: string;
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
 * Note: TenantConfig already includes these fields, but we keep this for clarity.
 */
type ExtendedTenantConfig = TenantConfig;


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
 * Find tenant by name.
 */
function findTenant(
  tenants: TenantConfig[] | undefined,
  name: string
): ExtendedTenantConfig | undefined {
  return tenants?.find(
    (t) => t.name.toLowerCase() === name.toLowerCase()
  );
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
 * Build signatures for relay info.
 */
async function buildRelayInfoSignatures(
  payloadB64: string,
  jwksJson: string,
): Promise<RelayInfoSignature[]> {
  const jwks: JWKS = JSON.parse(jwksJson);
  if (!jwks.keys || jwks.keys.length === 0) {
    throw new Error("jwks has no keys");
  }

  const signatures: RelayInfoSignature[] = [];

  // Sign with keys[0] (signing key)
  for (const jwk of jwks.keys.slice(0, 1)) {
    const kid = jwk.kid;
    if (!kid) {
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
  app.get("/v1/relay/tenants/:name/info", async (c) => {
    const name = c.req.param("name")?.trim();
    if (!name) {
      return c.text("name is required", 400);
    }

    const tenant = findTenant(config.tenants, name);
    if (!tenant) {
      return c.text("tenant not found", 404);
    }

    const jwks = config.jwks;
    if (!jwks) {
      return c.text("server jwks not configured", 500);
    }

    // Compute ETag
    const etag = await computeInfoETag(jwks, "");

    // Check If-None-Match
    const ifNoneMatch = c.req.header("If-None-Match");
    if (ifNoneMatch === etag) {
      return new Response(null, { status: 304 });
    }

    const reqCtx = extractRequestContext(c);
    const issuedAt = new Date();
    const ttl = tenant.info_ttl || RELAY_INFO_DEFAULT_TTL;
    const expiresAt = new Date(issuedAt.getTime() + ttl * 1000);

    const payload: RelayInfoPayload = {
      version: RELAY_INFO_VERSION,
      name: tenant.name,
      relay_url: buildRelayUrl(config.server.base_url, reqCtx.baseUrl),
      issued_at: issuedAt.toISOString(),
      expires_at: expiresAt.toISOString(),
    };

    const payloadJson = JSON.stringify(payload);
    const payloadB64 = base64UrlEncode(payloadJson);

    try {
      const signatures = await buildRelayInfoSignatures(
        payloadB64,
        jwks,
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
