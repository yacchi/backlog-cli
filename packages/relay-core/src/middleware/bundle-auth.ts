/**
 * Bundle token authentication middleware.
 *
 * This middleware verifies JWT tokens that are embedded in config bundles,
 * allowing CLIs to authenticate without user interaction.
 */

import type { Context, MiddlewareHandler } from "hono";
import type { TenantConfig, AuditLogger } from "../config/types.js";
import { AuditActions, createAuditEvent } from "./audit.js";
import { extractRequestContext } from "../utils/request.js";

/**
 * Extended tenant config with JWKS for bundle auth.
 */
export interface BundleAuthTenantConfig extends TenantConfig {
  /** JSON Web Key Set for token verification */
  jwks?: string;
}

/**
 * JWT header structure.
 */
interface JWTHeader {
  alg: string;
  kid: string;
  typ?: string;
}

/**
 * Bundle token claims.
 */
interface BundleTokenClaims {
  /** Subject (allowed_domain) */
  sub: string;
  /** Issued at (Unix timestamp) */
  iat: number;
  /** Not before (Unix timestamp) */
  nbf?: number;
  /** JWT ID */
  jti: string;
}

/**
 * JWK structure for Ed25519 keys.
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
 * Options for creating bundle auth middleware.
 */
export interface BundleAuthOptions {
  /** Function to find tenant by domain */
  findTenant: (domain: string) => BundleAuthTenantConfig | undefined;
  /** Audit logger */
  auditLogger: AuditLogger;
  /** Paths to skip authentication (e.g., ["/certs", "/info"]) */
  skipPaths?: string[];
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
 * Verify Ed25519 signature using Web Crypto API.
 * The key is imported and verified in a single operation to avoid type leakage.
 */
async function verifyEd25519SignatureWithJWK(
  jwk: JWK,
  data: Uint8Array,
  signature: Uint8Array
): Promise<boolean> {
  if (jwk.kty !== "OKP" || jwk.crv !== "Ed25519") {
    throw new Error(`Unsupported key type: ${jwk.kty}/${jwk.crv}`);
  }
  if (!jwk.x) {
    throw new Error("JWK missing public key (x)");
  }

  const publicKeyBytes = base64UrlDecode(jwk.x);

  // Import the public key
  const publicKey = await crypto.subtle.importKey(
    "raw",
    publicKeyBytes,
    { name: "Ed25519" },
    false,
    ["verify"]
  );

  // Verify the signature
  return await crypto.subtle.verify(
    "Ed25519",
    publicKey,
    signature,
    data
  );
}

/**
 * Verify bundle token JWT.
 */
async function verifyBundleToken(
  token: string,
  allowedDomain: string,
  jwksJson: string
): Promise<void> {
  const parts = token.split(".");
  if (parts.length !== 3) {
    throw new Error("Invalid JWT format");
  }

  const [headerB64, claimsB64, signatureB64] = parts;

  // Parse header
  const headerJson = new TextDecoder().decode(base64UrlDecode(headerB64));
  const header: JWTHeader = JSON.parse(headerJson);

  if (header.alg !== "EdDSA") {
    throw new Error(`Unsupported JWT algorithm: ${header.alg}`);
  }

  // Parse JWKS and find key
  const jwks: JWKS = JSON.parse(jwksJson);
  const jwk = jwks.keys.find((k) => k.kid === header.kid);
  if (!jwk) {
    throw new Error(`Unknown key ID: ${header.kid}`);
  }

  // Verify signature
  const signingInput = new TextEncoder().encode(`${headerB64}.${claimsB64}`);
  const signature = base64UrlDecode(signatureB64);

  const valid = await verifyEd25519SignatureWithJWK(jwk, signingInput, signature);
  if (!valid) {
    throw new Error("JWT signature verification failed");
  }

  // Parse and verify claims
  const claimsJson = new TextDecoder().decode(base64UrlDecode(claimsB64));
  const claims: BundleTokenClaims = JSON.parse(claimsJson);

  if (claims.sub !== allowedDomain) {
    throw new Error(
      `JWT subject mismatch: expected ${allowedDomain}, got ${claims.sub}`
    );
  }
}

/**
 * Extract domain from path.
 * Supports patterns like:
 * - /relay/bundle/:domain
 * - /v1/relay/tenants/:domain/bundle
 */
function extractDomainFromPath(path: string): string | undefined {
  // Pattern: /relay/bundle/:domain
  const relayMatch = path.match(/^\/relay\/bundle\/([^/]+)/);
  if (relayMatch) {
    return relayMatch[1];
  }

  // Pattern: /v1/relay/tenants/:domain/...
  const v1Match = path.match(/^\/v1\/relay\/tenants\/([^/]+)/);
  if (v1Match) {
    return v1Match[1];
  }

  return undefined;
}

/**
 * Check if path should skip authentication.
 */
function shouldSkipAuth(path: string, skipPaths: string[]): boolean {
  for (const skipPath of skipPaths) {
    // Check if path ends with skip path or contains it as a segment
    if (path.includes(skipPath)) {
      return true;
    }
  }
  return false;
}

/**
 * Create bundle authentication middleware.
 *
 * This middleware protects endpoints that require bundle token authentication.
 * It verifies JWT tokens embedded in config bundles.
 */
export function createBundleAuthMiddleware(
  options: BundleAuthOptions
): MiddlewareHandler {
  const { findTenant, auditLogger, skipPaths = ["/certs", "/info"] } = options;

  return async (c: Context, next: () => Promise<void>) => {
    const path = c.req.path;

    // Skip non-relay paths
    if (!path.includes("/relay/") && !path.includes("/v1/relay/tenants/")) {
      await next();
      return;
    }

    // Extract domain from path
    const domain = extractDomainFromPath(path);
    if (!domain) {
      await next();
      return;
    }

    // Skip auth for certain paths (certs, info are public)
    if (shouldSkipAuth(path, skipPaths)) {
      await next();
      return;
    }

    const reqCtx = extractRequestContext(c);

    // Find tenant
    const tenant = findTenant(domain);
    if (!tenant) {
      auditLogger.log(
        createAuditEvent({
          action: AuditActions.BUNDLE_AUTH,
          domain,
          clientIp: reqCtx.clientIp,
          userAgent: reqCtx.userAgent,
          result: "error",
          error: "tenant not found",
        })
      );
      return c.text("tenant not found", 404);
    }

    // Get Authorization header
    const authHeader = c.req.header("Authorization");
    if (!authHeader) {
      auditLogger.log(
        createAuditEvent({
          action: AuditActions.BUNDLE_AUTH,
          domain,
          clientIp: reqCtx.clientIp,
          userAgent: reqCtx.userAgent,
          result: "error",
          error: "missing authorization header",
        })
      );
      return c.text("authorization required", 401);
    }

    // Extract Bearer token
    if (!authHeader.startsWith("Bearer ")) {
      auditLogger.log(
        createAuditEvent({
          action: AuditActions.BUNDLE_AUTH,
          domain,
          clientIp: reqCtx.clientIp,
          userAgent: reqCtx.userAgent,
          result: "error",
          error: "invalid authorization format",
        })
      );
      return c.text("invalid authorization format", 401);
    }
    const token = authHeader.slice(7);

    // Get JWKS from tenant
    const jwks = tenant.jwks;
    if (!jwks) {
      auditLogger.log(
        createAuditEvent({
          action: AuditActions.BUNDLE_AUTH,
          domain,
          clientIp: reqCtx.clientIp,
          userAgent: reqCtx.userAgent,
          result: "error",
          error: "tenant jwks not configured",
        })
      );
      return c.text("tenant not configured for bundle auth", 500);
    }

    // Verify token
    try {
      await verifyBundleToken(token, domain, jwks);
    } catch (err) {
      auditLogger.log(
        createAuditEvent({
          action: AuditActions.BUNDLE_AUTH,
          domain,
          clientIp: reqCtx.clientIp,
          userAgent: reqCtx.userAgent,
          result: "error",
          error: (err as Error).message,
        })
      );
      return c.text("invalid token", 401);
    }

    // Authentication successful
    await next();
    return;
  };
}
