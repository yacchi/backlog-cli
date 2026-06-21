/**
 * Portal OAuth authentication handlers.
 *
 * Handles Backlog OAuth login for the portal web UI, creating a session
 * cookie upon successful authentication.
 */

import { Hono } from "hono";
import { getCookie, setCookie, deleteCookie } from "hono/cookie";
import type { Context } from "hono";
import type { RelayConfig, AuditLogger, TenantConfig } from "../config/types.js";
import { AuditActions, createAuditEvent } from "../middleware/audit.js";
import { extractRequestContext } from "../utils/request.js";
import { encodePortalState, type PortalStateClaims } from "../utils/state.js";
import { createPortalSessionToken, encryptRefreshToken } from "../utils/portal-session.js";

const NONCE_COOKIE = "portal_nonce";
const SESSION_COOKIE = "portal_session";
const REFRESH_COOKIE = "portal_refresh";
const REFRESH_COOKIE_MAX_AGE = 30 * 24 * 3600; // 30 days

function findTenant(config: RelayConfig, name: string): TenantConfig | undefined {
  return config.tenants?.find((t) => t.name === name);
}

function buildCallbackUrl(c: Context, config: RelayConfig): string {
  if (config.server.base_url) {
    return `${config.server.base_url}/auth/callback`;
  }
  const reqCtx = extractRequestContext(c);
  return `${reqCtx.baseUrl}/auth/callback`;
}

function isSecureContext(c: Context): boolean {
  const reqCtx = extractRequestContext(c);
  return reqCtx.protocol === "https";
}

/**
 * Fetch current user information from Backlog API.
 */
