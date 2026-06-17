/**
 * Portal handlers for configuration bundle distribution.
 */

import { Hono } from "hono";
import { getCookie, setCookie } from "hono/cookie";
import type { Context } from "hono";
import type { RelayConfig, AuditLogger, TenantConfig } from "../config/types.js";
import { AuditActions, createAuditEvent } from "../middleware/audit.js";
import { extractRequestContext } from "../utils/request.js";
import { verifyPortalSessionToken } from "../utils/portal-session.js";
import type { IssuedByInfo } from "../utils/bundle.js";

/**
 * Portal verify request.
 */
interface PortalVerifyRequest {
  name: string;
  passphrase: string;
}

/**
 * Portal verify response.
 */
interface PortalVerifyResponse {
  success: boolean;
  name?: string;
  relay_url?: string;
  error?: string;
}

/**
 * SPA assets for portal.
 */
export interface PortalAssets {
  /** HTML content for the SPA index page */
  indexHtml: string;
  /** Static assets by path (e.g., "assets/index-abc123.js" -> content) */
  assets: Map<string, { content: Uint8Array; contentType: string }>;
}

const SESSION_COOKIE = "portal_session";

interface AuthResult {
  method: "oauth" | "bearer" | "passphrase";
  user: IssuedByInfo | null;
}

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

async function authenticatePortalRequest(
  c: Context,
  tenant: TenantConfig,
  jwksJson: string | undefined,
  verifyPassphrase: (hash: string, passphrase: string) => Promise<boolean>,
): Promise<AuthResult | null> {
  // 1. Session Cookie (OAuth)
  if (jwksJson) {
    const sessionCookie = getCookie(c, SESSION_COOKIE);
    if (sessionCookie) {
      try {
        const claims = await verifyPortalSessionToken(sessionCookie, jwksJson);
        if (claims.tenant === tenant.name) {
          return {
            method: "oauth",
            user: { user_id: claims.sub, name: claims.name, email: claims.email },
          };
        }
      } catch {
        // Invalid/expired session — fall through
      }
    }
  }

  // 2. Bearer Token (CLI OAuth)
  const authHeader = c.req.header("Authorization");
  if (authHeader?.startsWith("Bearer ")) {
    const token = authHeader.slice(7);
    let body: Record<string, unknown> = {};
    try { body = await c.req.json(); } catch { /* empty body is ok */ }
    const space = (body.space as string) || tenant.default_space;
    if (space) {
      const user = await fetchCurrentUser(space, token);
      if (user) {
        return {
          method: "bearer",
          user: { user_id: user.userId, name: user.name, email: user.mailAddress },
        };
      }
    }
    return null;
  }

  // 3. Passphrase (existing)
  let body: Record<string, unknown>;
  try { body = await c.req.json(); } catch { return null; }
  const passphrase = body.passphrase as string | undefined;
  if (passphrase && tenant.passphrase_hash) {
    const valid = await verifyPassphrase(tenant.passphrase_hash, passphrase);
    if (valid) {
      return { method: "passphrase", user: null };
    }
  }

  return null;
}

/**
 * Create portal handlers with the given configuration.
 */
