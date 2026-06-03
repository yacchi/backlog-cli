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

export function jweAuth(tokenKey: string, tokenKeyPrev?: string) {
    const key = importKey(tokenKey);
    const prevKey = tokenKeyPrev ? importKey(tokenKeyPrev) : undefined;

    return async (c: Context, next: Next) => {
        const auth = c.req.header("authorization");
        if (!auth?.startsWith("Bearer ")) {
            return c.json(
                { error: "unauthorized", error_description: "Missing Bearer token" },
                401,
            );
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
                    return c.json(
                        { error: "invalid_token", error_description: "Token expired or invalid" },
                        401,
                    );
                }
            } else {
                return c.json(
                    { error: "invalid_token", error_description: "Token expired or invalid" },
                    401,
                );
            }
        }

        if (!token.bl_access_token) {
            return c.json(
                { error: "invalid_token", error_description: "Not an access token" },
                401,
            );
        }

        c.set(AUTH_CONTEXT_KEY, { token } satisfies AuthContext);
        await next();
    };
}