async function fetchCurrentUser(
  spaceHost: string,
  accessToken: string,
): Promise<{ userId: string; name: string; mailAddress: string } | null> {
  try {
    const url = `https://${spaceHost}/api/v2/users/myself`;
    const response = await fetch(url, {
      headers: { Authorization: `Bearer ${accessToken}` },
    });
    if (!response.ok) return null;

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
 * Exchange authorization code for tokens with Backlog.
 */
async function exchangeCode(
  spaceHost: string,
  code: string,
  redirectUri: string,
  clientId: string,
  clientSecret: string,
): Promise<{ access_token: string; refresh_token: string; expires_in: number }> {
  const params = new URLSearchParams();
  params.set("grant_type", "authorization_code");
  params.set("code", code);
  params.set("redirect_uri", redirectUri);
  params.set("client_id", clientId);
  params.set("client_secret", clientSecret);

  const response = await fetch(`https://${spaceHost}/api/v2/oauth2/token`, {
    method: "POST",
    headers: { "Content-Type": "application/x-www-form-urlencoded" },
    body: params.toString(),
  });

  const body = await response.text();
  if (!response.ok) {
    throw new Error(`Token exchange failed: ${body}`);
  }
  return JSON.parse(body);
}

/**
 * Create portal auth start handler.
 */
export function createPortalAuthHandlers(
  config: RelayConfig,
  auditLogger: AuditLogger,
): Hono {
  const app = new Hono();

  /**
   * GET /portal/:name/auth/start - Start portal OAuth flow.
   */
  app.get("/portal/:name/auth/start", (c) => {
    const name = c.req.param("name");
    const reqCtx = extractRequestContext(c);

    const tenant = findTenant(config, name);
    if (!tenant) {
      return c.text("tenant not found", 404);
    }

    const spaceParam = c.req.query("space");
    const space = spaceParam || tenant.default_space;
    if (!space) {
      return c.text("space is required (no default_space configured)", 400);
    }

    const nonce = crypto.randomUUID();
    const state = encodePortalState({
      purpose: "portal",
      tenant: name,
      space,
      nonce,
    });

    const secure = isSecureContext(c);
    setCookie(c, NONCE_COOKIE, nonce, {
      httpOnly: true,
      secure,
      sameSite: "Lax",
      maxAge: 300,
      path: "/",
    });

    auditLogger.log(
      createAuditEvent({
        action: AuditActions.PORTAL_OAUTH_START,
        domain: name,
        space,
        clientIp: reqCtx.clientIp,
        userAgent: reqCtx.userAgent,
        result: "success",
      }),
    );

    const redirectUri = buildCallbackUrl(c, config);
    const authUrl = new URL(`https://${space}/OAuth2AccessRequest.action`);
    authUrl.searchParams.set("response_type", "code");
    authUrl.searchParams.set("client_id", config.backlog_app.client_id);
    authUrl.searchParams.set("redirect_uri", redirectUri);
    authUrl.searchParams.set("state", state);

    c.header("Cache-Control", "no-store, no-cache, must-revalidate, max-age=0");
    c.header("Pragma", "no-cache");
    return c.redirect(authUrl.toString(), 302);
  });

  return app;
}

/**
 * Handle portal OAuth callback.
 *
 * Called from the shared /auth/callback handler when state has purpose="portal".
 */
export async function handlePortalCallback(
  c: Context,
  portalState: PortalStateClaims,
  code: string,
  config: RelayConfig,
  auditLogger: AuditLogger,
): Promise<Response> {
  const reqCtx = extractRequestContext(c);

  // CSRF protection: verify nonce cookie
  const nonceCookie = getCookie(c, NONCE_COOKIE);
  if (!nonceCookie || nonceCookie !== portalState.nonce) {
    auditLogger.log(
      createAuditEvent({
        action: AuditActions.PORTAL_OAUTH_LOGIN,
        domain: portalState.tenant,
        space: portalState.space,
        clientIp: reqCtx.clientIp,
        userAgent: reqCtx.userAgent,
        result: "error",
        error: "nonce mismatch",
      }),
    );
    return c.text("Invalid session. Please try again.", 400);
  }

  const jwksJson = config.jwks;
  if (!jwksJson) {
    return c.text("Server JWKS not configured", 500);
  }

  try {
    // Exchange code for token
    const redirectUri = buildCallbackUrl(c, config);
    const tokenResp = await exchangeCode(
      portalState.space,
      code,
      redirectUri,
      config.backlog_app.client_id,
      config.backlog_app.client_secret,
    );

    // Fetch user info
    const user = await fetchCurrentUser(portalState.space, tokenResp.access_token);
    if (!user) {
      auditLogger.log(
        createAuditEvent({
          action: AuditActions.PORTAL_OAUTH_LOGIN,
          domain: portalState.tenant,
          space: portalState.space,
          clientIp: reqCtx.clientIp,
          userAgent: reqCtx.userAgent,
          result: "error",
          error: "failed to fetch user info",
        }),
      );
      return c.text("Failed to verify user identity", 502);
    }

    // Create session token
    const sessionToken = await createPortalSessionToken(
      { userId: user.userId, name: user.name, email: user.mailAddress },
      portalState.tenant,
      portalState.space,
      jwksJson,
    );

    const secure = isSecureContext(c);

    // Set session cookie
    setCookie(c, SESSION_COOKIE, sessionToken, {
      httpOnly: true,
      secure,
      sameSite: "Lax",
      maxAge: 3600,
      path: "/",
    });

    // Set encrypted refresh token cookie
    const encryptedRefresh = await encryptRefreshToken(
      tokenResp.refresh_token,
      portalState.space,
      portalState.tenant,
      jwksJson,
    );
    setCookie(c, REFRESH_COOKIE, encryptedRefresh, {
      httpOnly: true,
      secure,
      sameSite: "Lax",
      maxAge: REFRESH_COOKIE_MAX_AGE,
      path: "/",
    });

    // Clear nonce cookie
    deleteCookie(c, NONCE_COOKIE, { path: "/" });

    auditLogger.log(
      createAuditEvent({
        action: AuditActions.PORTAL_OAUTH_LOGIN,
        domain: portalState.tenant,
        space: portalState.space,
        userId: user.userId,
        userName: user.name,
        userEmail: user.mailAddress,
        clientIp: reqCtx.clientIp,
        userAgent: reqCtx.userAgent,
        result: "success",
      }),
    );

    const baseUrl = config.server.base_url || reqCtx.baseUrl;
    c.header("Cache-Control", "no-store, no-cache, must-revalidate, max-age=0");
    c.header("Pragma", "no-cache");
    return c.redirect(`${baseUrl}/portal/${portalState.tenant}`, 302);
  } catch (err) {
    auditLogger.log(
      createAuditEvent({
        action: AuditActions.PORTAL_OAUTH_LOGIN,
        domain: portalState.tenant,
        space: portalState.space,
        clientIp: reqCtx.clientIp,
        userAgent: reqCtx.userAgent,
        result: "error",
        error: (err as Error).message,
      }),
    );
    return c.text("Authentication failed. Please try again.", 500);
  }
}

/**
 * Type for the portal callback handler function.
 */
export type PortalCallbackHandler = typeof handlePortalCallback;
