import { Hono, type Context } from "hono";
import { getCookie, setCookie } from "hono/cookie";
import type { McpServerConfig } from "../config/schema.js";
import { matchSpacePattern } from "../config/schema.js";
import { resolveBaseUrl } from "../base-url.js";
import { sign, verify, signToken, spaceKey, setSpaceAccess, setSpaceRefresh } from "../crypto/jwt.js";
import type { SpaceToken, SigningKeys } from "../crypto/jwt.js";
import { seal, open } from "../crypto/secret.js";
import { Logger, LOGGER_CONTEXT_KEY } from "../logging/logger.js";
import type {
    DcrRequest,
    DcrResponse,
    TokenRequest,
    TokenResponse,
    TokenErrorResponse,
    ClientIdPayload,
    AuthorizeState,
    SpaceRef,
} from "./types.js";

export interface TokenExchange {
    exchangeCode(space: string, code: string, redirectUri?: string): Promise<TokenResponse>;
    refreshToken(space: string, refreshToken: string): Promise<TokenResponse>;
}

export interface OAuthHandlerOptions {
    tokenExchange?: TokenExchange;
    callbackPath?: string;
}

const SCOPE_PATTERN = /^backlog:(.+)$/;
const COOKIE_PREFIX = "bl_space_";
const COOKIE_MAX_AGE = 300;