export function createPortalHandlers(
  config: RelayConfig,
  auditLogger: AuditLogger,
  verifyPassphrase: (hash: string, passphrase: string) => Promise<boolean>,
  createBundle: (
    tenant: TenantConfig,
    name: string,
    relayUrl: string,
    issuedBy?: IssuedByInfo,
  ) => Promise<Uint8Array>,
  generateProvisionToken: (
    tenant: TenantConfig,
    name: string,
    relayUrl: string,
    issuedBy?: IssuedByInfo,
  ) => Promise<string>,
  portalAssets?: PortalAssets
): Hono {
  const app = new Hono();
  const jwksJson = config.jwks;

  // SPA handler for portal
  if (portalAssets) {
    /**
     * GET /portal/:name - Serve the portal SPA.
     */
    app.get("/portal/:name", (c) => {
      c.header("Content-Type", "text/html; charset=utf-8");
      c.header("Cache-Control", "no-cache");
      return c.body(portalAssets.indexHtml);
    });

    app.get("/portal/:name/", (c) => {
      c.header("Content-Type", "text/html; charset=utf-8");
      c.header("Cache-Control", "no-cache");
      return c.body(portalAssets.indexHtml);
    });

    /**
     * GET /assets/* - Serve static assets.
     */
    app.get("/assets/*", (c) => {
      const path = c.req.path.slice(1); // Remove leading /
      const asset = portalAssets.assets.get(path);
      if (!asset) {
        return c.text("Not Found", 404);
      }
      return new Response(asset.content, {
        headers: {
          "Content-Type": asset.contentType,
          "Cache-Control": "public, max-age=31536000, immutable",
        },
      });
    });
  }

  /**
   * Find tenant by name.
   */
  function findTenant(name: string): TenantConfig | undefined {
    return config.tenants?.find((t) => t.name === name);
  }

  /**
   * Build relay URL from request context.
   */
  function buildRelayUrl(
    baseUrl: string | undefined,
    reqBaseUrl: string
  ): string {
    return baseUrl || reqBaseUrl;
  }

  /**
   * POST /api/v1/portal/verify - Verify passphrase for tenant access.
   */
  app.post("/api/v1/portal/verify", async (c) => {
    const reqCtx = extractRequestContext(c);

    let req: PortalVerifyRequest;
    try {
      req = await c.req.json();
    } catch {
      return c.json(
        { success: false, error: "invalid_request" } as PortalVerifyResponse,
        400
      );
    }

    if (!req.name || !req.passphrase) {
      return c.json(
        {
          success: false,
          error: "domain and passphrase are required",
        } as PortalVerifyResponse,
        400
      );
    }

    const tenant = findTenant(req.name);
    if (!tenant) {
      auditLogger.log(
        createAuditEvent({
          action: AuditActions.PORTAL_VERIFY,
          domain: req.name,
          clientIp: reqCtx.clientIp,
          userAgent: reqCtx.userAgent,
          result: "error",
          error: "tenant not found",
        })
      );
      return c.json(
        { success: false, error: "tenant not found" } as PortalVerifyResponse,
        404
      );
    }

    if (!tenant.passphrase_hash) {
      auditLogger.log(
        createAuditEvent({
          action: AuditActions.PORTAL_VERIFY,
          domain: req.name,
          clientIp: reqCtx.clientIp,
          userAgent: reqCtx.userAgent,
          result: "error",
          error: "portal not enabled",
        })
      );
      return c.json(
        { success: false, error: "portal_not_enabled" } as PortalVerifyResponse,
        403
      );
    }

    const valid = await verifyPassphrase(tenant.passphrase_hash, req.passphrase);
    if (!valid) {
      auditLogger.log(
        createAuditEvent({
          action: AuditActions.PORTAL_VERIFY,
          domain: req.name,
          clientIp: reqCtx.clientIp,
          userAgent: reqCtx.userAgent,
          result: "error",
          error: "invalid passphrase",
        })
      );
      return c.json(
        {
          success: false,
          error: "invalid_passphrase",
        } as PortalVerifyResponse,
        401
      );
    }

    const relayUrl = buildRelayUrl(config.server.base_url, reqCtx.baseUrl);

    auditLogger.log(
      createAuditEvent({
        action: AuditActions.PORTAL_VERIFY,
        domain: req.name,
        clientIp: reqCtx.clientIp,
        userAgent: reqCtx.userAgent,
        result: "success",
      })
    );

    return c.json({
      success: true,
      name: req.name,
      relay_url: relayUrl,
    } as PortalVerifyResponse);
  });

  /**
   * GET /api/v1/portal/:name/info - Get tenant portal info (unauthenticated).
   */
  app.get("/api/v1/portal/:name/info", (c) => {
    const name = c.req.param("name");
    const tenant = findTenant(name);
    if (!tenant) {
      return c.json({ error: "tenant not found" }, 404);
    }
    return c.json({
      name: tenant.name,
      has_passphrase: !!tenant.passphrase_hash,
      oauth_enabled: !!config.backlog_app?.client_id && !!config.jwks,
      ...(tenant.default_space ? { default_space: tenant.default_space } : {}),
    });
  });

  /**
   * GET /api/v1/portal/session - Check current portal session.
   */
  app.get("/api/v1/portal/session", async (c) => {
    if (!jwksJson) {
      return c.json({ authenticated: false });
    }
    const sessionCookie = getCookie(c, SESSION_COOKIE);
    if (!sessionCookie) {
      return c.json({ authenticated: false });
    }
    try {
      const claims = await verifyPortalSessionToken(sessionCookie, jwksJson);
      return c.json({
        authenticated: true,
        user: { id: claims.sub, name: claims.name, email: claims.email },
        tenant: claims.tenant,
      });
    } catch {
      return c.json({ authenticated: false });
    }
  });

  /**
   * DELETE /api/v1/portal/session - Logout (clear session cookie).
   */
  app.delete("/api/v1/portal/session", (c) => {
    const reqCtx = extractRequestContext(c);
    setCookie(c, SESSION_COOKIE, "", { maxAge: 0, path: "/" });
    auditLogger.log(
      createAuditEvent({
        action: AuditActions.PORTAL_LOGOUT,
        clientIp: reqCtx.clientIp,
        userAgent: reqCtx.userAgent,
        result: "success",
      }),
    );
    return c.json({ success: true });
  });

  /**
   * POST /api/v1/portal/:name/bundle - Download configuration bundle.
   * Supports: session cookie, Bearer token, or passphrase.
   */
  app.post("/api/v1/portal/:name/bundle", async (c) => {
    const reqCtx = extractRequestContext(c);
    const name = c.req.param("name");

    const tenant = findTenant(name);
    if (!tenant) {
      auditLogger.log(
        createAuditEvent({
          action: AuditActions.PORTAL_DOWNLOAD,
          domain: name,
          clientIp: reqCtx.clientIp,
          userAgent: reqCtx.userAgent,
          result: "error",
          error: "tenant not found",
        }),
      );
      return c.json({ success: false, error: "tenant not found" } as PortalVerifyResponse, 404);
    }

    const auth = await authenticatePortalRequest(c, tenant, jwksJson, verifyPassphrase);
    if (!auth) {
      auditLogger.log(
        createAuditEvent({
          action: AuditActions.PORTAL_DOWNLOAD,
          domain: name,
          clientIp: reqCtx.clientIp,
          userAgent: reqCtx.userAgent,
          result: "error",
          error: "authentication failed",
        }),
      );
      return c.json({ success: false, error: "authentication_required" } as PortalVerifyResponse, 401);
    }

    const relayUrl = buildRelayUrl(config.server.base_url, reqCtx.baseUrl);

    try {
      const bundleData = await createBundle(tenant, name, relayUrl, auth.user ?? undefined);

      auditLogger.log(
        createAuditEvent({
          action: AuditActions.PORTAL_DOWNLOAD,
          domain: name,
          userId: auth.user?.user_id,
          userName: auth.user?.name,
          userEmail: auth.user?.email,
          clientIp: reqCtx.clientIp,
          userAgent: reqCtx.userAgent,
          result: "success",
        }),
      );

      const filename = `${name}.backlog-cli.zip`;
      return new Response(bundleData, {
        headers: {
          "Content-Type": "application/zip",
          "Content-Disposition": `attachment; filename="${filename}"`,
          "Cache-Control": "no-store",
        },
      });
    } catch (err) {
      auditLogger.log(
        createAuditEvent({
          action: AuditActions.PORTAL_DOWNLOAD,
          domain: name,
          clientIp: reqCtx.clientIp,
          userAgent: reqCtx.userAgent,
          result: "error",
          error: (err as Error).message,
        }),
      );
      return c.json({ success: false, error: "failed to create bundle" } as PortalVerifyResponse, 500);
    }
  });

  /**
   * POST /api/v1/portal/:name/provision - Generate a provisioning key for CLI setup.
   * Supports: session cookie, Bearer token, or passphrase.
   */
  app.post("/api/v1/portal/:name/provision", async (c) => {
    const reqCtx = extractRequestContext(c);
    const name = c.req.param("name");

    const tenant = findTenant(name);
    if (!tenant) {
      auditLogger.log(
        createAuditEvent({
          action: AuditActions.PORTAL_PROVISION,
          domain: name,
          clientIp: reqCtx.clientIp,
          userAgent: reqCtx.userAgent,
          result: "error",
          error: "tenant not found",
        }),
      );
      return c.json({ success: false, error: "tenant not found" } as PortalVerifyResponse, 404);
    }

    const auth = await authenticatePortalRequest(c, tenant, jwksJson, verifyPassphrase);
    if (!auth) {
      auditLogger.log(
        createAuditEvent({
          action: AuditActions.PORTAL_PROVISION,
          domain: name,
          clientIp: reqCtx.clientIp,
          userAgent: reqCtx.userAgent,
          result: "error",
          error: "authentication failed",
        }),
      );
      return c.json({ success: false, error: "authentication_required" } as PortalVerifyResponse, 401);
    }

    const relayUrl = buildRelayUrl(config.server.base_url, reqCtx.baseUrl);

    try {
      const provisioningKey = await generateProvisionToken(
        tenant,
        name,
        relayUrl,
        auth.user ?? undefined,
      );

      auditLogger.log(
        createAuditEvent({
          action: AuditActions.PORTAL_PROVISION,
          domain: name,
          userId: auth.user?.user_id,
          userName: auth.user?.name,
          userEmail: auth.user?.email,
          clientIp: reqCtx.clientIp,
          userAgent: reqCtx.userAgent,
          result: "success",
        }),
      );

      return c.json({
        success: true,
        provisioning_key: provisioningKey,
        ...(tenant.default_space ? { default_space: tenant.default_space } : {}),
      });
    } catch (err) {
      auditLogger.log(
        createAuditEvent({
          action: AuditActions.PORTAL_PROVISION,
          domain: name,
          clientIp: reqCtx.clientIp,
          userAgent: reqCtx.userAgent,
          result: "error",
          error: (err as Error).message,
        }),
      );
      return c.json(
        { success: false, error: "failed to generate provisioning key" } as PortalVerifyResponse,
        500,
      );
    }
  });

  return app;
}
