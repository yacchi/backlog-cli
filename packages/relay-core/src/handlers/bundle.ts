/**
 * Bundle handlers for configuration bundle distribution (no auth).
 */

import { Hono } from "hono";
import type { RelayConfig, AuditLogger, TenantConfig } from "../config/types.js";
import { AuditActions, createAuditEvent } from "../middleware/audit.js";
import { extractRequestContext } from "../utils/request.js";

/**
 * Find tenant by name.
 */
function findTenant(
  tenants: TenantConfig[] | undefined,
  name: string
): TenantConfig | undefined {
  return tenants?.find(
    (t) => t.name.toLowerCase() === name.toLowerCase()
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
    name: string,
    relayUrl: string
  ) => Promise<Uint8Array>
): Hono {
  const app = new Hono();

  /**
   * GET /v1/relay/tenants/:name/bundle - Download configuration bundle.
   * This endpoint does not require authentication.
   */
  app.get("/v1/relay/tenants/:name/bundle", async (c) => {
    const reqCtx = extractRequestContext(c);
    const name = c.req.param("name")?.trim();

    if (!name) {
      return c.text("name is required", 400);
    }

    const tenant = findTenant(config.tenants, name);
    if (!tenant) {
      auditLogger.log(
        createAuditEvent({
          action: AuditActions.RELAY_BUNDLE,
          domain: name,
          clientIp: reqCtx.clientIp,
          userAgent: reqCtx.userAgent,
          result: "error",
          error: "tenant not found",
        })
      );
      return c.text("tenant not found", 404);
    }

    const relayUrl = buildRelayUrl(config.server.base_url, reqCtx.baseUrl);

    try {
      const bundleData = await createBundle(tenant, name, relayUrl);

      auditLogger.log(
        createAuditEvent({
          action: AuditActions.RELAY_BUNDLE,
          domain: name,
          clientIp: reqCtx.clientIp,
          userAgent: reqCtx.userAgent,
          result: "success",
        })
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
          action: AuditActions.RELAY_BUNDLE,
          domain: name,
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
