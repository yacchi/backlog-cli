/**
 * Portal session JWT utilities.
 *
 * Creates and verifies HttpOnly session cookies for portal OAuth authentication.
 * Sessions are Ed25519-signed JWTs using the server JWKS.
 */

import {
  type JWKS,
  base64UrlEncode,
  base64UrlDecode,
  randomBytes,
  signEd25519,
  verifyEd25519,
  normalizeJWKS,
  getFirstSigningKey,
} from "./crypto.js";

const SESSION_EXPIRY_SECONDS = 3600; // 1 hour

export interface PortalSessionClaims {
  sub: string;
  name: string;
  email: string;
  tenant: string;
  space: string;
  purpose: "portal_session";
  iat: number;
  exp: number;
  jti: string;
}

export async function createPortalSessionToken(
  user: { userId: string; name: string; email: string },
  tenant: string,
  space: string,
  jwksJson: string,
): Promise<string> {
  const jwks: JWKS = JSON.parse(jwksJson);
  const jwkByKid = await normalizeJWKS(jwks);
  const { kid, jwk } = getFirstSigningKey(jwks, jwkByKid);

  const now = Math.floor(Date.now() / 1000);
  const jti = base64UrlEncode(randomBytes(16));

  const header = { alg: "EdDSA", typ: "JWT", kid };
  const claims: PortalSessionClaims = {
    sub: user.userId,
    name: user.name,
    email: user.email,
    tenant,
    space,
    purpose: "portal_session",
    iat: now,
    exp: now + SESSION_EXPIRY_SECONDS,
    jti,
  };

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

export async function verifyPortalSessionToken(
  token: string,
  jwksJson: string,
): Promise<PortalSessionClaims> {
  const parts = token.split(".");
  if (parts.length !== 3) {
    throw new Error("Invalid JWT format");
  }

  const [headerB64, claimsB64, signatureB64] = parts;

  const header = JSON.parse(
    new TextDecoder().decode(base64UrlDecode(headerB64)),
  ) as { alg: string; kid?: string };

  if (header.alg !== "EdDSA") {
    throw new Error(`Unsupported algorithm: ${header.alg}`);
  }

  const jwks: JWKS = JSON.parse(jwksJson);
  const jwkByKid = await normalizeJWKS(jwks);

  const kid = header.kid;
  if (!kid) {
    throw new Error("Missing kid in JWT header");
  }

  const jwk = jwkByKid.get(kid);
  if (!jwk || !jwk.x) {
    throw new Error(`Unknown key: ${kid}`);
  }

  const publicKeyBytes = base64UrlDecode(jwk.x);
  const signatureBytes = base64UrlDecode(signatureB64);
  const signingInput = new TextEncoder().encode(`${headerB64}.${claimsB64}`);

  const valid = await verifyEd25519(publicKeyBytes, signatureBytes, signingInput);
  if (!valid) {
    throw new Error("Invalid signature");
  }

  const claims = JSON.parse(
    new TextDecoder().decode(base64UrlDecode(claimsB64)),
  ) as PortalSessionClaims;

  if (claims.purpose !== "portal_session") {
    throw new Error("Invalid token purpose");
  }

  const now = Math.floor(Date.now() / 1000);
  if (claims.exp && claims.exp < now) {
    throw new Error("Token expired");
  }

  return claims;
}