function base64url(input: string): string {
    return btoa(input).replace(/\+/g, "-").replace(/\//g, "_").replace(/=+$/, "");
}

/**
 * Derive a stable fingerprint of the authorize session JWT. The session string
 * is embedded once per /mcp/authorize page render, so all popups and the
 * status/complete calls of one page produce the same fingerprint, while
 * distinct authorize sessions (different client/state) produce different ones.
 */
export async function sessionFingerprint(session: string): Promise<string> {
    const digest = await crypto.subtle.digest("SHA-256", new TextEncoder().encode(session));
    const bytes = new Uint8Array(digest);
    let bin = "";
    for (const b of bytes) bin += String.fromCharCode(b);
    return base64url(bin);
}

/**
 * Cookie name binds the space token to the authorize session (sid). A different
 * session computes a different name and therefore cannot observe or consume
 * another session's space cookie, even within the same browser/origin.
 */
export function spaceCookieName(space: string, sid: string): string {
    return COOKIE_PREFIX + base64url(`${space}:${sid}`);
}

export function parseScopes(
    scope: string | undefined,
    config: McpServerConfig,
): SpaceRef[] {
    if (scope) {
        const parts = scope.split(/[\s+]+/).filter(Boolean);
        const spaces: SpaceRef[] = [];
        for (const part of parts) {
            const match = part.match(SCOPE_PATTERN);
            if (match) {
                const spaceHost = match[1];
                // Support both old format (space.domain) and new format (full host)
                if (matchSpacePattern(spaceHost, config.spaces)) {
                    spaces.push({ space: spaceHost });
                }
            }
        }
        if (spaces.length > 0) return spaces;
    }

    if (config.default_spaces.length > 0) {
        const spaces: SpaceRef[] = [];
        for (const key of config.default_spaces) {
            spaces.push({ space: key });
        }
        if (spaces.length > 0) return spaces;
    }

    return [];
}

export function createOAuthHandlers(config: McpServerConfig, keys: SigningKeys, options?: OAuthHandlerOptions): Hono {
    const app = new Hono();
    const { signingKey, signingKid, verifyKeys, encKeys } = keys;

    // Active encryption key, derived from the current signing key. Used to seal
    // raw Backlog tokens before they leave the server inside any JWT/cookie.
    const sealKey = encKeys.get(signingKid);
    if (!sealKey) {
        throw new Error(`No encryption key derived for active signing kid ${signingKid}`);
    }
    /** Seal a raw Backlog token value (sp/use bind it to its slot). */
    const sealValue = (domain: string, use: "at" | "rt", value: string) =>
        seal(value, sealKey, signingKid, domain, use);
    /** Open a sealed value; throws DecryptError on legacy plaintext / bad key. */
    const openValue = (domain: string, use: "at" | "rt", value: string) =>
        open(value, (kid) => encKeys.get(kid), { sp: domain, use });

    const tokenExchange = options?.tokenExchange;

    // Path appended to the resolved base URL to form the Backlog redirect_uri.
    // base_url is derived per-request (resolveBaseUrl); authorize and callback
    // arrive via the same public host, so the redirect_uri matches across both.
    const callbackPath = options?.callbackPath ?? "/mcp/authorize/callback";

    async function doExchangeCode(space: string, code: string, redirectUri: string): Promise<TokenResponse> {
        if (tokenExchange) {
            return tokenExchange.exchangeCode(space, code, redirectUri);
        }
        if (!config.relay_url) {
            throw new Error("relay_url is required when tokenExchange is not provided");
        }
        return exchangeCodeViaRelay(config.relay_url, space, code);
    }

    async function doRefreshToken(space: string, refreshTokenValue: string): Promise<TokenResponse> {
        if (tokenExchange) {
            return tokenExchange.refreshToken(space, refreshTokenValue);
        }
        if (!config.relay_url) {
            throw new Error("relay_url is required when tokenExchange is not provided");
        }
        return refreshViaRelay(config.relay_url, space, refreshTokenValue);
    }

    async function verifyState(jwt: string): Promise<AuthorizeState> {
        const payload = await verify(jwt, verifyKeys);
        return payload as unknown as AuthorizeState;
    }

    async function readSpaceCookie(c: Context, ref: SpaceRef, sid: string): Promise<SpaceToken | null> {
        const name = spaceCookieName(ref.space, sid);
        const value = getCookie(c, name);
        if (!value) return null;
        try {
            const payload = await verify(value, verifyKeys);
            // Defense in depth: the cookie payload must be bound to this session.
            if ((payload as { sid?: string }).sid !== sid) return null;
            return payload as unknown as SpaceToken;
        } catch {
            return null;
        }
    }

    function jsonError(
        c: Context,
        status: number,
        error: string,
        description?: string,
    ): Response {
        const body: TokenErrorResponse = { error, error_description: description };
        return c.json(body, status as 400);
    }

    function getLogger(c: Context): import("../logging/logger.js").Logger {
        return (c.get(LOGGER_CONTEXT_KEY) as import("../logging/logger.js").Logger | undefined) ?? new Logger();
    }

    // POST /mcp/register — Stateless Dynamic Client Registration
    app.post("/mcp/register", async (c) => {
        let req: DcrRequest;
        try {
            req = await c.req.json();
        } catch {
            return jsonError(c, 400, "invalid_request", "Invalid JSON body");
        }

        if (
            !req.redirect_uris ||
            !Array.isArray(req.redirect_uris) ||
            req.redirect_uris.length === 0
        ) {
            return jsonError(
                c,
                400,
                "invalid_client_metadata",
                "redirect_uris is required",
            );
        }

        for (const uri of req.redirect_uris) {
            try {
                new URL(uri);
            } catch {
                return jsonError(
                    c,
                    400,
                    "invalid_redirect_uri",
                    `Invalid redirect_uri: ${uri}`,
                );
            }
        }

        const clientPayload: ClientIdPayload = {
            redirect_uris: req.redirect_uris,
            client_name: req.client_name,
            iat: Math.floor(Date.now() / 1000),
        };

        const clientId = await sign(
            clientPayload as unknown as Record<string, unknown>,
            signingKey,
            signingKid,
        );

        const resp: DcrResponse = {
            client_id: clientId,
            client_name: req.client_name,
            redirect_uris: req.redirect_uris,
            grant_types: ["authorization_code", "refresh_token"],
            response_types: ["code"],
            token_endpoint_auth_method: "none",
        };

        getLogger(c).info({
            component: "oauth",
            action: "dcr",
            client_id: clientId,
            client_payload: clientPayload,
            redirect_uris: req.redirect_uris,
            grant_types: req.grant_types,
            response_types: req.response_types,
            token_endpoint_auth_method: req.token_endpoint_auth_method,
        });

        c.header("Cache-Control", "no-store");
        return c.json(resp, 201);
    });

    // GET /mcp/authorize — Render multi-space auth page or redirect for single space
    app.get("/mcp/authorize", async (c) => {
        const clientId = c.req.query("client_id");
        const redirectUri = c.req.query("redirect_uri");
        const responseType = c.req.query("response_type");
        const codeChallenge = c.req.query("code_challenge");
        const codeChallengeMethod = c.req.query("code_challenge_method");
        const state = c.req.query("state");
        const scope = c.req.query("scope");

        if (!clientId || !redirectUri || !state) {
            return jsonError(
                c,
                400,
                "invalid_request",
                "client_id, redirect_uri, and state are required",
            );
        }

        if (responseType !== "code") {
            return jsonError(
                c,
                400,
                "unsupported_response_type",
                "Only response_type=code is supported",
            );
        }

        if (codeChallengeMethod && codeChallengeMethod !== "S256") {
            return jsonError(
                c,
                400,
                "invalid_request",
                "Only S256 code_challenge_method is supported",
            );
        }

        if (!codeChallenge) {
            return jsonError(
                c,
                400,
                "invalid_request",
                "code_challenge is required (PKCE)",
            );
        }

        let clientPayload: ClientIdPayload;
        try {
            const raw = await verifyClientId(clientId, verifyKeys);
            clientPayload = {
                redirect_uris: raw.redirect_uris ?? [],
                client_name: raw.client_name,
                iat: raw.iat,
            };
        } catch {
            return jsonError(c, 400, "invalid_client", "Invalid client_id");
        }

        if (!clientPayload.redirect_uris.includes(redirectUri)) {
            return jsonError(
                c,
                400,
                "invalid_redirect_uri",
                "redirect_uri does not match registered URIs",
            );
        }

        const requiredSpaces = parseScopes(scope, config);
        if (requiredSpaces.length === 0) {
            return jsonError(
                c,
                400,
                "invalid_scope",
                "Unable to determine Backlog space. Use scope=backlog:space.backlog.jp format",
            );
        }

        const authorizeState: AuthorizeState = {
            client_id: clientId,
            redirect_uri: redirectUri,
            code_challenge: codeChallenge,
            code_challenge_method: codeChallengeMethod || "S256",
            state,
            space: requiredSpaces[0].space,
            requiredSpaces,
        };

        const session = await sign(
            {
                ...authorizeState,
                iat: Math.floor(Date.now() / 1000),
                exp: Math.floor(Date.now() / 1000) + 600,
            },
            signingKey,
            signingKid,
        );

        const spacePatterns = config.spaces.map((s) => s.pattern);
        const auditNotice = config.audit?.collect_user_info !== false;
        c.header("Cache-Control", "no-store");
        return c.html(renderAuthPage(requiredSpaces, session, spacePatterns, auditNotice));
    });

    // GET /mcp/authorize/space — Per-space OAuth popup
    app.get("/mcp/authorize/space", async (c) => {
        const space = c.req.query("space");
        const session = c.req.query("session");

        if (!space || !session) {
            return c.html(errorPage("不正なリクエスト", "パラメーターが不足しています"), 400);
        }

        if (!matchSpacePattern(space, config.spaces)) {
            return c.html(
                errorPage("スペースが許可されていません", `スペース '${space}' はこのサーバーでは許可されていません。`),
                403,
            );
        }

        try {
            await verifyState(session);
        } catch {
            return c.html(errorPage("セッションの有効期限切れ", "もう一度お試しください"), 400);
        }

        const sid = await sessionFingerprint(session);
        const now = Math.floor(Date.now() / 1000);
        const signedState = await sign(
            {
                space,
                popup: true,
                sid,
                iat: now,
                exp: now + 600,
            },
            signingKey,
            signingKid,
        );

        const callbackUrl = `${resolveBaseUrl(c, config.base_url)}${callbackPath}`;
        const authUrl = new URL(
            `https://${space}/OAuth2AccessRequest.action`,
        );
        authUrl.searchParams.set("response_type", "code");
        authUrl.searchParams.set("client_id", config.backlog_app.client_id);
        authUrl.searchParams.set("redirect_uri", callbackUrl);
        authUrl.searchParams.set("state", signedState);

        c.header("Cache-Control", "no-store");
        return c.redirect(authUrl.toString(), 302);
    });

    // GET /mcp/authorize/callback — Backlog redirects here after user consent
    app.get("/mcp/authorize/callback", async (c) => {
        const code = c.req.query("code");
        const signedState = c.req.query("state");
        const errorParam = c.req.query("error");

        if (errorParam) {
            const desc = c.req.query("error_description") || "認可が拒否されました";
            return c.html(errorPage("認可に失敗しました", desc), 400);
        }

        if (!code || !signedState) {
            return c.html(
                errorPage("不正なリクエスト", "認可コードまたはstateが不足しています"),
                400,
            );
        }

        let authorizeState: AuthorizeState;
        try {
            authorizeState = await verifyState(signedState);
        } catch {
            return c.html(
                errorPage("セッションの有効期限切れ", "もう一度お試しください"),
                400,
            );
        }

        let backlogTokens: TokenResponse;
        try {
            backlogTokens = await doExchangeCode(
                authorizeState.space,
                code,
                `${resolveBaseUrl(c, config.base_url)}${callbackPath}`,
            );
        } catch (err) {
            return c.html(
                errorPage(
                    "トークン交換に失敗しました",
                    (err as Error).message,
                ),
                502,
            );
        }

        const now = Math.floor(Date.now() / 1000);

        if (authorizeState.popup) {
            const sid = authorizeState.sid;
            if (!sid) {
                return c.html(
                    errorPage("セッションの有効期限切れ", "もう一度お試しください"),
                    400,
                );
            }

            const spaceToken: SpaceToken = {
                space: authorizeState.space,
                bl_access_token: await sealValue(authorizeState.space, "at", backlogTokens.access_token),
                bl_refresh_token: await sealValue(authorizeState.space, "rt", backlogTokens.refresh_token),
                bl_expires_at: now + backlogTokens.expires_in,
            };

            const cookieValue = await sign(
                { ...spaceToken, sid } as unknown as Record<string, unknown>,
                signingKey,
                signingKid,
            );

            setCookie(c, spaceCookieName(authorizeState.space, sid), cookieValue, {
                httpOnly: true,
                secure: true,
                sameSite: "Lax",
                maxAge: COOKIE_MAX_AGE,
                path: "/mcp/authorize",
            });

            c.header("Cache-Control", "no-store");
            return c.html(popupSuccessPage(authorizeState.space));
        }

        // Legacy single-space flow (no popup flag)
        const mcpCode = await signToken(
            {
                bl_access_token: await sealValue(authorizeState.space, "at", backlogTokens.access_token),
                bl_refresh_token: await sealValue(authorizeState.space, "rt", backlogTokens.refresh_token),
                bl_expires_at: now + backlogTokens.expires_in,
                space: authorizeState.space,
                iat: now,
                exp: now + 300,
            },
            signingKey,
            signingKid,
        );

        const redirectUrl = new URL(authorizeState.redirect_uri);
        redirectUrl.searchParams.set("code", mcpCode);
        redirectUrl.searchParams.set("state", authorizeState.state);

        c.header("Cache-Control", "no-store");
        return c.redirect(redirectUrl.toString(), 302);
    });

    // GET /mcp/authorize/status — Check per-space auth status via cookies
    app.get("/mcp/authorize/status", async (c) => {
        const session = c.req.query("session");
        const spacesParam = c.req.query("spaces");
        if (!session) {
            return c.json({ error: "missing session" }, 400);
        }

        try {
            await verifyState(session);
        } catch {
            return c.json({ error: "invalid session" }, 400);
        }

        const sid = await sessionFingerprint(session);
        const spaceHosts = spacesParam ? spacesParam.split(",").filter(Boolean) : [];
        const statuses: Array<{ space: string; authenticated: boolean }> = [];
        for (const space of spaceHosts) {
            const ref = { space };
            const token = await readSpaceCookie(c, ref, sid);
            statuses.push({
                space,
                authenticated: token !== null,
            });
        }

        return c.json({ spaces: statuses });
    });

    // POST /mcp/authorize/complete — Collect all cookies, issue multi-space code
    app.post("/mcp/authorize/complete", async (c) => {
        let session: string | undefined;
        let spacesParam: string | undefined;

        const contentType = c.req.header("content-type") || "";
        if (contentType.includes("application/x-www-form-urlencoded")) {
            const body = new URLSearchParams(await c.req.text());
            session = body.get("session") || undefined;
            spacesParam = body.get("spaces") || undefined;
        } else {
            session = c.req.query("session");
            spacesParam = c.req.query("spaces");
        }

        if (!session) {
            return c.html(errorPage("不正なリクエスト", "セッション情報が不足しています"), 400);
        }

        let authorizeState: AuthorizeState;
        try {
            authorizeState = await verifyState(session);
        } catch {
            return c.html(errorPage("セッションの有効期限切れ", "もう一度お試しください"), 400);
        }

        const sid = await sessionFingerprint(session);
        const spaceHosts = spacesParam ? spacesParam.split(",").filter(Boolean) : [];
        const spaceTokens: SpaceToken[] = [];
        for (const space of spaceHosts) {
            const ref = { space };
            const token = await readSpaceCookie(c, ref, sid);
            if (token) {
                spaceTokens.push(token);
            }
        }

        if (spaceTokens.length === 0) {
            return c.html(
                errorPage("認証なし", "少なくとも1つのスペースを認証する必要があります"),
                400,
            );
        }

        const primary = spaceTokens[0];
        const now = Math.floor(Date.now() / 1000);

        const codePayloadEntries: Record<string, unknown> = {};
        for (const t of spaceTokens) {
            codePayloadEntries[spaceKey(t.space)] = {
                at: t.bl_access_token,
                rt: t.bl_refresh_token,
                exp: t.bl_expires_at,
            };
        }

        const mcpCode = await sign(
            {
                ...codePayloadEntries,
                space: primary.space,
                iat: now,
                exp: now + 300,
            },
            signingKey,
            signingKid,
        );

        // Clear space cookies
        for (const t of spaceTokens) {
            setCookie(c, spaceCookieName(t.space, sid), "", {
                httpOnly: true,
                secure: true,
                sameSite: "Lax",
                maxAge: 0,
                path: "/mcp/authorize",
            });
        }

        const redirectUrl = new URL(authorizeState.redirect_uri);
        redirectUrl.searchParams.set("code", mcpCode);
        redirectUrl.searchParams.set("state", authorizeState.state);

        c.header("Cache-Control", "no-store");
        return c.redirect(redirectUrl.toString(), 302);
    });

    // POST /mcp/token — Token exchange / refresh
    app.post("/mcp/token", async (c) => {
        let req: TokenRequest;
        try {
            const contentType = c.req.header("content-type") || "";
            if (contentType.includes("application/x-www-form-urlencoded")) {
                const params = new URLSearchParams(await c.req.text());
                req = {
                    grant_type: params.get("grant_type") || "",
                    code: params.get("code") || undefined,
                    redirect_uri: params.get("redirect_uri") || undefined,
                    client_id: params.get("client_id") || undefined,
                    code_verifier: params.get("code_verifier") || undefined,
                    refresh_token: params.get("refresh_token") || undefined,
                };
            } else {
                req = await c.req.json();
            }
        } catch {
            return jsonError(c, 400, "invalid_request", "Invalid request body");
        }

        c.header("Cache-Control", "no-store");

        switch (req.grant_type) {
            case "authorization_code":
                return handleCodeExchange(c, req);
            case "refresh_token":
                return handleRefreshToken(c, req);
            default:
                return jsonError(
                    c,
                    400,
                    "unsupported_grant_type",
                    "Supported: authorization_code, refresh_token",
                );
        }
    });

    async function handleCodeExchange(
        c: Context,
        req: TokenRequest,
    ): Promise<Response> {
        if (!req.code) {
            return jsonError(c, 400, "invalid_request", "code is required");
        }

        let codePayload: Record<string, unknown>;
        try {
            codePayload = await verify(req.code, verifyKeys);
        } catch {
            return jsonError(c, 400, "invalid_grant", "Invalid or expired code");
        }

        const now = Math.floor(Date.now() / 1000);

        // Extract space entries from code payload (new "space:*" format or legacy "spaces" array)
        // verifyToken already normalizes legacy formats, but codePayload is raw verify() output
        type CodeSpaceEntry = { at: string; rt: string; exp: number };
        const codeSpaces: Array<[string, CodeSpaceEntry]> = [];

        // New format: "space:example.backlog.jp" keys
        for (const [key, value] of Object.entries(codePayload)) {
            if (key.startsWith("space:")) {
                const domain = key.slice("space:".length);
                codeSpaces.push([domain, value as CodeSpaceEntry]);
            }
        }

        // Legacy format: "spaces" array
        if (codeSpaces.length === 0) {
            const legacySpaces = codePayload.spaces as SpaceToken[] | undefined;
            if (legacySpaces) {
                for (const s of legacySpaces) {
                    codeSpaces.push([s.space, { at: s.bl_access_token, rt: s.bl_refresh_token, exp: s.bl_expires_at }]);
                }
            }
        }

        // Extract clientName from client_id JWT
        let clientName: string | undefined;
        if (req.client_id) {
            try {
                const clientPayload = await verifyClientId(req.client_id, verifyKeys);
                clientName = clientPayload.client_name;
            } catch { /* ignore invalid client_id */ }
        }

        const collectUserInfo = config.audit?.collect_user_info !== false;

        if (codeSpaces.length > 0) {
            const primarySpace = codePayload.space as string || codeSpaces[0][0];
            const minExpires = Math.min(...codeSpaces.map(([, e]) => e.exp));
            const expiresIn = Math.max(minExpires - now, 60);

            const accessEntries: Record<string, unknown> = {};
            const refreshEntries: Record<string, unknown> = {};
            for (const [domain, e] of codeSpaces) {
                setSpaceAccess(accessEntries, domain, e.at, e.exp);
                setSpaceRefresh(refreshEntries, domain, e.rt);
            }

            // Fetch user email from Backlog API (audit enabled)
            let userEmail: string | undefined;
            if (collectUserInfo) {
                const primaryEntry = codeSpaces.find(([d]) => d === primarySpace) || codeSpaces[0];
                if (primaryEntry) {
                    try {
                        const rawAt = await openValue(primaryEntry[0], "at", primaryEntry[1].at);
                        const user = await fetchCurrentUser(primaryEntry[0], rawAt);
                        userEmail = user?.mailAddress;
                    } catch { /* ignore */ }
                }
            }

            const tokenMeta: Record<string, unknown> = {};
            if (userEmail) tokenMeta.userEmail = userEmail;
            if (clientName) tokenMeta.clientName = clientName;

            const accessTokenJwt = await sign(
                {
                    ...accessEntries,
                    ...tokenMeta,
                    space: primarySpace,
                    iat: now,
                    exp: now + expiresIn,
                },
                signingKey,
                signingKid,
            );

            const refreshTokenJwt = await sign(
                {
                    ...refreshEntries,
                    ...tokenMeta,
                    space: primarySpace,
                    iat: now,
                },
                signingKey,
                signingKid,
            );

            return c.json({
                access_token: accessTokenJwt,
                token_type: "Bearer",
                expires_in: expiresIn,
                refresh_token: refreshTokenJwt,
            });
        }

        // Legacy single-space code (no spaces array, no space:* keys)
        const bl_access_token = codePayload.bl_access_token as string;
        const bl_refresh_token = codePayload.bl_refresh_token as string;
        if (!bl_access_token || !bl_refresh_token) {
            return jsonError(c, 400, "invalid_grant", "Malformed code");
        }

        const bl_expires_at = (codePayload.bl_expires_at as number) ?? now + 3600;
        const expiresIn = Math.max(bl_expires_at - now, 60);

        let space = codePayload.space as string;
        if (space && !space.includes(".") && (codePayload as any).domain) {
            space = `${space}.${(codePayload as any).domain}`;
        }

        // Fetch user email for legacy path
        let legacyUserEmail: string | undefined;
        if (collectUserInfo) {
            try {
                const rawAt = await openValue(space, "at", bl_access_token);
                const user = await fetchCurrentUser(space, rawAt);
                legacyUserEmail = user?.mailAddress;
            } catch { /* ignore */ }
        }

        const legacyMeta: Record<string, unknown> = {};
        if (legacyUserEmail) legacyMeta.userEmail = legacyUserEmail;
        if (clientName) legacyMeta.clientName = clientName;

        const accessPayload: Record<string, unknown> = { ...legacyMeta, space, iat: now, exp: now + expiresIn };
        setSpaceAccess(accessPayload, space, bl_access_token, bl_expires_at);
        const accessTokenJwt = await sign(accessPayload, signingKey, signingKid);

        const refreshPayload: Record<string, unknown> = { ...legacyMeta, space, iat: now };
        setSpaceRefresh(refreshPayload, space, bl_refresh_token);
        const refreshTokenJwt = await sign(refreshPayload, signingKey, signingKid);

        return c.json({
            access_token: accessTokenJwt,
            token_type: "Bearer",
            expires_in: expiresIn,
            refresh_token: refreshTokenJwt,
        });
    }

    async function handleRefreshToken(
        c: Context,
        req: TokenRequest,
    ): Promise<Response> {
        if (!req.refresh_token) {
            return jsonError(
                c,
                400,
                "invalid_request",
                "refresh_token is required",
            );
        }

        let refreshPayload: Record<string, unknown>;
        try {
            refreshPayload = await verify(req.refresh_token, verifyKeys);
        } catch {
            return jsonError(c, 400, "invalid_grant", "Invalid refresh token");
        }

        // Extract refresh entries from payload (new "space:*" format or legacy "spaces" array)
        type RefreshEntry = { domain: string; rt: string };
        const refreshEntries: RefreshEntry[] = [];

        for (const [key, value] of Object.entries(refreshPayload)) {
            if (key.startsWith("space:")) {
                const domain = key.slice("space:".length);
                const entry = value as { rt?: string };
                if (entry.rt) {
                    refreshEntries.push({ domain, rt: entry.rt });
                }
            }
        }

        // Legacy format
        if (refreshEntries.length === 0) {
            const legacySpaces = refreshPayload.spaces as SpaceToken[] | undefined;
            if (legacySpaces) {
                for (const s of legacySpaces) {
                    if (s.bl_refresh_token) {
                        refreshEntries.push({ domain: s.space, rt: s.bl_refresh_token });
                    }
                }
            }
        }

        // Carry forward clientName from previous token
        const prevClientName = refreshPayload.clientName as string | undefined;
        const collectUserInfo = config.audit?.collect_user_info !== false;

        if (refreshEntries.length > 0) {
            const accessEntries: Record<string, unknown> = {};
            const newRefreshEntries: Record<string, unknown> = {};
            const expirations: number[] = [];
            let freshUserEmail: string | undefined;

            const primarySpace = refreshPayload.space as string || refreshEntries[0].domain;

            for (const entry of refreshEntries) {
                let rt: string;
                try {
                    rt = await openValue(entry.domain, "rt", entry.rt);
                } catch {
                    return jsonError(
                        c,
                        400,
                        "invalid_grant",
                        `Stale refresh token for ${entry.domain}. Re-authentication required.`,
                    );
                }
                let tokens: TokenResponse;
                try {
                    tokens = await doRefreshToken(entry.domain, rt);
                } catch {
                    return jsonError(
                        c,
                        403,
                        "insufficient_scope",
                        `Token refresh failed for ${entry.domain}. Re-authentication required.`,
                    );
                }

                // Fetch user email from primary space's fresh access token
                if (collectUserInfo && !freshUserEmail && entry.domain === primarySpace) {
                    const user = await fetchCurrentUser(entry.domain, tokens.access_token);
                    freshUserEmail = user?.mailAddress;
                }

                const now = Math.floor(Date.now() / 1000);
                const expiresAt = now + tokens.expires_in;
                setSpaceAccess(accessEntries, entry.domain, await sealValue(entry.domain, "at", tokens.access_token), expiresAt);
                setSpaceRefresh(newRefreshEntries, entry.domain, await sealValue(entry.domain, "rt", tokens.refresh_token));
                expirations.push(expiresAt);
            }

            if (Object.keys(accessEntries).length === 0) {
                return jsonError(c, 400, "invalid_grant", "No refreshable tokens");
            }

            const now = Math.floor(Date.now() / 1000);
            const minExpires = Math.min(...expirations);
            const expiresIn = Math.max(minExpires - now, 60);

            const refreshMeta: Record<string, unknown> = {};
            if (freshUserEmail) refreshMeta.userEmail = freshUserEmail;
            if (prevClientName) refreshMeta.clientName = prevClientName;

            const accessTokenJwt = await sign(
                {
                    ...accessEntries,
                    ...refreshMeta,
                    space: primarySpace,
                    iat: now,
                    exp: now + expiresIn,
                },
                signingKey,
                signingKid,
            );

            const refreshTokenJwt = await sign(
                {
                    ...newRefreshEntries,
                    ...refreshMeta,
                    space: primarySpace,
                    iat: now,
                },
                signingKey,
                signingKid,
            );

            return c.json({
                access_token: accessTokenJwt,
                token_type: "Bearer",
                expires_in: expiresIn,
                refresh_token: refreshTokenJwt,
            });
        }

        // Legacy single-space refresh (no space:* keys, no spaces array)
        const bl_refresh_token = refreshPayload.bl_refresh_token as string;
        if (!bl_refresh_token) {
            return jsonError(c, 400, "invalid_grant", "Malformed refresh token");
        }

        let space = refreshPayload.space as string;
        if (space && !space.includes(".") && (refreshPayload as any).domain) {
            space = `${space}.${(refreshPayload as any).domain}`;
        }

        let rt: string;
        try {
            rt = await openValue(space, "rt", bl_refresh_token);
        } catch {
            return jsonError(c, 400, "invalid_grant", "Stale refresh token. Re-authentication required.");
        }

        let backlogTokens: TokenResponse;
        try {
            backlogTokens = await doRefreshToken(space, rt);
        } catch (err) {
            return jsonError(
                c,
                502,
                "upstream_error",
                (err as Error).message,
            );
        }

        const now = Math.floor(Date.now() / 1000);
        const expiresAt = now + backlogTokens.expires_in;

        let legacyRefreshEmail: string | undefined;
        if (collectUserInfo) {
            const user = await fetchCurrentUser(space, backlogTokens.access_token);
            legacyRefreshEmail = user?.mailAddress;
        }

        const legacyRefreshMeta: Record<string, unknown> = {};
        if (legacyRefreshEmail) legacyRefreshMeta.userEmail = legacyRefreshEmail;
        if (prevClientName) legacyRefreshMeta.clientName = prevClientName;

        const newAccessPayload: Record<string, unknown> = { ...legacyRefreshMeta, space, iat: now, exp: expiresAt };
        setSpaceAccess(newAccessPayload, space, await sealValue(space, "at", backlogTokens.access_token), expiresAt);
        const accessTokenJwt = await sign(newAccessPayload, signingKey, signingKid);

        const newRefreshPayload: Record<string, unknown> = { ...legacyRefreshMeta, space, iat: now };
        setSpaceRefresh(newRefreshPayload, space, await sealValue(space, "rt", backlogTokens.refresh_token));
        const refreshTokenJwt = await sign(newRefreshPayload, signingKey, signingKid);

        return c.json({
            access_token: accessTokenJwt,
            token_type: "Bearer",
            expires_in: backlogTokens.expires_in,
            refresh_token: refreshTokenJwt,
        });
    }

    return app;
}

// --- Helper functions ---

async function fetchCurrentUser(
    spaceHost: string,
    accessToken: string,
): Promise<{ mailAddress: string } | null> {
    try {
        const resp = await fetch(`https://${spaceHost}/api/v2/users/myself`, {
            headers: { Authorization: `Bearer ${accessToken}` },
        });
        if (!resp.ok) return null;
        const data = (await resp.json()) as { mailAddress?: string };
        return { mailAddress: data.mailAddress ?? "" };
    } catch {
        return null;
    }
}

async function verifyClientId(
    clientId: string,
    verifyKeys: Map<string, import("jose").CryptoKey>,
): Promise<ClientIdPayload> {
    const payload = await verify(clientId, verifyKeys);
    return payload as unknown as ClientIdPayload;
}

async function exchangeCodeViaRelay(
    relayUrl: string,
    space: string,
    code: string,
): Promise<TokenResponse> {
    const resp = await fetch(`${relayUrl}/auth/token`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
            grant_type: "authorization_code",
            code,
            space,
        }),
    });

    if (!resp.ok) {
        const body = await resp.text();
        throw new Error(`Relay token exchange failed: ${body}`);
    }

    return (await resp.json()) as TokenResponse;
}

