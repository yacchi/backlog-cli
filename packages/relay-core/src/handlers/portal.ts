/**
 * Portal handlers for configuration bundle distribution.
 */

import { Hono } from "hono";
import type { RelayConfig, AuditLogger, TenantConfig } from "../config/types.js";
import { AuditActions, createAuditEvent } from "../middleware/audit.js";
import { extractRequestContext } from "../utils/request.js";

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
    relayUrl: string
  ) => Promise<Uint8Array>,
  generateProvisionToken: (
    tenant: TenantConfig,
    name: string,
    relayUrl: string
  ) => Promise<string>,
  portalAssets?: PortalAssets
): Hono {
  const app = new Hono();

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
   * POST /api/v1/portal/:name/bundle - Download configuration bundle.
   */
  app.post("/api/v1/portal/:name/bundle", async (c) => {
    const reqCtx = extractRequestContext(c);
    const name = c.req.param("name");

    let req: PortalVerifyRequest;
    try {
      req = await c.req.json();
    } catch {
      return c.json(
        { success: false, error: "invalid_request" } as PortalVerifyResponse,
        400
      );
    }

    if (!name || !req.passphrase) {
      return c.json(
        {
          success: false,
          error: "domain and passphrase are required",
        } as PortalVerifyResponse,
        400
      );
    }

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
          action: AuditActions.PORTAL_DOWNLOAD,
          domain: name,
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
          action: AuditActions.PORTAL_DOWNLOAD,
          domain: name,
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

    try {
      const bundleData = await createBundle(tenant, name, relayUrl);

      auditLogger.log(
        createAuditEvent({
          action: AuditActions.PORTAL_DOWNLOAD,
          domain: name,
          clientIp: reqCtx.clientIp,
          userAgent: reqCtx.userAgent,
          result: "success",
        })
      );

      const filename = `${name}.backlog-cli.zip`;
      c.header("Content-Type", "application/zip");
      c.header("Content-Disposition", `attachment; filename="${filename}"`);
      c.header("Cache-Control", "no-store");

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
        })
      );
      return c.json(
        {
          success: false,
          error: "failed to create bundle",
        } as PortalVerifyResponse,
        500
      );
    }
  });

  /**
   * POST /api/v1/portal/:name/provision - Generate a provisioning key for CLI setup.
   */
  app.post("/api/v1/portal/:name/provision", async (c) => {
    const reqCtx = extractRequestContext(c);
    const name = c.req.param("name");

    let req: PortalVerifyRequest;
    try {
      req = await c.req.json();
    } catch {
      return c.json(
        { success: false, error: "invalid_request" } as PortalVerifyResponse,
        400
      );
    }

    if (!name || !req.passphrase) {
      return c.json(
        {
          success: false,
          error: "domain and passphrase are required",
        } as PortalVerifyResponse,
        400
      );
    }

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
          action: AuditActions.PORTAL_PROVISION,
          domain: name,
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
          action: AuditActions.PORTAL_PROVISION,
          domain: name,
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

    try {
      const provisioningKey = await generateProvisionToken(
        tenant,
        name,
        relayUrl
      );

      auditLogger.log(
        createAuditEvent({
          action: AuditActions.PORTAL_PROVISION,
          domain: name,
          clientIp: reqCtx.clientIp,
          userAgent: reqCtx.userAgent,
          result: "success",
        })
      );

      return c.json({
        success: true,
        provisioning_key: provisioningKey,
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
        })
      );
      return c.json(
        {
          success: false,
          error: "failed to generate provisioning key",
        } as PortalVerifyResponse,
        500
      );
    }
  });

  return app;
}
