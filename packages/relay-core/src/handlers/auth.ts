/**
 * OAuth authentication handlers.
 */

import { Hono } from "hono";
import type { Context } from "hono";
import type {
  RelayConfig,
  AuditLogger,
  BacklogAppConfig,
} from "../config/types.js";
import { AccessControl } from "../middleware/access-control.js";
import { AuditActions, createAuditEvent } from "../middleware/audit.js";
import { extractRequestContext } from "../utils/request.js";
import { encodeState, decodeState, extractSessionId } from "../utils/state.js";

/**
 * Error response type.
 */
interface ErrorResponse {
  error: string;
  error_description?: string;
}

/**
 * Create auth handlers with the given configuration.
 */
export function createAuthHandlers(
  config: RelayConfig,
  auditLogger: AuditLogger
): Hono {
  const app = new Hono();
  const accessControl = new AccessControl(config.accessControl);

  /**
   * Find Backlog app configuration by domain.
   */
  function findBacklogApp(domain: string): BacklogAppConfig | undefined {
    return config.backlogApps.find((app) => app.domain === domain);
  }

  /**
   * Build callback URL for OAuth redirect.
   */
  function buildCallbackUrl(c: Context): string {
    if (config.server.baseUrl) {
      return `${config.server.baseUrl}/auth/callback`;
    }

    const reqCtx = extractRequestContext(c);

    // Validate host if patterns are configured
    if (config.server.allowedHostPatterns) {
      if (!isHostAllowed(reqCtx.host, config.server.allowedHostPatterns)) {
        // Fall back to localhost
        return `http://localhost:${config.server.port}/auth/callback`;
      }
    }

    return `${reqCtx.baseUrl}/auth/callback`;
  }

  /**
   * Check if a host matches the allowed patterns.
   */
  function isHostAllowed(host: string, patterns: string): boolean {
    // Remove port from host
    const hostOnly = host.split(":")[0];

    for (const pattern of patterns.split(";")) {
      const trimmed = pattern.trim();
      if (!trimmed) continue;

      // Convert glob pattern to regex
      const regexPattern = trimmed
        .replace(/[.+^${}()|[\]\\]/g, "\\$&")
        .replace(/\*/g, ".*");

      const regex = new RegExp(`^${regexPattern}$`);
      if (regex.test(hostOnly)) {
        return true;
      }
    }

    return false;
  }

  /**
   * Write JSON error response.
   */
  function writeError(
    c: Context,
    status: number,
    error: string,
    description?: string
  ): Response {
    const response: ErrorResponse = {
      error,
      error_description: description,
    };
    return c.json(response, status as 400);
  }

  /**
   * Render HTML error page.
   */
  function renderErrorPage(
    c: Context,
    title: string,
    message: string
  ): Response {
    return c.html(
      `<!DOCTYPE html>
<html>
<head><title>${title}</title></head>
<body>
<h1>${title}</h1>
<p>${message}</p>
<p>You can close this window.</p>
</body>
</html>`,
      400
    );
  }

  /**
   * GET /auth/start - Start OAuth authorization flow.
   */
  app.get("/auth/start", async (c) => {
    const domain = c.req.query("domain");
    const space = c.req.query("space");
    const portStr = c.req.query("port");
    const cliState = c.req.query("state");
    const project = c.req.query("project");

    const reqCtx = extractRequestContext(c);

    // Validation
    if (!domain || !space || !portStr || !cliState) {
      return writeError(
        c,
        400,
        "invalid_request",
        "domain, space, port, and state are required"
      );
    }

    const port = parseInt(portStr, 10);
    if (isNaN(port) || port < 1024 || port > 65535) {
      return writeError(
        c,
        400,
        "invalid_request",
        "port must be between 1024 and 65535"
      );
    }

    // Access control checks
    try {
      accessControl.checkSpace(space);
    } catch (err) {
      auditLogger.log(
        createAuditEvent({
          action: AuditActions.ACCESS_DENIED,
          space,
          domain,
          project,
          clientIp: reqCtx.clientIp,
          userAgent: reqCtx.userAgent,
          result: "error",
          error: (err as Error).message,
        })
      );
      return writeError(c, 403, "access_denied", (err as Error).message);
    }

    try {
      accessControl.checkProject(project);
    } catch (err) {
      auditLogger.log(
        createAuditEvent({
          action: AuditActions.ACCESS_DENIED,
          space,
          domain,
          project,
          clientIp: reqCtx.clientIp,
          userAgent: reqCtx.userAgent,
          result: "error",
          error: (err as Error).message,
        })
      );
      return writeError(c, 403, "access_denied", (err as Error).message);
    }

    // Find Backlog app config
    const backlogApp = findBacklogApp(domain);
    if (!backlogApp) {
      return writeError(
        c,
        400,
        "invalid_request",
        `domain '${domain}' is not supported`
      );
    }

    // Encode state
    const encodedState = encodeState({
      port,
      cliState,
      space,
      domain,
      project,
    });

    // Log audit event
    auditLogger.log(
      createAuditEvent({
        sessionId: extractSessionId(cliState),
        action: AuditActions.AUTH_START,
        space,
        domain,
        project,
        clientIp: reqCtx.clientIp,
        userAgent: reqCtx.userAgent,
        result: "success",
      })
    );

    // Build Backlog authorization URL
    const redirectUri = buildCallbackUrl(c);
    const authUrl = new URL(
      `https://${space}.${domain}/OAuth2AccessRequest.action`
    );
    authUrl.searchParams.set("response_type", "code");
    authUrl.searchParams.set("client_id", backlogApp.clientId);
    authUrl.searchParams.set("redirect_uri", redirectUri);
    authUrl.searchParams.set("state", encodedState);

    return c.redirect(authUrl.toString(), 302);
  });

  /**
   * GET /auth/callback - Handle OAuth callback from Backlog.
   */
  app.get("/auth/callback", async (c) => {
    const code = c.req.query("code");
    const encodedState = c.req.query("state");
    const errorParam = c.req.query("error");

    const reqCtx = extractRequestContext(c);

    // Handle error from Backlog
    if (errorParam) {
      const errorDesc = c.req.query("error_description") || "";
      auditLogger.log(
        createAuditEvent({
          action: AuditActions.AUTH_CALLBACK,
          clientIp: reqCtx.clientIp,
          userAgent: reqCtx.userAgent,
          result: "error",
          error: `${errorParam}: ${errorDesc}`,
        })
      );
      return renderErrorPage(c, "Authorization Failed", errorDesc);
    }

    if (!code || !encodedState) {
      auditLogger.log(
        createAuditEvent({
          action: AuditActions.AUTH_CALLBACK,
          clientIp: reqCtx.clientIp,
          userAgent: reqCtx.userAgent,
          result: "error",
          error: "missing code or state parameter",
        })
      );
      return renderErrorPage(
        c,
        "Invalid Request",
        "Missing code or state parameter"
      );
    }

    // Decode state
    let claims;
    try {
      claims = decodeState(encodedState);
    } catch (err) {
      auditLogger.log(
        createAuditEvent({
          action: AuditActions.AUTH_CALLBACK,
          clientIp: reqCtx.clientIp,
          userAgent: reqCtx.userAgent,
          result: "error",
          error: `invalid state: ${(err as Error).message}`,
        })
      );
      return renderErrorPage(
        c,
        "Session Invalid",
        "Please try logging in again"
      );
    }

    // Log success
    auditLogger.log(
      createAuditEvent({
        sessionId: extractSessionId(claims.cliState),
        action: AuditActions.AUTH_CALLBACK,
        space: claims.space,
        domain: claims.domain,
        project: claims.project,
        clientIp: reqCtx.clientIp,
        userAgent: reqCtx.userAgent,
        result: "success",
      })
    );

    // Redirect to CLI local server
    const localUrl = new URL(`http://localhost:${claims.port}/callback`);
    localUrl.searchParams.set("code", code);
    localUrl.searchParams.set("state", claims.cliState);

    return c.redirect(localUrl.toString(), 302);
  });

  return app;
}