async function refreshViaRelay(
    relayUrl: string,
    space: string,
    refreshToken: string,
): Promise<TokenResponse> {
    const resp = await fetch(`${relayUrl}/auth/token`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
            grant_type: "refresh_token",
            refresh_token: refreshToken,
            space,
        }),
    });

    if (!resp.ok) {
        const body = await resp.text();
        throw new Error(`Relay token refresh failed: ${body}`);
    }

    return (await resp.json()) as TokenResponse;
}

const BASE_STYLE = `* { margin:0; padding:0; box-sizing:border-box; }
body {
  font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, "Helvetica Neue", Arial, sans-serif;
  background: linear-gradient(135deg, #1a1a2e 0%, #16213e 100%);
  color: #e8e8e8; min-height:100vh; display:flex; align-items:center; justify-content:center; padding:20px;
}
.container { max-width:520px; width:100%; text-align:center; }
h1 { font-size:1.5rem; font-weight:600; margin-bottom:1rem; color:#fff; }
p { font-size:.95rem; line-height:1.6; color:#b0b0b0; }
code {
  background:rgba(66,184,131,.15); color:#42b883; padding:.15rem .4rem;
  border-radius:4px; font-family:"SF Mono",Monaco,"Cascadia Code",monospace; font-size:.85rem;
}
.card {
  background:rgba(255,255,255,.05); border:1px solid rgba(255,255,255,.1);
  border-radius:12px; padding:1.5rem; text-align:left;
}
.footer { margin-top:2rem; font-size:.8rem; color:#666; }`;

