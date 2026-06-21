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
  deriveEncKey,
  seal,
  open,
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

interface RefreshTokenPayload {
  rt: string;
  tn: string;
}

export async function encryptRefreshToken(
  refreshToken: string,
  space: string,
  tenant: string,
  jwksJson: string,
): Promise<string> {
  const jwks: JWKS = JSON.parse(jwksJson);
  const jwkByKid = await normalizeJWKS(jwks);
  const { kid, jwk } = getFirstSigningKey(jwks, jwkByKid);
  const key = await deriveEncKey(jwk.d!);
  const payload: RefreshTokenPayload = { rt: refreshToken, tn: tenant };
  return seal(JSON.stringify(payload), key, kid, space, "rt");
}

export async function decryptRefreshToken(
  jwe: string,
  jwksJson: string,
): Promise<{ refreshToken: string; space: string; tenant: string }> {
  const jwks: JWKS = JSON.parse(jwksJson);
  const jwkByKid = await normalizeJWKS(jwks);

  const keyForKid = (kid: string): Uint8Array | undefined => {
    const jwk = jwkByKid.get(kid);
    if (!jwk?.d) return undefined;
    // deriveEncKey is async but jose's keyResolver needs sync return.
    // Pre-derive below and use a Map lookup instead.
    return derivedKeys.get(kid);
  };

  const derivedKeys = new Map<string, Uint8Array>();
  for (const [kid, jwk] of jwkByKid) {
    if (jwk.d) {
      derivedKeys.set(kid, await deriveEncKey(jwk.d));
    }
  }

  const json = await open(jwe, keyForKid, { use: "rt" });
  const { rt, tn } = JSON.parse(json) as RefreshTokenPayload;

  // Extract sp from the JWE protected header
  const headerB64 = jwe.split(".")[0];
  const header = JSON.parse(
    new TextDecoder().decode(base64UrlDecode(headerB64)),
  ) as { sp?: string };

  return { refreshToken: rt, space: header.sp ?? "", tenant: tn };
}

export interface RefreshResult {
  sessionToken: string;
  encryptedRefreshToken: string;
  claims: PortalSessionClaims;
}

/**
 * Refresh a portal session using an encrypted refresh token cookie.
 * Calls Backlog's token endpoint, fetches updated user info, and
 * returns new session + refresh tokens.
 */
export async function refreshPortalSession(
  encryptedRefresh: string,
  jwksJson: string,
  clientId: string,
  clientSecret: string,
): Promise<RefreshResult> {
  const { refreshToken, space, tenant } = await decryptRefreshToken(
    encryptedRefresh,
    jwksJson,
  );

  const params = new URLSearchParams();
  params.set("grant_type", "refresh_token");
  params.set("refresh_token", refreshToken);
  params.set("client_id", clientId);
  params.set("client_secret", clientSecret);

  const tokenResp = await fetch(`https://${space}/api/v2/oauth2/token`, {
    method: "POST",
    headers: { "Content-Type": "application/x-www-form-urlencoded" },
    body: params.toString(),
  });
  if (!tokenResp.ok) {
    throw new Error(`Token refresh failed: ${await tokenResp.text()}`);
  }
  const tokens = (await tokenResp.json()) as {
    access_token: string;
    refresh_token: string;
  };

  const userResp = await fetch(`https://${space}/api/v2/users/myself`, {
    headers: { Authorization: `Bearer ${tokens.access_token}` },
  });
  if (!userResp.ok) {
    throw new Error("Failed to fetch user info after refresh");
  }
  const user = (await userResp.json()) as {
    userId?: string;
    name?: string;
    mailAddress?: string;
  };

  const sessionToken = await createPortalSessionToken(
    {
      userId: user.userId ?? "",
      name: user.name ?? "",
      email: user.mailAddress ?? "",
    },
    tenant,
    space,
    jwksJson,
  );

  const newEncryptedRefresh = await encryptRefreshToken(
    tokens.refresh_token,
    space,
    tenant,
    jwksJson,
  );

  const parts = sessionToken.split(".");
  const claimsB64 = parts[1];
  const claims = JSON.parse(
    new TextDecoder().decode(base64UrlDecode(claimsB64)),
  ) as PortalSessionClaims;

  return {
    sessionToken,
    encryptedRefreshToken: newEncryptedRefresh,
    claims,
  };
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
