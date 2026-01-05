/**
 * State encoding/decoding utilities for OAuth flow.
 *
 * The encoded state is used to pass information through the OAuth callback
 * without relying on cookies (which may not work in cross-origin scenarios).
 */

/**
 * Claims stored in the encoded state.
 */
export interface EncodedStateClaims {
  /** CLI callback port */
  port: number;
  /** CLI-generated state for CSRF protection */
  cliState: string;
  /** Backlog space name */
  space: string;
  /** Backlog domain (e.g., "backlog.jp") */
  domain: string;
  /** Optional project key */
  project?: string;
}

/**
 * Encode state claims to a URL-safe string.
 *
 * Note: This encoding is NOT signed. It's used for convenience,
 * not for security. The CLI state is what provides CSRF protection.
 */
export function encodeState(claims: EncodedStateClaims): string {
  const json = JSON.stringify(claims);
  // Use base64url encoding (RFC 4648)
  const base64 = btoa(json);
  return base64.replace(/\+/g, "-").replace(/\//g, "_").replace(/=+$/, "");
}

/**
 * Decode state from a URL-safe string back to claims.
 */
export function decodeState(encoded: string): EncodedStateClaims {
  // Restore base64 padding and characters
  let base64 = encoded.replace(/-/g, "+").replace(/_/g, "/");
  while (base64.length % 4) {
    base64 += "=";
  }

  const json = atob(base64);
  const claims = JSON.parse(json) as EncodedStateClaims;

  // Validate required fields
  if (
    typeof claims.port !== "number" ||
    typeof claims.cliState !== "string" ||
    typeof claims.space !== "string" ||
    typeof claims.domain !== "string"
  ) {
    throw new Error("Invalid state claims");
  }

  return claims;
}

/**
 * Extract a shortened session ID from CLI state for logging.
 * Returns the first 16 characters of the state.
 */
export function extractSessionId(cliState: string): string {
  return cliState.slice(0, 16);
}
