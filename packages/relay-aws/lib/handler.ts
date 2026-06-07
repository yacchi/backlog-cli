/**
 * AWS Lambda adapter for Backlog OAuth Relay Server.
 * Optionally serves MCP endpoints when MCP integration is configured.
 */

import { Hono } from "hono";
import { handle, type LambdaEvent, type LambdaContext } from "hono/aws-lambda";
import {
    createRelayApp,
    createBundle,
    verifyPassphrase,
    parseConfig,
    type RelayConfig,
    type AuditLogger,
    type AuditEvent,
    type PortalAssets,
} from "@backlog-cli/relay-core";
import {
    createMcpApp,
    createSandboxClient,
    decrypt,
    importKey,
    parseConfig as parseMcpConfig,
    type McpServerConfig,
    type CreateMcpAppOptions,
    type TokenExchange,
} from "@backlog-cli/mcp-server";
import { cors } from "hono/cors";
import { loadPortalAssets } from "./portal-assets.js";

/**
 * Environment variable names for AWS Lambda.
 */
export const ENV_VARS = {
    RELAY_CONFIG: "RELAY_CONFIG",
    CONFIG_PARAMETER_NAME: "CONFIG_PARAMETER_NAME",
    RELAY_SECRETS_NAME: "RELAY_SECRETS_NAME",
    TOKEN_KEY_SECRET_NAME: "TOKEN_KEY_SECRET_NAME",
    BACKLOG_BIN_PATH: "BACKLOG_BIN_PATH",
    SANDBOX_WORKER_PATH: "SANDBOX_WORKER_PATH",
} as const;

interface RelaySecrets {
    apps: Record<string, { client_secret: string }>;
    tenants: Record<string, { jwks?: string; passphrase_hash?: string }>;
}

let cachedRelayConfig: RelayConfig | null = null;
let cachedRawConfig: Record<string, unknown> | null = null;
let cachedRelaySecrets: RelaySecrets | null = null;
let cachedSandbox: Awaited<ReturnType<typeof createSandboxClient>> | null = null;

/**
 * Load raw JSON configuration from environment or SSM.
 */
async function loadRawConfig(): Promise<Record<string, unknown>> {
    if (cachedRawConfig) {
        return cachedRawConfig;
    }

    const envConfig = process.env[ENV_VARS.RELAY_CONFIG];
    if (envConfig) {
        cachedRawConfig = JSON.parse(envConfig) as Record<string, unknown>;
        return cachedRawConfig;
    }

    const parameterName = process.env[ENV_VARS.CONFIG_PARAMETER_NAME];
    if (!parameterName) {
        throw new Error(
            `Either ${ENV_VARS.RELAY_CONFIG} or ${ENV_VARS.CONFIG_PARAMETER_NAME} environment variable is required`,
        );
    }

    const { SSMClient, GetParameterCommand } = await import(
        "@aws-sdk/client-ssm"
    );
    const client = new SSMClient({});
    const response = await client.send(
        new GetParameterCommand({
            Name: parameterName,
            WithDecryption: true,
        }),
    );

    if (!response.Parameter?.Value) {
        throw new Error(`SSM parameter ${parameterName} not found or empty`);
    }

    cachedRawConfig = JSON.parse(response.Parameter.Value) as Record<string, unknown>;
    return cachedRawConfig;
}

/**
 * Load relay secrets from Secrets Manager.
 */
async function loadRelaySecrets(
    secretName: string,
): Promise<RelaySecrets> {
    if (cachedRelaySecrets) {
        return cachedRelaySecrets;
    }

    const {
        SecretsManagerClient,
        GetSecretValueCommand,
    } = await import("@aws-sdk/client-secrets-manager");
    const client = new SecretsManagerClient({});
    const resp = await client.send(
        new GetSecretValueCommand({ SecretId: secretName }),
    );
    if (!resp.SecretString) {
        throw new Error(`Secret ${secretName} not found or empty`);
    }

    cachedRelaySecrets = JSON.parse(resp.SecretString) as RelaySecrets;
    return cachedRelaySecrets;
}

/**
 * Parse relay configuration.
 * Merges secrets from SM into SSM config before passing to relay-core's Zod parser.
 * Unknown keys like mcp_tenants are stripped by Zod.
 */
async function getRelayConfig(): Promise<RelayConfig> {
    if (cachedRelayConfig) {
        return cachedRelayConfig;
    }

    const raw = await loadRawConfig();

    const secretName = process.env[ENV_VARS.RELAY_SECRETS_NAME];
    if (secretName) {
        const secrets = await loadRelaySecrets(secretName);

        if (Array.isArray(raw.backlog_apps) && secrets.apps) {
            raw.backlog_apps = (raw.backlog_apps as Array<Record<string, unknown>>).map((app) => ({
                ...app,
                client_secret: secrets.apps[app.domain as string]?.client_secret
                    ?? (app.client_secret || ""),
            }));
        }

        if (Array.isArray(raw.tenants) && secrets.tenants) {
            raw.tenants = (raw.tenants as Array<Record<string, unknown>>).map((t) => ({
                ...t,
                jwks: secrets.tenants[t.allowed_domain as string]?.jwks ?? t.jwks,
                passphrase_hash: secrets.tenants[t.allowed_domain as string]?.passphrase_hash
                    ?? t.passphrase_hash,
            }));
        }
    }

    cachedRelayConfig = parseConfig(JSON.stringify(raw));
    return cachedRelayConfig;
}

