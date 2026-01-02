/**
 * Bundle handlers for configuration bundle distribution (no auth).
 */

import { Hono } from "hono";
import type { RelayConfig, AuditLogger, TenantConfig } from "../config/types.js";
import { AuditActions, createAuditEvent } from "../middleware/audit.js";
import { extractRequestContext } from "../utils/request.js";

/**
 * Split domain into space and backlog domain.
 */
function splitDomain(domain: string): { space: string; backlogDomain: string } {
  const parts = domain.split(".");
  if (parts.length < 3) {
    return { space: "", backlogDomain: domain };
  }
  const space = parts[0];
  const backlogDomain = parts.slice(1).join(".");
  return { space, backlogDomain };
}

/**
 * Find tenant by allowed domain.
 */
function findTenant(
  tenants: TenantConfig[] | undefined,
  domain: string
): TenantConfig | undefined {
  return tenants?.find(
    (t) => t.allowedDomain.toLowerCase() === domain.toLowerCase()
  );
}

/**
 * Build relay URL from config and request context.
 */
function buildRelayUrl(
  baseUrl: string | undefined,
  reqBaseUrl: string
): string {
  return baseUrl || reqBaseUrl;
}

/**
 * Create bundle handlers (no authentication required).
 */
export function createBundleHandlers(
  config: RelayConfig,
  auditLogger: AuditLogger,
  createBundle: (
    tenant: TenantConfig,
    domain: string,
    relayUrl: string
  ) => Promise<Uint8Array>
): Hono {
  const app = new Hono();

  /**
   * GET /relay/bundle/:domain - Download configuration bundle.
   * This endpoint does not require authentication.
   */
  app.get("/relay/bundle/:domain", async (c) => {
    const reqCtx = extractRequestContext(c);
    const allowedDomain = c.req.param("domain")?.trim();

    if (!allowedDomain) {
      return c.text("domain is required", 400);
    }

    const tenant = findTenant(config.tenants, allowedDomain);
    if (!tenant) {
      auditLogger.log(
        createAuditEvent({
          action: AuditActions.RELAY_BUNDLE,
          domain: allowedDomain,
          clientIp: reqCtx.clientIp,
          userAgent: reqCtx.userAgent,
          result: "error",
          error: "tenant not found",
        })
      );
      return c.text("tenant not found", 404);
    }

    const relayUrl = buildRelayUrl(config.server.baseUrl, reqCtx.baseUrl);

    try {
      const bundleData = await createBundle(tenant, allowedDomain, relayUrl);

      const { space, backlogDomain } = splitDomain(allowedDomain);
      auditLogger.log(
        createAuditEvent({
          action: AuditActions.RELAY_BUNDLE,
          space,
          domain: backlogDomain,
          clientIp: reqCtx.clientIp,
          userAgent: reqCtx.userAgent,
          result: "success",
        })
      );

      const filename = `${allowedDomain}.backlog-cli.zip`;
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
          action: AuditActions.RELAY_BUNDLE,
          domain: allowedDomain,
          clientIp: reqCtx.clientIp,
          userAgent: reqCtx.userAgent,
          result: "error",
          error: (err as Error).message,
        })
      );
      return c.text("failed to create bundle", 500);
    }
  });

  return app;
}
