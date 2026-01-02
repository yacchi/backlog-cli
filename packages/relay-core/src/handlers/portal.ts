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
  domain: string;
  passphrase: string;
}

/**
 * Portal verify response.
 */
interface PortalVerifyResponse {
  success: boolean;
  domain?: string;
  relay_url?: string;
  space?: string;
  backlog_domain?: string;
  error?: string;
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
    domain: string,
    relayUrl: string
  ) => Promise<Uint8Array>
): Hono {
  const app = new Hono();

  /**
   * Find tenant by allowed domain.
   */
  function findTenant(domain: string): TenantConfig | undefined {
    return config.tenants?.find((t) => t.allowedDomain === domain);
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
   * Split domain into space and backlog domain.
   * e.g., "myspace.backlog.jp" -> { space: "myspace", backlogDomain: "backlog.jp" }
   */
  function splitDomain(domain: string): {
    space: string;
    backlogDomain: string;
  } {
    const parts = domain.split(".");
    if (parts.length < 3) {
      return { space: "", backlogDomain: domain };
    }
    const space = parts[0];
    const backlogDomain = parts.slice(1).join(".");
    return { space, backlogDomain };
  }

  /**
   * POST /portal/verify - Verify passphrase for tenant access.
   */
  app.post("/portal/verify", async (c) => {
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

    if (!req.domain || !req.passphrase) {
      return c.json(
        {
          success: false,
          error: "domain and passphrase are required",
        } as PortalVerifyResponse,
        400
      );
    }

    const tenant = findTenant(req.domain);
    if (!tenant) {
      auditLogger.log(
        createAuditEvent({
          action: AuditActions.PORTAL_VERIFY,
          domain: req.domain,
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

    if (!tenant.passphraseHash) {
      auditLogger.log(
        createAuditEvent({
          action: AuditActions.PORTAL_VERIFY,
          domain: req.domain,
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

    const valid = await verifyPassphrase(tenant.passphraseHash, req.passphrase);
    if (!valid) {
      auditLogger.log(
        createAuditEvent({
          action: AuditActions.PORTAL_VERIFY,
          domain: req.domain,
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

    const { space, backlogDomain } = splitDomain(req.domain);
    const relayUrl = buildRelayUrl(config.server.baseUrl, reqCtx.baseUrl);

    auditLogger.log(
      createAuditEvent({
        action: AuditActions.PORTAL_VERIFY,
        space,
        domain: backlogDomain,
        clientIp: reqCtx.clientIp,
        userAgent: reqCtx.userAgent,
        result: "success",
      })
    );

    return c.json({
      success: true,
      domain: req.domain,
      relay_url: relayUrl,
      space,
      backlog_domain: backlogDomain,
    } as PortalVerifyResponse);
  });

  /**
   * POST /portal/bundle/:domain - Download configuration bundle.
   */
  app.post("/portal/bundle/:domain", async (c) => {
    const reqCtx = extractRequestContext(c);
    const allowedDomain = c.req.param("domain");

    let req: PortalVerifyRequest;
    try {
      req = await c.req.json();
    } catch {
      return c.json(
        { success: false, error: "invalid_request" } as PortalVerifyResponse,
        400
      );
    }

    if (!allowedDomain || !req.passphrase) {
      return c.json(
        {
          success: false,
          error: "domain and passphrase are required",
        } as PortalVerifyResponse,
        400
      );
    }

    const tenant = findTenant(allowedDomain);
    if (!tenant) {
      auditLogger.log(
        createAuditEvent({
          action: AuditActions.PORTAL_DOWNLOAD,
          domain: allowedDomain,
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

    if (!tenant.passphraseHash) {
      auditLogger.log(
        createAuditEvent({
          action: AuditActions.PORTAL_DOWNLOAD,
          domain: allowedDomain,
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

    const valid = await verifyPassphrase(tenant.passphraseHash, req.passphrase);
    if (!valid) {
      auditLogger.log(
        createAuditEvent({
          action: AuditActions.PORTAL_DOWNLOAD,
          domain: allowedDomain,
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

    const relayUrl = buildRelayUrl(config.server.baseUrl, reqCtx.baseUrl);

    try {
      const bundleData = await createBundle(tenant, allowedDomain, relayUrl);

      const { space, backlogDomain } = splitDomain(allowedDomain);
      auditLogger.log(
        createAuditEvent({
          action: AuditActions.PORTAL_DOWNLOAD,
          space,
          domain: backlogDomain,
          clientIp: reqCtx.clientIp,
          userAgent: reqCtx.userAgent,
          result: "success",
        })
      );

      const filename = `${allowedDomain}.backlog-cli.zip`;
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
          domain: allowedDomain,
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

  return app;
}