/**
 * Fetch JWE token keys from Secrets Manager.
 */
async function getTokenKeys(
    secretName: string,
): Promise<{ current: string; previous?: string }> {
    const {
        SecretsManagerClient,
        GetSecretValueCommand,
    } = await import("@aws-sdk/client-secrets-manager");
    const client = new SecretsManagerClient({});

    const currentResp = await client.send(
        new GetSecretValueCommand({
            SecretId: secretName,
            VersionStage: "AWSCURRENT",
        }),
    );
    if (!currentResp.SecretString) {
        throw new Error(`Secret ${secretName} (AWSCURRENT) not found or empty`);
    }

    let previous: string | undefined;
    try {
        const prevResp = await client.send(
            new GetSecretValueCommand({
                SecretId: secretName,
                VersionStage: "AWSPREVIOUS",
            }),
        );
        previous = prevResp.SecretString ?? undefined;
    } catch {
        // AWSPREVIOUS doesn't exist yet (no rotation has occurred)
    }

    return { current: currentResp.SecretString, previous };
}

/**
 * Build McpServerConfig from raw SSM config + relay config + Secrets Manager keys.
 */
async function buildMcpConfig(
    relayConfig: RelayConfig,
    eventBaseUrl?: string,
): Promise<McpServerConfig | null> {
    const raw = await loadRawConfig();
    const mcpTenants = raw.mcp_tenants as Record<string, unknown> | undefined;
    if (!mcpTenants || Object.keys(mcpTenants).length === 0) {
        return null;
    }

    const secretName = process.env[ENV_VARS.TOKEN_KEY_SECRET_NAME];
    if (!secretName) {
        return null;
    }

    const { current, previous } = await getTokenKeys(secretName);

    const baseUrl = relayConfig.server.base_url || eventBaseUrl;
    if (!baseUrl) {
        console.warn("MCP integration requires server.base_url in relay config");
        return null;
    }

    const mcpConfigObj: Record<string, unknown> = {
        base_url: baseUrl,
        token_key: current,
        token_key_prev: previous,
        backlog_apps: relayConfig.backlog_apps.map((a) => ({
            domain: a.domain,
            client_id: a.client_id,
        })),
        tenants: mcpTenants,
    };

    return parseMcpConfig(JSON.stringify(mcpConfigObj));
}

/**
 * Create an in-process TokenExchange using the relay's BacklogAppConfig
 * (which includes client_secret). No HTTP round-trip needed.
 */
function createDirectTokenExchange(relayConfig: RelayConfig): TokenExchange {
    function findApp(domain: string) {
        const app = relayConfig.backlog_apps.find((a) => a.domain === domain);
        if (!app) {
            throw new Error(`Unsupported domain: ${domain}`);
        }
        return app;
    }

    async function requestToken(
        tokenUrl: string,
        params: URLSearchParams,
    ): Promise<{ access_token: string; token_type: string; expires_in: number; refresh_token: string }> {
        const response = await fetch(tokenUrl, {
            method: "POST",
            headers: { "Content-Type": "application/x-www-form-urlencoded" },
            body: params.toString(),
        });
        const body = await response.text();
        if (!response.ok) {
            throw new Error(`Token request failed: ${body}`);
        }
        return JSON.parse(body);
    }

    return {
        async exchangeCode(domain, space, code, redirectUri) {
            const app = findApp(domain);
            const params = new URLSearchParams();
            params.set("grant_type", "authorization_code");
            params.set("code", code);
            params.set("client_id", app.client_id);
            params.set("client_secret", app.client_secret);
            if (redirectUri) {
                params.set("redirect_uri", redirectUri);
            }
            const tokenUrl = `https://${space}.${app.domain}/api/v2/oauth2/token`;
            return requestToken(tokenUrl, params);
        },
        async refreshToken(domain, space, refreshTokenValue) {
            const app = findApp(domain);
            const params = new URLSearchParams();
            params.set("grant_type", "refresh_token");
            params.set("refresh_token", refreshTokenValue);
            params.set("client_id", app.client_id);
            params.set("client_secret", app.client_secret);
            const tokenUrl = `https://${space}.${app.domain}/api/v2/oauth2/token`;
            return requestToken(tokenUrl, params);
        },
    };
}

/**
 * Create sandbox runner if any MCP tenant enables scripting.
 */
