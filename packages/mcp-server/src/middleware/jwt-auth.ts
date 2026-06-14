import type { Context, Next } from "hono";
import type { CryptoKey } from "jose";
import { verifyToken } from "../crypto/jwt.js";
import type { TokenPayload, SpaceAccessEntry } from "../crypto/jwt.js";
import { getSpaceEntry, listSpaceEntries } from "../crypto/jwt.js";
import { open } from "../crypto/secret.js";
import { Logger, LOGGER_CONTEXT_KEY } from "../logging/logger.js";

export interface AuthContext {
    token: TokenPayload;
}

const AUTH_CONTEXT_KEY = "mcp-auth";

export function getAuthContext(c: Context): AuthContext {
    return c.get(AUTH_CONTEXT_KEY) as AuthContext;
}

/**
 * Resolve the (decrypted) Backlog access token for the requested space.
 *
 * The `at` carried in the token is a sealed JWE; it is opened here with the
 * server-held encryption key. A token whose `at` cannot be opened (legacy
 * plaintext, wrong/unknown key, tampering) resolves to `null`, which callers
 * surface as "space not authenticated" → re-authentication.
 */
export async function resolveSpaceToken(
    token: TokenPayload,
    encKeys: Map<string, Uint8Array>,
    requestedSpace?: string,
): Promise<{ space: string; bl_access_token: string } | null> {
    const domain = requestedSpace ?? token.space;

    let sealed: string | undefined;
    const entry = getSpaceEntry(token, domain);
    if (entry && "at" in entry) {
        sealed = (entry as SpaceAccessEntry).at;
    } else if (token.space === domain && token.bl_access_token) {
        // Legacy single-space fallback (top-level bl_access_token).
        sealed = token.bl_access_token;
    }
    if (!sealed) return null;

    try {
        const at = await open(sealed, (kid) => encKeys.get(kid), { sp: domain, use: "at" });
        return { space: domain, bl_access_token: at };
    } catch {
        return null;
    }
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

export function jwtAuth(
    verifyKeys: Map<string, CryptoKey>,
    resourceMetadataUrl?: (c: Context) => string,
) {
    const bypassToken = loadBypassToken();
    if (bypassToken) {
        // eslint-disable-next-line no-console
        console.warn(
            `⚠️  MCP_AUTH_BYPASS_TOKEN is set — JWT verification is DISABLED. ` +
            `space=${bypassToken.space}. This must only be used for local testing.`,
        );
    }

    function unauthorized(c: Context, description: string) {
        const url = resourceMetadataUrl?.(c);
        const wwwAuth = url
            ? `Bearer resource_metadata="${url}"`
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
            const parentLogger = (c.get(LOGGER_CONTEXT_KEY) as Logger | undefined) ?? new Logger();
            c.set(LOGGER_CONTEXT_KEY, parentLogger.child({ tenant: bypassToken.space }));
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

        // Enrich logger with JWT-derived context
        const parentLogger = (c.get(LOGGER_CONTEXT_KEY) as Logger | undefined) ?? new Logger();
        const bindings: Record<string, unknown> = { tenant: token.space };
        if (token.userEmail) bindings.userEmail = token.userEmail;
        if (token.clientName) bindings.clientName = token.clientName;
        c.set(LOGGER_CONTEXT_KEY, parentLogger.child(bindings));

        return next();
    };
}
