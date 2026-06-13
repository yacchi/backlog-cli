/**
 * Platform-agnostic unified application builder.
 *
 * Mounts the relay app (relay-core) and, when MCP spaces are configured,
 * the MCP server app (mcp-server) into a single Hono app — including the
 * shared `/auth/callback` dispatcher required because Backlog OAuth allows
 * only one redirect_uri per app.
 *
 * This logic was originally embedded in the AWS Lambda handler; it is extracted
 * here so the same runtime serves Docker, local, and Lambda-container targets.
 */

import { Hono } from "hono";
import { cors } from "hono/cors";
import {
  createRelayApp,
  createBundle,
  generateProvisioningToken,
  verifyPassphrase,
  parseConfig,
  type RelayConfig,
  type AuditLogger,
  type PortalAssets,
} from "@backlog-cli/relay-core";
import {
  createMcpApp,
  verify,
  loadSigningKeys,
  parseConfig as parseMcpConfig,
  type McpServerConfig,
  type CreateMcpAppOptions,
  type TokenExchange,
} from "@backlog-cli/mcp-server";

/**
 * Restore the `Authorization` header from `x-mcp-authorization`.
 *
 * Behind CloudFront with Origin Access Control, the OAC overwrites the viewer's
 * `Authorization` header with its SigV4 signature; a CloudFront Function copies
 * the original Bearer token to `x-mcp-authorization`. This restores it so MCP
 * Bearer auth works. No-op when `x-mcp-authorization` is absent (i.e. harmless
 * for plain Docker / non-CloudFront deployments).
 */
export function restoreMcpAuthorization(req: Request): Request {
  const mcpAuth = req.headers.get("x-mcp-authorization");
  if (!mcpAuth) {
    return req;
  }
  const existing = req.headers.get("authorization");
  if (existing && existing.startsWith("Bearer ")) {
    return req;
  }
  const headers = new Headers(req.headers);
  headers.set("authorization", mcpAuth);
  return new Request(req, { headers });
}

/**
 * Options for {@link createUnifiedApp}.
 */
export interface CreateUnifiedAppOptions {
  /** Raw config object (secrets already merged by the ConfigSource). */
  rawConfig: Record<string, unknown>;
  /** Audit logger. */
  auditLogger: AuditLogger;
  /** Portal SPA assets (optional). */
  portalAssets?: PortalAssets;
  /** Backlog CLI binary path for the MCP `backlog` tool. */
  binPath?: string;
  /**
   * Factory that produces a `runScript` implementation for the MCP sandbox.
   * Called only when MCP is enabled. If omitted, `run_script` is disabled.
   */
  createRunScript?: (
    mcpConfig: McpServerConfig,
  ) => Promise<CreateMcpAppOptions["runScript"]>;
  /**
   * Base URL to use when the relay config has no `server.base_url`
   * (e.g. derived from the incoming request in a serverless adapter).
   */
  baseUrlFallback?: string;
}

/**
 * Build the MCP server config from the raw config's `mcp_*` keys.
 *
 * MCP reuses the relay's server-level JWKS and Backlog client_id; it uses the
 * same signing keys as the relay (no separate token key). Returns null when MCP
 * is not configured or prerequisites are missing.
 */
export function buildMcpConfig(
  rawConfig: Record<string, unknown>,
  relayConfig: RelayConfig,
  baseUrlFallback?: string,
): McpServerConfig | null {
  const mcpSpaces = rawConfig.mcp_spaces as
    | Array<{ pattern: string; writable: boolean }>
    | undefined;
  if (!mcpSpaces || mcpSpaces.length === 0) {
    return null;
  }

  const jwksJson = relayConfig.jwks;
  if (!jwksJson) {
    console.warn("MCP integration requires a server-level jwks in relay config");
    return null;
  }

  // base_url is optional: the OAuth issuer is derived per-request from the host
  // (resolveBaseUrl) unless explicitly configured. Only the Function URL /
  // CloudFront domain are not known at deploy without a circular dependency, so
  // we must not require base_url here.
  const baseUrl = relayConfig.server.base_url || baseUrlFallback;

  const mcpConfigObj: Record<string, unknown> = {
    jwks: jwksJson,
    backlog_app: {
      client_id: relayConfig.backlog_app.client_id,
    },
    spaces: mcpSpaces,
    script: rawConfig.mcp_script,
    default_spaces: rawConfig.mcp_default_spaces ?? [],
  };
  if (baseUrl) {
    mcpConfigObj.base_url = baseUrl;
  }

  return parseMcpConfig(JSON.stringify(mcpConfigObj));
}

