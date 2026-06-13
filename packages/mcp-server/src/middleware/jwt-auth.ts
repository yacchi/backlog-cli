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

/**
 * Test/dev-only auth bypass. When the `MCP_AUTH_BYPASS_TOKEN` env var is set to a
 * JSON-encoded TokenPayload, JWT verification is skipped entirely and that payload
 * is injected as the authenticated token.
 *
 * SECURITY: This MUST never be set in production. It exists solely so the sandbox /
 * transport layers can be exercised end-to-end without minting signed JWTs. The
 * server logs a loud warning at startup when it is active.
 */
export function loadBypassToken(): TokenPayload | null {
    const raw = process.env.MCP_AUTH_BYPASS_TOKEN;
    if (!raw) return null;
    let parsed: TokenPayload;
    try {
        parsed = JSON.parse(raw) as TokenPayload;
    } catch (err) {
        throw new Error(`MCP_AUTH_BYPASS_TOKEN is not valid JSON: ${(err as Error).message}`);
    }
    if (!parsed.space) {
        throw new Error("MCP_AUTH_BYPASS_TOKEN must include a 'space' field");
    }
    if (typeof parsed.iat !== "number") {
        parsed.iat = Math.floor(Date.now() / 1000);
    }
    return parsed;
}

export function jwtAuth(verifyKeys: Map<string, CryptoKey>, resourceMetadataUrl?: string) {
    const bypassToken = loadBypassToken();
    if (bypassToken) {
        // eslint-disable-next-line no-console
        console.warn(
            `⚠️  MCP_AUTH_BYPASS_TOKEN is set — JWT verification is DISABLED. ` +
            `space=${bypassToken.space}. This must only be used for local testing.`,
        );
    }

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
        if (bypassToken) {
            c.set(AUTH_CONTEXT_KEY, { token: bypassToken } satisfies AuthContext);
            return next();
        }

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
