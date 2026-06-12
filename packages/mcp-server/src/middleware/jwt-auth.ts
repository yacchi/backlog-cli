import type { Context, Next } from "hono";
import type { CryptoKey } from "jose";
import { verifyToken } from "../crypto/jwt.js";
import type { TokenPayload, SpaceAccessEntry } from "../crypto/jwt.js";
import { getSpaceEntry, listSpaceEntries } from "../crypto/jwt.js";

export interface AuthContext {
    token: TokenPayload;
}

const AUTH_CONTEXT_KEY = "mcp-auth";

export function getAuthContext(c: Context): AuthContext {
    return c.get(AUTH_CONTEXT_KEY) as AuthContext;
}

export function resolveSpaceToken(
    token: TokenPayload,
    requestedSpace?: string,
): { space: string; bl_access_token: string } | null {
    if (!requestedSpace) {
        const entry = getSpaceEntry(token, token.space);
        if (entry && "at" in entry) {
            return { space: token.space, bl_access_token: entry.at };
        }
        if (token.bl_access_token) {
            return { space: token.space, bl_access_token: token.bl_access_token };
        }
        return null;
    }

    const entry = getSpaceEntry(token, requestedSpace);
    if (entry && "at" in entry) {
        return { space: requestedSpace, bl_access_token: (entry as SpaceAccessEntry).at };
    }

    // Legacy single-space fallback
    if (token.space === requestedSpace && token.bl_access_token) {
        return { space: token.space, bl_access_token: token.bl_access_token };
    }

    return null;
}

export function jwtAuth(verifyKeys: Map<string, CryptoKey>, resourceMetadataUrl?: string) {
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

        const jwt = auth.slice(7);
        let token: TokenPayload;
        try {
            token = await verifyToken(jwt, verifyKeys);
        } catch {
            return unauthorized(c, "Token expired or invalid");
        }

        const hasSpaceAccess = listSpaceEntries(token).some(([, e]) => "at" in e);
        const hasAccess = token.bl_access_token || hasSpaceAccess;
        if (!hasAccess) {
            return unauthorized(c, "Not an access token");
        }

        c.set(AUTH_CONTEXT_KEY, { token } satisfies AuthContext);
        return next();
    };
}