/**
 * Create an in-process {@link TokenExchange} using the relay's BacklogAppConfig
 * (which includes client_secret). No HTTP round-trip to the relay needed.
 */
export function createDirectTokenExchange(
  relayConfig: RelayConfig,
): TokenExchange {
  const app = relayConfig.backlog_app;

  async function requestToken(
    tokenUrl: string,
    params: URLSearchParams,
  ): Promise<{
    access_token: string;
    token_type: string;
    expires_in: number;
    refresh_token: string;
  }> {
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
    // `space` is the full Backlog host (e.g. "myspace.backlog.com") per the
    // spaceHost migration; do not split it into space/domain.
    async exchangeCode(space, code, redirectUri) {
      const params = new URLSearchParams();
      params.set("grant_type", "authorization_code");
      params.set("code", code);
      params.set("client_id", app.client_id);
      params.set("client_secret", app.client_secret);
      if (redirectUri) {
        params.set("redirect_uri", redirectUri);
      }
      return requestToken(`https://${space}/api/v2/oauth2/token`, params);
    },
    async refreshToken(space, refreshTokenValue) {
      const params = new URLSearchParams();
      params.set("grant_type", "refresh_token");
      params.set("refresh_token", refreshTokenValue);
      params.set("client_id", app.client_id);
      params.set("client_secret", app.client_secret);
      return requestToken(`https://${space}/api/v2/oauth2/token`, params);
    },
  };
}

/**
 * Create the unified Hono application (relay + optional MCP).
 */
export async function createUnifiedApp(
  options: CreateUnifiedAppOptions,
): Promise<Hono> {
  const { rawConfig, auditLogger } = options;

  const relayConfig = parseConfig(JSON.stringify(rawConfig));
  const serverJwks = relayConfig.jwks;

  const relayApp = createRelayApp({
    config: relayConfig,
    auditLogger,
    verifyPassphrase,
    createBundle: (tenant, domain, relayUrl) =>
      createBundle(tenant, domain, relayUrl, serverJwks),
    generateProvisionToken: (tenant, domain, relayUrl) =>
      generateProvisioningToken(tenant, domain, relayUrl, serverJwks),
    portalAssets: options.portalAssets,
  });

  const app = new Hono();

  // CORS for MCP browser clients (e.g. MCP Inspector). Harmless for relay
  // endpoints, which are cookie-based and same-origin.
  app.use(
    "*",
    cors({
      origin: "*",
      allowMethods: ["GET", "POST", "DELETE", "OPTIONS"],
      allowHeaders: [
        "Content-Type",
        "Authorization",
        "Accept",
        "MCP-Protocol-Version",
      ],
      exposeHeaders: ["WWW-Authenticate"],
    }),
  );

  const mcpConfig = buildMcpConfig(
    rawConfig,
    relayConfig,
    options.baseUrlFallback,
  );

  if (mcpConfig) {
    const tokenExchange = createDirectTokenExchange(relayConfig);
    const runScript = options.createRunScript
      ? await options.createRunScript(mcpConfig)
      : undefined;

    const mcpApp = await createMcpApp({
      config: mcpConfig,
      binPath: options.binPath,
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
          return await mcpApp.fetch(new Request(url.toString(), c.req.raw));
        } catch {
          // Not an MCP state — fall through to relay.
        }
      }
      return relayApp.fetch(c.req.raw);
    });

    app.route("/", mcpApp);
  }

  app.route("/", relayApp);

  return app;
}
