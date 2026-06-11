/**
 * AWS Lambda adapter for Backlog OAuth Relay Server.
 * Optionally serves MCP endpoints when MCP integration is configured.
 */

import { Hono } from "hono";
import { handle, type LambdaEvent, type LambdaContext } from "hono/aws-lambda";
import {
    createRelayApp,
    createBundle,
    generateProvisioningToken,
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
    verify,
    loadSigningKeys,
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
    BACKLOG_BIN_PATH: "BACKLOG_BIN_PATH",
    SANDBOX_WORKER_PATH: "SANDBOX_WORKER_PATH",
} as const;

interface RelaySecrets {
    app?: { client_secret: string };
    server?: { jwks?: string };
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
 * Unknown keys like mcp_spaces are stripped by Zod.
 */
async function getRelayConfig(): Promise<RelayConfig> {
    if (cachedRelayConfig) {
        return cachedRelayConfig;
    }

    const raw = await loadRawConfig();

    const secretName = process.env[ENV_VARS.RELAY_SECRETS_NAME];
    if (secretName) {
        const secrets = await loadRelaySecrets(secretName);

        if (raw.backlog_app && secrets.app) {
            raw.backlog_app = {
                ...(raw.backlog_app as Record<string, unknown>),
                client_secret: secrets.app.client_secret,
            };
        }

        // Server-level JWKS: prefer server.jwks, fallback to first tenant's jwks (migration)
        const serverJwks = secrets.server?.jwks
            ?? Object.values(secrets.tenants ?? {}).find((t) => t.jwks)?.jwks;
        if (serverJwks) {
            raw.jwks = serverJwks;
        }

        // Merge per-tenant passphrase_hash from secrets
        if (Array.isArray(raw.tenants) && secrets.tenants) {
            raw.tenants = (raw.tenants as Array<Record<string, unknown>>).map((t) => ({
                ...t,
                passphrase_hash: secrets.tenants[t.name as string]?.passphrase_hash
                    ?? t.passphrase_hash,
            }));
        }
    }

    cachedRelayConfig = parseConfig(JSON.stringify(raw));
    return cachedRelayConfig;
}

/**
 * Build McpServerConfig from raw SSM config + relay secrets (JWKS).
 * MCP uses JWS (Ed25519) with the same JWKS as relay — no separate token key needed.
 */
async function buildMcpConfig(
    relayConfig: RelayConfig,
    eventBaseUrl?: string,
): Promise<McpServerConfig | null> {
    const raw = await loadRawConfig();
    const mcpSpaces = raw.mcp_spaces as Array<{ pattern: string; writable: boolean }> | undefined;
    if (!mcpSpaces || mcpSpaces.length === 0) {
        return null;
    }

    const baseUrl = relayConfig.server.base_url || eventBaseUrl;
    if (!baseUrl) {
        console.warn("MCP integration requires server.base_url in relay config");
        return null;
    }

    // Get JWKS from relay secrets (server-level)
    const secretName = process.env[ENV_VARS.RELAY_SECRETS_NAME];
    if (!secretName) {
        console.warn("MCP integration requires RELAY_SECRETS_NAME");
        return null;
    }
    const secrets = await loadRelaySecrets(secretName);
    const jwksJson = secrets.server?.jwks
        ?? Object.values(secrets.tenants ?? {}).find((t) => t.jwks)?.jwks;
    if (!jwksJson) {
        console.warn("MCP integration requires server.jwks in relay secrets");
        return null;
    }

    const mcpConfigObj: Record<string, unknown> = {
        base_url: baseUrl,
        jwks: jwksJson,
        backlog_app: {
            client_id: relayConfig.backlog_app.client_id,
        },
        spaces: mcpSpaces,
        script: raw.mcp_script,
        default_spaces: raw.mcp_default_spaces ?? [],
    };

    return parseMcpConfig(JSON.stringify(mcpConfigObj));
}

/**
 * Create an in-process TokenExchange using the relay's BacklogAppConfig
 * (which includes client_secret). No HTTP round-trip needed.
 */
function createDirectTokenExchange(relayConfig: RelayConfig): TokenExchange {
    const app = relayConfig.backlog_app;

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
            const params = new URLSearchParams();
            params.set("grant_type", "authorization_code");
            params.set("code", code);
            params.set("client_id", app.client_id);
            params.set("client_secret", app.client_secret);
            if (redirectUri) {
                params.set("redirect_uri", redirectUri);
            }
            const tokenUrl = `https://${space}.${domain}/api/v2/oauth2/token`;
            return requestToken(tokenUrl, params);
        },
        async refreshToken(domain, space, refreshTokenValue) {
            const params = new URLSearchParams();
            params.set("grant_type", "refresh_token");
            params.set("refresh_token", refreshTokenValue);
            params.set("client_id", app.client_id);
            params.set("client_secret", app.client_secret);
            const tokenUrl = `https://${space}.${domain}/api/v2/oauth2/token`;
            return requestToken(tokenUrl, params);
        },
    };
}

/**
 * Create sandbox runner if MCP has spaces configured.
 */
async function getSandbox(
    mcpConfig: McpServerConfig,
): Promise<CreateMcpAppOptions["runScript"]> {
    if (mcpConfig.spaces.length === 0) {
        return undefined;
    }

    if (!cachedSandbox) {
        cachedSandbox = await createSandboxClient({
            workerPath: process.env[ENV_VARS.SANDBOX_WORKER_PATH],
            binPath: process.env[ENV_VARS.BACKLOG_BIN_PATH],
        });
    }

    return (script, token, scriptConfig, opts) => cachedSandbox!.execute(script, token, scriptConfig, opts?.readOnly);
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

    const serverJwks = relayConfig.jwks;
    const relayApp = createRelayApp({
        config: relayConfig,
        auditLogger,
        verifyPassphrase,
        createBundle: (tenant, domain, relayUrl) => createBundle(tenant, domain, relayUrl, serverJwks),
        generateProvisionToken: (tenant, domain, relayUrl) => generateProvisioningToken(tenant, domain, relayUrl, serverJwks),
        portalAssets,
    });

    const app = new Hono();

    // CORS for MCP browser clients (e.g. MCP Inspector).
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

        const mcpApp = await createMcpApp({
            config: mcpConfig,
            binPath: process.env[ENV_VARS.BACKLOG_BIN_PATH],
            runScript,
            tokenExchange,
            callbackPath: "/auth/callback",
        });

        // Shared /auth/callback: Backlog OAuth allows only one redirect_uri per app.
        // Dispatch by trying MCP JWT verification; fall through to relay on failure.
        const mcpKeys = await loadSigningKeys(mcpConfig.jwks);

        app.get("/auth/callback", async (c) => {
            const state = c.req.query("state");
            if (state) {
                try {
                    await verify(state, mcpKeys.verifyKeys);
                    const url = new URL(c.req.url);
                    url.pathname = "/mcp/authorize/callback";
                    return await mcpApp.fetch(
                        new Request(url.toString(), c.req.raw),
                    );
                } catch {
                    // Not an MCP state — fall through to relay
                }
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