function errorPage(title: string, message: string): string {
    return `<!DOCTYPE html>
<html><head><meta charset="UTF-8"><meta name="viewport" content="width=device-width,initial-scale=1.0">
<title>${escapeHtml(title)}</title>
<style>${BASE_STYLE}</style>
</head><body>
<div class="container">
  <h1>${escapeHtml(title)}</h1>
  <div class="card" style="text-align:center;">
    <p>${escapeHtml(message)}</p>
    <p style="margin-top:1rem;color:#666;">このウィンドウを閉じることができます。</p>
  </div>
  <p class="footer">Backlog CLI OAuth 中継サーバー</p>
</div>
</body></html>`;
}

function popupSuccessPage(space: string): string {
    return `<!DOCTYPE html>
<html><head><meta charset="UTF-8"><meta name="viewport" content="width=device-width,initial-scale=1.0">
<title>認証完了</title>
<style>${BASE_STYLE}
.check { font-size:3rem; margin-bottom:1rem; }</style>
</head><body>
<div class="container">
  <div class="check">&#x2705;</div>
  <h1>認証完了</h1>
  <div class="card" style="text-align:center;">
    <p><code>${escapeHtml(space)}</code> に接続しました</p>
    <p style="margin-top:.75rem;color:#666;">このウィンドウは自動的に閉じます。</p>
  </div>
</div>
<script>
if(window.opener){window.opener.postMessage({type:"backlog-space-auth",space:${JSON.stringify(space)},ok:true},window.location.origin);}
setTimeout(()=>window.close(),1500);
</script>
</body></html>`;
}

