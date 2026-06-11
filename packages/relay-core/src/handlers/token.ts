/**
 * Token exchange and refresh handlers.
 */

import { Hono } from "hono";
import type { Context } from "hono";
import type {
  RelayConfig,
  AuditLogger,
} from "../config/types.js";
import { AuditActions, createAuditEvent } from "../middleware/audit.js";
import { extractRequestContext } from "../utils/request.js";
import { extractSessionId } from "../utils/state.js";

/**
 * Token request body.
 */
interface TokenRequest {
  grant_type: string;
  code?: string;
  refresh_token?: string;
  space: string;
  domain?: string;
  state?: string;
}

/**
 * Token response from Backlog.
 */
interface TokenResponse {
  access_token: string;
  token_type: string;
  expires_in: number;
  refresh_token: string;
}

/**
 * Error response.
 */
interface ErrorResponse {
  error: string;
  error_description?: string;
}

/**
 * Normalize space to a full host (e.g., "myspace.backlog.jp").
 * If space already contains a dot, it's the full host.
 * Otherwise, combine with domain if provided.
 */
function normalizeSpace(space: string, domain?: string): string {
  if (space.includes(".")) return space;
  if (domain) return `${space}.${domain}`;
  return space;
}

/**
 * Create token handlers with the given configuration.
 */
export function createTokenHandlers(
  config: RelayConfig,
  auditLogger: AuditLogger
): Hono {
  const app = new Hono();

  /**
   * Build callback URL for OAuth redirect (needed for code exchange).
   */
  function buildCallbackUrl(c: Context): string {
    if (config.server.base_url) {
      return `${config.server.base_url}/auth/callback`;
    }

    const reqCtx = extractRequestContext(c);
    return `${reqCtx.baseUrl}/auth/callback`;
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
   * Request token from Backlog OAuth server.
   */
  async function requestToken(
    spaceHost: string,
    params: URLSearchParams
  ): Promise<TokenResponse> {
    const tokenUrl = `https://${spaceHost}/api/v2/oauth2/token`;

    const response = await fetch(tokenUrl, {
      method: "POST",
      headers: {
        "Content-Type": "application/x-www-form-urlencoded",
      },
      body: params.toString(),
    });

    const body = await response.text();

    if (!response.ok) {
      throw new Error(`Token request failed: ${body}`);
    }

    return JSON.parse(body) as TokenResponse;
  }

  const backlogApp = config.backlog_app;

  /**
   * Exchange authorization code for tokens.
   */
  async function exchangeCode(
    c: Context,
    spaceHost: string,
    code: string
  ): Promise<TokenResponse> {
    const params = new URLSearchParams();
    params.set("grant_type", "authorization_code");
    params.set("code", code);
    params.set("redirect_uri", buildCallbackUrl(c));
    params.set("client_id", backlogApp.client_id);
    params.set("client_secret", backlogApp.client_secret);

    return requestToken(spaceHost, params);
  }

  /**
   * Refresh access token.
   */
  async function refreshToken(
    spaceHost: string,
    refreshTokenValue: string
  ): Promise<TokenResponse> {
    const params = new URLSearchParams();
    params.set("grant_type", "refresh_token");
    params.set("refresh_token", refreshTokenValue);
    params.set("client_id", backlogApp.client_id);
    params.set("client_secret", backlogApp.client_secret);

    return requestToken(spaceHost, params);
  }

  /**
   * Fetch current user information from Backlog API.
   */
  async function fetchCurrentUser(
    spaceHost: string,
    accessToken: string
  ): Promise<{ userId: string; name: string; mailAddress: string } | null> {
    try {
      const url = `https://${spaceHost}/api/v2/users/myself`;
      const response = await fetch(url, {
        headers: {
          Authorization: `Bearer ${accessToken}`,
        },
      });

      if (!response.ok) {
        return null;
      }

      const data = (await response.json()) as {
        userId?: string;
        name?: string;
        mailAddress?: string;
      };
      return {
        userId: data.userId ?? "",
        name: data.name ?? "",
        mailAddress: data.mailAddress ?? "",
      };
    } catch {
      return null;
    }
  }

  /**
   * POST /auth/token - Exchange code or refresh token.
   */
  app.post("/auth/token", async (c) => {
    const reqCtx = extractRequestContext(c);

    let req: TokenRequest;
    try {
      req = await c.req.json();
    } catch {
      return writeError(c, 400, "invalid_request", "Invalid JSON body");
    }

    // Validation - space is required
    if (!req.space) {
      return writeError(
        c,
        400,
        "invalid_request",
        "space is required"
      );
    }

    // Normalize space: if it doesn't contain a dot but domain is provided, combine them
    const spaceHost = normalizeSpace(req.space, req.domain);

    let tokenResp: TokenResponse;
    let auditAction: string;

    try {
      switch (req.grant_type) {
        case "authorization_code":
          auditAction = AuditActions.TOKEN_EXCHANGE;
          if (!req.code) {
            return writeError(
              c,
              400,
              "invalid_request",
              "code is required for authorization_code grant"
            );
          }
          tokenResp = await exchangeCode(c, spaceHost, req.code);
          break;

        case "refresh_token":
          auditAction = AuditActions.TOKEN_REFRESH;
          if (!req.refresh_token) {
            return writeError(
              c,
              400,
              "invalid_request",
              "refresh_token is required for refresh_token grant"
            );
          }
          tokenResp = await refreshToken(
            spaceHost,
            req.refresh_token
          );
          break;

        default:
          return writeError(
            c,
            400,
            "unsupported_grant_type",
            "Supported: authorization_code, refresh_token"
          );
      }
    } catch (err) {
      const sessionId = req.state ? extractSessionId(req.state) : undefined;
      auditLogger.log(
        createAuditEvent({
          sessionId,
          action: auditAction!,
          space: spaceHost,
          clientIp: reqCtx.clientIp,
          userAgent: reqCtx.userAgent,
          result: "error",
          error: (err as Error).message,
        })
      );
      return writeError(c, 502, "upstream_error", (err as Error).message);
    }

    // Fetch user information
    const user = await fetchCurrentUser(
      spaceHost,
      tokenResp.access_token
    );

    // Log success
    const sessionId = req.state ? extractSessionId(req.state) : undefined;
    auditLogger.log(
      createAuditEvent({
        sessionId,
        action: auditAction,
        userId: user?.userId,
        userName: user?.name,
        userEmail: user?.mailAddress,
        space: spaceHost,
        clientIp: reqCtx.clientIp,
        userAgent: reqCtx.userAgent,
        result: "success",
      })
    );

    // Set cache headers
    c.header("Cache-Control", "no-store");
    c.header("Pragma", "no-cache");

    return c.json(tokenResp);
  });

  return app;
}
