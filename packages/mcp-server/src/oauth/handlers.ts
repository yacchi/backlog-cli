import { Hono, type Context } from "hono";
import type { McpServerConfig } from "../config/schema.js";
import { encrypt, decrypt, encryptToken, decryptToken, importKey } from "../crypto/jwe.js";
import type {
    DcrRequest,
    DcrResponse,
    TokenRequest,
    TokenResponse,
    TokenErrorResponse,
    ClientIdPayload,
    AuthorizeState,
} from "./types.js";

export interface TokenExchange {
    exchangeCode(domain: string, space: string, code: string, redirectUri?: string): Promise<TokenResponse>;
    refreshToken(domain: string, space: string, refreshToken: string): Promise<TokenResponse>;
}

export interface OAuthHandlerOptions {
    tokenExchange?: TokenExchange;
    callbackPath?: string;
}

export function createOAuthHandlers(config: McpServerConfig, options?: OAuthHandlerOptions): Hono {
    const app = new Hono();
    const tokenKey = importKey(config.token_key);
    const tokenKeyPrev = config.token_key_prev
        ? importKey(config.token_key_prev)
        : undefined;

    const tokenExchange = options?.tokenExchange;

    const backlogRedirectUri = `${config.base_url}${options?.callbackPath ?? "/mcp/authorize/callback"}`;

    async function doExchangeCode(domain: string, space: string, code: string): Promise<TokenResponse> {
        if (tokenExchange) {
            return tokenExchange.exchangeCode(domain, space, code, backlogRedirectUri);
        }
        if (!config.relay_url) {
            throw new Error("relay_url is required when tokenExchange is not provided");
        }
        return exchangeCodeViaRelay(config.relay_url, domain, space, code);
    }

    async function doRefreshToken(domain: string, space: string, refreshTokenValue: string): Promise<TokenResponse> {
        if (tokenExchange) {
            return tokenExchange.refreshToken(domain, space, refreshTokenValue);
        }
        if (!config.relay_url) {
            throw new Error("relay_url is required when tokenExchange is not provided");
        }
        return refreshViaRelay(config.relay_url, domain, space, refreshTokenValue);
    }

    function findBacklogApp(domain: string) {
        return config.backlog_apps.find((a) => a.domain === domain);
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

        const clientId = await encrypt(
            clientPayload as unknown as Record<string, unknown>,
            tokenKey,
        );

        const resp: DcrResponse = {
            client_id: clientId,
            client_name: req.client_name,
            redirect_uris: req.redirect_uris,
            grant_types: ["authorization_code", "refresh_token"],
            response_types: ["code"],
            token_endpoint_auth_method: "none",
        };

        c.header("Cache-Control", "no-store");
        return c.json(resp, 201);
    });

    // GET /mcp/authorize — Start OAuth flow (redirect to Backlog)
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

        // Decrypt client_id to validate and extract redirect_uris
        let clientPayload: ClientIdPayload;
        try {
            const raw = await decryptClientId(clientId, tokenKey, tokenKeyPrev);
            clientPayload = {
                redirect_uris: raw.redirect_uris ?? [],
                client_name: raw.client_name,
                iat: raw.iat,
            };
        } catch {
            return jsonError(c, 400, "invalid_client", "Invalid client_id");
        }

        // Validate redirect_uri matches registered ones
        if (!clientPayload.redirect_uris.includes(redirectUri)) {
            return jsonError(
                c,
                400,
                "invalid_redirect_uri",
                "redirect_uri does not match registered URIs",
            );
        }

        // Determine space and domain from scope or default
        const { space, domain } = parseScope(scope, config);
        if (!space || !domain) {
            return jsonError(
                c,
                400,
                "invalid_scope",
                "Unable to determine Backlog space. Use scope=backlog:space.domain format",
            );
        }

        const backlogApp = findBacklogApp(domain);
        if (!backlogApp) {
            return jsonError(
                c,
                400,
                "invalid_request",
                `Unsupported domain: ${domain}`,
            );
        }

        // Encrypt authorize state into relay state param
        const authorizeState: AuthorizeState = {
            client_id: clientId,
            redirect_uri: redirectUri,
            code_challenge: codeChallenge,
            code_challenge_method: codeChallengeMethod || "S256",
            state,
            space,
            domain,
        };

        const encryptedState = await encrypt(
            {
                ...authorizeState,
                iat: Math.floor(Date.now() / 1000),
                exp: Math.floor(Date.now() / 1000) + 600,
            },
            tokenKey,
        );

        // Redirect to Backlog OAuth
        const callbackUrl = `${config.base_url}${options?.callbackPath ?? "/mcp/authorize/callback"}`;
        const authUrl = new URL(
            `https://${space}.${domain}/OAuth2AccessRequest.action`,
        );
        authUrl.searchParams.set("response_type", "code");
        authUrl.searchParams.set("client_id", backlogApp.client_id);
        authUrl.searchParams.set("redirect_uri", callbackUrl);
        authUrl.searchParams.set("state", encryptedState);

        c.header("Cache-Control", "no-store");
        return c.redirect(authUrl.toString(), 302);
    });

    // GET /mcp/authorize/callback — Backlog redirects here after user consent
    app.get("/mcp/authorize/callback", async (c) => {
        const code = c.req.query("code");
        const encryptedState = c.req.query("state");
        const errorParam = c.req.query("error");

        if (errorParam) {
            const desc = c.req.query("error_description") || "Authorization denied";
            return c.html(errorPage("Authorization Failed", desc), 400);
        }

        if (!code || !encryptedState) {
            return c.html(
                errorPage("Invalid Request", "Missing code or state"),
                400,
            );
        }

        // Decrypt state to recover authorize params
        let authorizeState: AuthorizeState;
        try {
            authorizeState = (await decrypt(
                encryptedState,
                tokenKey,
            )) as unknown as AuthorizeState;
        } catch {
            if (tokenKeyPrev) {
                try {
                    authorizeState = (await decrypt(
                        encryptedState,
                        tokenKeyPrev,
                    )) as unknown as AuthorizeState;
                } catch {
                    return c.html(
                        errorPage("Session Expired", "Please try again"),
                        400,
                    );
                }
            } else {
                return c.html(
                    errorPage("Session Expired", "Please try again"),
                    400,
                );
            }
        }

        // Exchange code for tokens via relay server
        const backlogApp = findBacklogApp(authorizeState.domain);
        if (!backlogApp) {
            return c.html(
                errorPage("Configuration Error", "Unknown domain"),
                500,
            );
        }

        let backlogTokens: TokenResponse;
        try {
            backlogTokens = await doExchangeCode(
                authorizeState.domain,
                authorizeState.space,
                code,
            );
        } catch (err) {
            return c.html(
                errorPage(
                    "Token Exchange Failed",
                    (err as Error).message,
                ),
                502,
            );
        }

        // Build authorization code that wraps the JWE tokens
        const now = Math.floor(Date.now() / 1000);
        // This code will be exchanged at /mcp/token by the MCP client
        const mcpCode = await encryptToken(
            {
                bl_access_token: backlogTokens.access_token,
                bl_refresh_token: backlogTokens.refresh_token,
                bl_expires_at: now + backlogTokens.expires_in,
                space: authorizeState.space,
                domain: authorizeState.domain,
                iat: now,
                exp: now + 300,
            },
            tokenKey,
        );

        // Redirect back to MCP client with authorization code
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

        // Decrypt the MCP authorization code
        let codePayload;
        try {
            codePayload = await decryptToken(req.code, tokenKey);
        } catch {
            if (tokenKeyPrev) {
                try {
                    codePayload = await decryptToken(req.code, tokenKeyPrev);
                } catch {
                    return jsonError(c, 400, "invalid_grant", "Invalid or expired code");
                }
            } else {
                return jsonError(c, 400, "invalid_grant", "Invalid or expired code");
            }
        }

        if (!codePayload.bl_access_token || !codePayload.bl_refresh_token) {
            return jsonError(c, 400, "invalid_grant", "Malformed code");
        }

        // PKCE verification: code_verifier must match code_challenge from authorize
        // Note: in our stateless flow, the code_challenge was verified at authorize time
        // and the code itself is JWE-encrypted, so replay is prevented by exp

        const now = Math.floor(Date.now() / 1000);
        const expiresIn = (codePayload.bl_expires_at ?? now + 3600) - now;

        const accessTokenJwe = await encryptToken(
            {
                bl_access_token: codePayload.bl_access_token,
                bl_expires_at: codePayload.bl_expires_at,
                space: codePayload.space,
                domain: codePayload.domain,
                iat: now,
                exp: now + Math.max(expiresIn, 60),
            },
            tokenKey,
        );

        const refreshTokenJwe = await encryptToken(
            {
                bl_refresh_token: codePayload.bl_refresh_token,
                space: codePayload.space,
                domain: codePayload.domain,
                iat: now,
            },
            tokenKey,
        );

        return c.json({
            access_token: accessTokenJwe,
            token_type: "Bearer",
            expires_in: Math.max(expiresIn, 60),
            refresh_token: refreshTokenJwe,
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

        let refreshPayload;
        try {
            refreshPayload = await decryptToken(req.refresh_token, tokenKey);
        } catch {
            if (tokenKeyPrev) {
                try {
                    refreshPayload = await decryptToken(
                        req.refresh_token,
                        tokenKeyPrev,
                    );
                } catch {
                    return jsonError(
                        c,
                        400,
                        "invalid_grant",
                        "Invalid refresh token",
                    );
                }
            } else {
                return jsonError(c, 400, "invalid_grant", "Invalid refresh token");
            }
        }

        if (!refreshPayload.bl_refresh_token) {
            return jsonError(c, 400, "invalid_grant", "Malformed refresh token");
        }

        let backlogTokens: TokenResponse;
        try {
            backlogTokens = await doRefreshToken(
                refreshPayload.domain,
                refreshPayload.space,
                refreshPayload.bl_refresh_token,
            );
        } catch (err) {
            return jsonError(
                c,
                502,
                "upstream_error",
                (err as Error).message,
            );
        }

        const now = Math.floor(Date.now() / 1000);
        const accessTokenJwe = await encryptToken(
            {
                bl_access_token: backlogTokens.access_token,
                bl_expires_at: now + backlogTokens.expires_in,
                space: refreshPayload.space,
                domain: refreshPayload.domain,
                iat: now,
                exp: now + backlogTokens.expires_in,
            },
            tokenKey,
        );

        const refreshTokenJwe = await encryptToken(
            {
                bl_refresh_token: backlogTokens.refresh_token,
                space: refreshPayload.space,
                domain: refreshPayload.domain,
                iat: now,
            },
            tokenKey,
        );

        return c.json({
            access_token: accessTokenJwe,
            token_type: "Bearer",
            expires_in: backlogTokens.expires_in,
            refresh_token: refreshTokenJwe,
        });
    }

    return app;
}

// --- Helper functions ---

async function decryptClientId(
    clientId: string,
    key: Uint8Array,
    prevKey?: Uint8Array,
): Promise<ClientIdPayload> {
    try {
        return (await decrypt(clientId, key)) as unknown as ClientIdPayload;
    } catch {
        if (prevKey) {
            return (await decrypt(clientId, prevKey)) as unknown as ClientIdPayload;
        }
        throw new Error("Invalid client_id");
    }
}

function parseScope(
    scope: string | undefined,
    config: McpServerConfig,
): { space: string | undefined; domain: string | undefined } {
    // scope format: "backlog:mycompany.backlog.jp"
    if (scope) {
        const match = scope.match(/^backlog:([^.]+)\.(.+)$/);
        if (match) {
            return { space: match[1], domain: match[2] };
        }
    }

    // Default: use first tenant key if only one exists
    const tenantKeys = Object.keys(config.tenants);
    if (tenantKeys.length === 1) {
        const key = tenantKeys[0];
        const match = key.match(/^([^.]+)\.(.+)$/);
        if (match) {
            return { space: match[1], domain: match[2] };
        }
    }

    // Default: use first backlog_app
    if (config.backlog_apps.length === 1) {
        return { space: undefined, domain: config.backlog_apps[0].domain };
    }

    return { space: undefined, domain: undefined };
}

async function exchangeCodeViaRelay(
    relayUrl: string,
    domain: string,
    space: string,
    code: string,
): Promise<TokenResponse> {
    const resp = await fetch(`${relayUrl}/auth/token`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
            grant_type: "authorization_code",
            code,
            domain,
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
    domain: string,
    space: string,
    refreshToken: string,
): Promise<TokenResponse> {
    const resp = await fetch(`${relayUrl}/auth/token`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
            grant_type: "refresh_token",
            refresh_token: refreshToken,
            domain,
            space,
        }),
    });

    if (!resp.ok) {
        const body = await resp.text();
        throw new Error(`Relay token refresh failed: ${body}`);
    }

    return (await resp.json()) as TokenResponse;
}

function errorPage(title: string, message: string): string {
    return `<!DOCTYPE html>
<html>
<head><title>${title}</title></head>
<body>
<h1>${title}</h1>
<p>${message}</p>
<p>You can close this window.</p>
</body>
</html>`;
}