function renderAuthPage(spaces: SpaceRef[], session: string, spacePatterns: string[], auditNotice: boolean): string {
    const auditHtml = auditNotice
        ? `<p class="audit-notice">セキュリティ監査のため、認証時にユーザーのメールアドレスを取得しアクセスログに記録します。</p>`
        : "";
    return `<!DOCTYPE html>
<html lang="ja">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>Backlog スペース認証</title>
<style>
${BASE_STYLE}
.container { text-align:left; }
.subtitle { font-size:.85rem; color:#888; margin-bottom:1.5rem; text-align:center; }
.audit-notice { font-size:.8rem; color:#b0a060; margin-bottom:1.5rem; text-align:center; }
h1 { text-align:center; }
#spaces { margin-bottom:1rem; }
.space { display:flex; align-items:center; gap:.75rem; padding:.65rem 0; border-bottom:1px solid rgba(255,255,255,.08); }
.space:last-child { border-bottom:none; }
.name { flex:1; font-family:"SF Mono",Monaco,"Cascadia Code",monospace; font-size:.9rem; color:#e8e8e8; word-break:break-all; }
.status { font-size:.9rem; min-width:24px; text-align:center; color:#42b883; }
.auth-btn {
  padding:.35rem .75rem; border:1px solid rgba(66,184,131,.4); background:rgba(66,184,131,.15);
  color:#42b883; border-radius:6px; cursor:pointer; font-size:.8rem; white-space:nowrap; transition:all .2s;
}
.auth-btn:hover { background:rgba(66,184,131,.25); border-color:#42b883; }
.auth-btn:disabled { background:rgba(255,255,255,.05); border-color:rgba(255,255,255,.1); color:#666; cursor:default; }
.remove-btn {
  background:none; border:none; color:#555; cursor:pointer; font-size:1.1rem; padding:0 .3rem; transition:color .2s;
}
.remove-btn:hover { color:#e74c3c; }
.add-row { display:flex; gap:.5rem; margin-top:.75rem; }
.add-row input {
  flex:1; padding:.45rem .6rem; border:1px solid rgba(255,255,255,.15); border-radius:6px;
  font-size:.85rem; font-family:"SF Mono",Monaco,"Cascadia Code",monospace;
  background:rgba(255,255,255,.05); color:#e8e8e8;
}
.add-row input::placeholder { color:#555; }
.add-row input:focus { outline:none; border-color:#42b883; }
.add-row button {
  padding:.45rem .8rem; border:1px solid rgba(66,184,131,.4); background:rgba(66,184,131,.15);
  color:#42b883; border-radius:6px; cursor:pointer; font-size:.85rem; white-space:nowrap; transition:all .2s;
}
.add-row button:hover { background:rgba(66,184,131,.25); border-color:#42b883; }
.continue-btn {
  display:block; width:100%; margin-top:1.5rem; padding:.75rem; border:none;
  background:#42b883; color:#fff; font-size:1rem; border-radius:8px; cursor:pointer; transition:background .2s;
}
.continue-btn:disabled { background:rgba(255,255,255,.1); color:#555; cursor:default; }
.continue-btn:hover:not(:disabled) { background:#38a373; }
.error { color:#e74c3c; font-size:.8rem; margin-top:.3rem; display:none; }
</style>
</head>
<body>
<div class="container">
  <h1>Backlog スペースの認証</h1>
  <p class="subtitle">続行するには、少なくとも1つのスペースを認証してください。必要に応じて追加できます。</p>
  ${auditHtml}
  <div class="card">
    <div id="spaces"></div>
    <div class="add-row">
      <input type="text" id="newSpace" placeholder="space.backlog.jp" />
      <button onclick="addSpaceFromInput()">+ 追加</button>
    </div>
    <div class="error" id="addError"></div>
    <form id="completeForm" method="POST" action="/mcp/authorize/complete">
      <input type="hidden" name="session" value="${escapeHtml(session)}" />
      <input type="hidden" name="spaces" id="spacesInput" value="" />
      <button type="submit" class="continue-btn" id="continueBtn" disabled>続行</button>
    </form>
  </div>
  <p class="footer" style="text-align:center;">Backlog CLI OAuth 中継サーバー</p>
</div>
<script>
const SESSION = ${JSON.stringify(session)};
const INITIAL = ${JSON.stringify(spaces.map((s) => s.space))};
const SPACE_PATTERNS = ${JSON.stringify(spacePatterns)}.map(p => new RegExp("^" + p + "$"));
const LS_KEY = "backlog_mcp_spaces";
const authenticated = new Set();
const allSpaces = new Map();

function isSpaceAllowed(key) {
  return SPACE_PATTERNS.some(re => re.test(key));
}

function loadSavedSpaces() {
  try { return JSON.parse(localStorage.getItem(LS_KEY) || "[]"); } catch { return []; }
}
function saveSpaces() {
  const keys = [...allSpaces.keys()];
  try { localStorage.setItem(LS_KEY, JSON.stringify(keys)); } catch {}
}

function addSpace(key, save) {
  if (allSpaces.has(key)) return;
  allSpaces.set(key, {});
  renderSpaces();
  if (save) saveSpaces();
}

function removeSpace(key) {
  if (authenticated.has(key)) return;
  allSpaces.delete(key);
  renderSpaces();
  saveSpaces();
}

function renderSpaces() {
  const container = document.getElementById("spaces");
  container.innerHTML = "";
  for (const key of allSpaces.keys()) {
    const done = authenticated.has(key);
    const div = document.createElement("div");
    div.className = "space";
    div.dataset.key = key;
    div.innerHTML =
      '<span class="name">' + esc(key) + '</span>' +
      '<span class="status">' + (done ? '\\u2713' : '') + '</span>' +
      (done
        ? '<button class="auth-btn" disabled>完了</button>'
        : '<button class="auth-btn" onclick="authSpace(\\''+esc(key)+'\\',event)">認証</button>') +
      (done ? '' : '<button class="remove-btn" onclick="removeSpace(\\''+esc(key)+'\\')">\\u00d7</button>');
    container.appendChild(div);
  }
  updateContinue();
}

function authSpace(space, evt) {
  const el = document.querySelector('[data-key="'+space+'"]');
  if (el) {
    el.querySelector(".auth-btn").disabled = true;
    el.querySelector(".auth-btn").textContent = "認証中...";
  }
  const w = 600, h = 700;
  let left = window.screenX + (window.outerWidth - w) / 2;
  let top = window.screenY + (window.outerHeight - h) / 2;
  if (evt && evt.target) {
    const rect = evt.target.getBoundingClientRect();
    left = window.screenX + rect.right + 8;
    top = window.screenY + window.outerHeight - window.innerHeight + rect.top;
    if (left + w > screen.availLeft + screen.availWidth) left = window.screenX + rect.left - w - 8;
    if (top + h > screen.availTop + screen.availHeight) top = screen.availTop + screen.availHeight - h;
    if (top < screen.availTop) top = screen.availTop;
  }
  const url = "/mcp/authorize/space?space=" + encodeURIComponent(space)
    + "&session=" + encodeURIComponent(SESSION);
  window.open(url, "backlog_auth_" + space, "width="+w+",height="+h+",left="+Math.round(left)+",top="+Math.round(top)+",popup=yes");
}

function markDone(key) {
  authenticated.add(key);
  renderSpaces();
}

function updateContinue() {
  document.getElementById("continueBtn").disabled = authenticated.size === 0;
  document.getElementById("spacesInput").value = [...authenticated].join(",");
}

function addSpaceFromInput() {
  const input = document.getElementById("newSpace");
  const errEl = document.getElementById("addError");
  const val = input.value.trim();
  errEl.style.display = "none";
  if (!val) return;
  if (!/^[a-z0-9-]+\\.[a-z0-9.-]+$/i.test(val)) {
    errEl.textContent = "形式: space.backlog.jp または space.backlog.com";
    errEl.style.display = "block";
    return;
  }
  if (!isSpaceAllowed(val)) {
    errEl.textContent = "このスペースはこのサーバーでは許可されていません。";
    errEl.style.display = "block";
    return;
  }
  addSpace(val, true);
  input.value = "";
}

document.getElementById("newSpace").addEventListener("keydown", (e) => {
  if (e.key === "Enter") { e.preventDefault(); addSpaceFromInput(); }
});

window.addEventListener("message", (e) => {
  if (e.origin !== window.location.origin) return;
  if (e.data && e.data.type === "backlog-space-auth" && e.data.ok) {
    markDone(e.data.space);
  }
});

setInterval(async () => {
  const keys = [...allSpaces.keys()].filter(k => !authenticated.has(k));
  if (keys.length === 0) return;
  try {
    const resp = await fetch("/mcp/authorize/status?session=" + encodeURIComponent(SESSION) + "&spaces=" + encodeURIComponent(keys.join(",")));
    const data = await resp.json();
    for (const s of data.spaces || []) {
      if (s.authenticated) markDone(s.space);
    }
  } catch {}
}, 2000);

function esc(s) { return s.replace(/&/g,"&amp;").replace(/</g,"&lt;").replace(/>/g,"&gt;").replace(/"/g,"&quot;").replace(/'/g,"&#39;"); }

// Init: merge scope spaces + localStorage saved spaces, filtered by space patterns
const saved = loadSavedSpaces();
const merged = new Set([...INITIAL, ...saved]);
for (const key of merged) { if (isSpaceAllowed(key)) addSpace(key, false); }
saveSpaces();
</script>
</body>
</html>`;
}

function escapeHtml(s: string): string {
    return s.replace(/&/g, "&amp;").replace(/</g, "&lt;").replace(/>/g, "&gt;").replace(/"/g, "&quot;").replace(/'/g, "&#39;");
}