async function getSandbox(
    mcpConfig: McpServerConfig,
): Promise<CreateMcpAppOptions["runScript"]> {
    const hasSandboxEnabled = Object.values(mcpConfig.tenants).some(
        (t) => t.script?.enabled,
    );
    if (!hasSandboxEnabled) {
        return undefined;
    }

    if (!cachedSandbox) {
        cachedSandbox = await createSandboxClient({
            workerPath: process.env[ENV_VARS.SANDBOX_WORKER_PATH],
            binPath: process.env[ENV_VARS.BACKLOG_BIN_PATH],
        });
    }

    return (script, token, tenant, opts) => cachedSandbox!.execute(script, token, tenant, opts?.readOnly);
}

/**
 * Create an AuditLogger that logs to CloudWatch.
 */
function createAWSAuditLogger(): AuditLogger {
    return {
        log(event: AuditEvent): void {
            console.log(JSON.stringify(event));
        },
    };
}

// Load portal assets at module initialization (cold start)
let cachedPortalAssets: PortalAssets | undefined;
function getPortalAssets(): PortalAssets | undefined {
    if (cachedPortalAssets === undefined) {
        cachedPortalAssets = loadPortalAssets();
    }
    return cachedPortalAssets;
}

/**
 * Default Lambda handler.
 */
export const handler = async (event: LambdaEvent, context: LambdaContext) => {
    // CloudFront OAC overwrites Authorization with SigV4.
    // The CloudFront Function copies the original to x-mcp-authorization; restore it here.
    if (!event.headers) event.headers = {};
    const mcpAuth = event.headers["x-mcp-authorization"];
    if (mcpAuth && typeof mcpAuth === "string") {
        const existing = event.headers["authorization"];
        if (!existing || (typeof existing === "string" && !existing.startsWith("Bearer "))) {
            event.headers["authorization"] = mcpAuth;
        }
    }

    const relayConfig = await getRelayConfig();
    const auditLogger = createAWSAuditLogger();
    const portalAssets = getPortalAssets();

    const relayApp = createRelayApp({
        config: relayConfig,
        auditLogger,
        verifyPassphrase,
        createBundle,
        portalAssets,
    });

    const app = new Hono();

    // CORS for MCP browser clients (e.g. MCP Inspector).
    // Applied at the top level because Hono's app.route() does not copy sub-app middleware.
    app.use("*", cors({
        origin: "*",
        allowMethods: ["GET", "POST", "DELETE", "OPTIONS"],
        allowHeaders: ["Content-Type", "Authorization", "Accept", "MCP-Protocol-Version"],
        exposeHeaders: ["WWW-Authenticate"],
    }));

    // Derive base URL from request (x-original-host for CloudFront, host for direct)
    const headers = event.headers ?? {};
    const host = headers["x-original-host"] || headers["host"];
    const eventBaseUrl = host ? `https://${host}` : undefined;

    // Mount MCP endpoints if configured
    const mcpConfig = await buildMcpConfig(relayConfig, eventBaseUrl);
    if (mcpConfig) {
        const tokenExchange = createDirectTokenExchange(relayConfig);
        const runScript = await getSandbox(mcpConfig);

        const mcpApp = createMcpApp({
            config: mcpConfig,
            binPath: process.env[ENV_VARS.BACKLOG_BIN_PATH],
            runScript,
            tokenExchange,
            callbackPath: "/auth/callback",
        });

        // Shared /auth/callback: Backlog OAuth allows only one redirect_uri per app.
        // MCP authorize redirects Backlog to /auth/callback (same as CLI relay).
        // Dispatch by trying MCP state decryption first; fall through to relay on failure.
        const mcpTokenKey = importKey(mcpConfig.token_key);
        const mcpTokenKeyPrev = mcpConfig.token_key_prev
            ? importKey(mcpConfig.token_key_prev)
            : undefined;

        app.get("/auth/callback", async (c) => {
            const state = c.req.query("state");
            if (state) {
                const tryMcp = async (key: Uint8Array): Promise<Response | null> => {
                    try {
                        await decrypt(state, key);
                        const url = new URL(c.req.url);
                        url.pathname = "/mcp/authorize/callback";
                        return await mcpApp.fetch(
                            new Request(url.toString(), c.req.raw),
                        );
                    } catch {
                        return null;
                    }
                };

                const resp =
                    (await tryMcp(mcpTokenKey)) ??
                    (mcpTokenKeyPrev ? await tryMcp(mcpTokenKeyPrev) : null);
                if (resp) return resp;
            }
            return relayApp.fetch(c.req.raw);
        });

        app.route("/", mcpApp);
    }

    app.route("/", relayApp);

    const lambdaHandler = handle(app);
    return lambdaHandler(event, context);
};

// Export utilities for customization
export { getRelayConfig, buildMcpConfig, createAWSAuditLogger, createDirectTokenExchange };
