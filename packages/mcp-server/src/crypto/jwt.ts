import { SignJWT, jwtVerify, importJWK, type JWK, type CryptoKey } from "jose";

export interface SpaceToken {
    space: string;
    bl_access_token: string;
    bl_refresh_token: string;
    bl_expires_at: number;
}

export interface TokenPayload {
    spaces?: SpaceToken[];
    bl_access_token?: string;
    bl_refresh_token?: string;
    bl_expires_at?: number;
    space: string;
    iat: number;
    exp?: number;
}

const ALG = "EdDSA";

interface JWKWithKid extends JWK {
    kid: string;
    kty: string;
    crv: string;
    x: string;
    d?: string;
}

interface JWKS {
    keys: JWKWithKid[];
}

export interface SigningKeys {
    signingKey: CryptoKey;
    signingKid: string;
    verifyKeys: Map<string, CryptoKey>;
}

export async function loadSigningKeys(jwksJson: string): Promise<SigningKeys> {
    const jwks: JWKS = JSON.parse(jwksJson);
    if (!jwks.keys || jwks.keys.length === 0) {
        throw new Error("JWKS has no keys");
    }

    const first = jwks.keys[0];
    if (!first.d) {
        throw new Error("Signing key (keys[0]) must include private key (d)");
    }

    const signingKey = await importJWK(first, ALG) as CryptoKey;
    const signingKid = first.kid;

    const verifyKeys = new Map<string, CryptoKey>();
    for (const jwk of jwks.keys) {
        const publicJwk: JWK = { kty: jwk.kty, crv: jwk.crv, x: jwk.x };
        const key = await importJWK(publicJwk, ALG) as CryptoKey;
        verifyKeys.set(jwk.kid, key);
    }

    return { signingKey, signingKid, verifyKeys };
}

export async function sign(
    payload: Record<string, unknown>,
    signingKey: CryptoKey,
    kid: string,
): Promise<string> {
    const { iat, exp, ...rest } = payload;
    const builder = new SignJWT(rest)
        .setProtectedHeader({ alg: ALG, kid })
        .setIssuedAt(iat as number);
    if (typeof exp === "number") {
        builder.setExpirationTime(exp);
    }
    return builder.sign(signingKey);
}

export async function verify(
    jwt: string,
    verifyKeys: Map<string, CryptoKey>,
): Promise<Record<string, unknown>> {
    const header = decodeProtectedHeaderLazy(jwt);
    const kid = header.kid;
    if (!kid) {
        throw new Error("JWT missing kid header");
    }

    const key = verifyKeys.get(kid);
    if (!key) {
        throw new Error(`Unknown key: ${kid}`);
    }

    const { payload } = await jwtVerify(jwt, key, { algorithms: [ALG] });
    return payload as Record<string, unknown>;
}

export async function signToken(
    payload: TokenPayload,
    signingKey: CryptoKey,
    kid: string,
): Promise<string> {
    return sign(payload as unknown as Record<string, unknown>, signingKey, kid);
}

export async function verifyToken(
    jwt: string,
    verifyKeys: Map<string, CryptoKey>,
): Promise<TokenPayload> {
    const payload = await verify(jwt, verifyKeys);
    const token = payload as unknown as TokenPayload;

    // Backward compatibility: normalize old tokens with separate space/domain into single space
    if (!token.space.includes(".") && (payload as any).domain) {
        token.space = `${token.space}.${(payload as any).domain}`;
    }

    // Normalize spaces array if present
    if (token.spaces) {
        token.spaces = token.spaces.map((s) => ({
            space: s.space.includes(".") ? s.space : `${s.space}.${(s as any).domain || ''}`,
            bl_access_token: s.bl_access_token,
            bl_refresh_token: s.bl_refresh_token,
            bl_expires_at: s.bl_expires_at,
        }));
    }

    return token;
}

export function normalizeSpace(space: string, domain?: string): string {
    if (space.includes(".")) return space;
    if (domain) return `${space}.${domain}`;
    return space;
}

function decodeProtectedHeaderLazy(jwt: string): { alg: string; kid?: string } {
    const [headerB64] = jwt.split(".");
    if (!headerB64) throw new Error("Invalid JWT");
    const json = atob(headerB64.replace(/-/g, "+").replace(/_/g, "/"));
    return JSON.parse(json);
}
