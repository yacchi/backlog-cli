import type { Context, Next } from "hono";
import { decryptToken, importKey } from "../crypto/jwe.js";
import type { TokenPayload } from "../crypto/jwe.js";

export interface AuthContext {
    token: TokenPayload;
}

const AUTH_CONTEXT_KEY = "mcp-auth";

export function getAuthContext(c: Context): AuthContext {
    return c.get(AUTH_CONTEXT_KEY) as AuthContext;
}

export function resolveSpaceToken(
    token: TokenPayload,
    spaceKey?: string,
): { space: string; domain: string; bl_access_token: string } | null {
    if (!spaceKey) {
        return {
            space: token.space,
            domain: token.domain,
            bl_access_token: token.bl_access_token ?? token.spaces?.[0]?.bl_access_token ?? "",
        };
    }

    if (token.spaces) {
        const found = token.spaces.find(
            (s) => `${s.space}.${s.domain}` === spaceKey,
        );
        if (found) {
            return {
                space: found.space,
                domain: found.domain,
                bl_access_token: found.bl_access_token,
            };
        }
    }

    if (`${token.space}.${token.domain}` === spaceKey && token.bl_access_token) {
        return {
            space: token.space,
            domain: token.domain,
            bl_access_token: token.bl_access_token,
        };
    }

    return null;
}

export function jweAuth(tokenKey: string, tokenKeyPrev?: string, resourceMetadataUrl?: string) {
    const key = importKey(tokenKey);
    const prevKey = tokenKeyPrev ? importKey(tokenKeyPrev) : undefined;

    function unauthorized(c: Context, description: string) {
        const wwwAuth = resourceMetadataUrl
            ? `Bearer resource_metadata="${resourceMetadataUrl}"`
            : "Bearer";
        c.header("WWW-Authenticate", wwwAuth);
        return c.json(
            { error: "unauthorized", error_description: description },
            401,
        );
    }

    return async (c: Context, next: Next) => {
        const auth = c.req.header("authorization");
        if (!auth?.startsWith("Bearer ")) {
            return unauthorized(c, "Missing Bearer token");
        }

        const jwe = auth.slice(7);
        let token: TokenPayload;
        try {
            token = await decryptToken(jwe, key);
        } catch {
            if (prevKey) {
                try {
                    token = await decryptToken(jwe, prevKey);
                } catch {
                    return unauthorized(c, "Token expired or invalid");
                }
            } else {
                return unauthorized(c, "Token expired or invalid");
            }
        }

        const hasAccess = token.bl_access_token || (token.spaces && token.spaces.length > 0);
        if (!hasAccess) {
            return unauthorized(c, "Not an access token");
        }

        c.set(AUTH_CONTEXT_KEY, { token } satisfies AuthContext);
        await next();
    };
}
