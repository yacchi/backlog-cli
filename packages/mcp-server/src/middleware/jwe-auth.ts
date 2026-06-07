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

        if (!token.bl_access_token) {
            return unauthorized(c, "Not an access token");
        }

        c.set(AUTH_CONTEXT_KEY, { token } satisfies AuthContext);
        await next();
    };
}
