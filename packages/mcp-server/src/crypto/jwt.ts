import { SignJWT, jwtVerify, importJWK, type JWK, type CryptoKey } from "jose";
import { deriveEncKey } from "./secret.js";

/** Per-space entry in the authorization code (has both access + refresh). */
export interface SpaceToken {
    space: string;
    bl_access_token: string;
    bl_refresh_token: string;
    bl_expires_at: number;
}

export interface SpaceAccessEntry {
    at: string;
    exp: number;
}

export interface SpaceRefreshEntry {
    rt: string;
}

export type SpaceEntry = SpaceAccessEntry | SpaceRefreshEntry;

export const SPACE_KEY_PREFIX = "space:";

export interface TokenPayload {
    /** @deprecated Legacy array format — read-only for backward compat. */
    spaces?: SpaceToken[];
    bl_access_token?: string;
    bl_refresh_token?: string;
    bl_expires_at?: number;
    space: string;
    iat: number;
    exp?: number;
    userEmail?: string;
    clientName?: string;
    [key: string]: unknown;
}

export function spaceKey(spaceDomain: string): string {
    return SPACE_KEY_PREFIX + spaceDomain;
}

export function getSpaceEntry(token: TokenPayload, spaceDomain: string): SpaceEntry | undefined {
    return token[spaceKey(spaceDomain)] as SpaceEntry | undefined;
}

export function setSpaceAccess(payload: Record<string, unknown>, spaceDomain: string, at: string, exp: number): void {
    payload[spaceKey(spaceDomain)] = { at, exp } as SpaceAccessEntry;
}

export function setSpaceRefresh(payload: Record<string, unknown>, spaceDomain: string, rt: string): void {
    payload[spaceKey(spaceDomain)] = { rt } as SpaceRefreshEntry;
}

export function listSpaceEntries(token: TokenPayload): Array<[string, SpaceEntry]> {
    return Object.entries(token)
        .filter(([key]) => key.startsWith(SPACE_KEY_PREFIX))
        .map(([key, value]) => [key.slice(SPACE_KEY_PREFIX.length), value as SpaceEntry]);
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
    /**
     * Per-`kid` AES-256-GCM keys derived from each signing key's private
     * scalar (`d`), used to seal/open the raw Backlog tokens carried inside the
     * JWT. A key is present only when its JWK includes `d`; retired keys must
     * keep `d` so previously-issued (encrypted) tokens can still be opened
     * after a signing-key rotation.
     */
    encKeys: Map<string, Uint8Array>;
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
    const encKeys = new Map<string, Uint8Array>();
    for (const jwk of jwks.keys) {
        const publicJwk: JWK = { kty: jwk.kty, crv: jwk.crv, x: jwk.x };
        const key = await importJWK(publicJwk, ALG) as CryptoKey;
        verifyKeys.set(jwk.kid, key);
        if (jwk.d) {
            encKeys.set(jwk.kid, deriveEncKey(jwk.d));
        }
    }

    return { signingKey, signingKid, verifyKeys, encKeys };
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

    // Normalize "space:*" keys with missing domain
    for (const key of Object.keys(token)) {
        if (!key.startsWith(SPACE_KEY_PREFIX)) continue;
        const domain = key.slice(SPACE_KEY_PREFIX.length);
        if (!domain.includes(".") && (payload as any).domain) {
            const normKey = spaceKey(`${domain}.${(payload as any).domain}`);
            token[normKey] = token[key];
            delete token[key];
        }
    }

    // Migrate legacy spaces array → flat "space:*" entries
    if (token.spaces && !listSpaceEntries(token).length) {
        for (const s of token.spaces) {
            const normSpace = s.space.includes(".") ? s.space : `${s.space}.${(s as any).domain || ""}`;
            if (s.bl_access_token) {
                token[spaceKey(normSpace)] = { at: s.bl_access_token, exp: s.bl_expires_at } satisfies SpaceAccessEntry;
            } else if (s.bl_refresh_token) {
                token[spaceKey(normSpace)] = { rt: s.bl_refresh_token } satisfies SpaceRefreshEntry;
            }
        }
        delete token.spaces;
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
