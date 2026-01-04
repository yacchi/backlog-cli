/**
 * @backlog-cli/relay-core
 *
 * Core library for the Backlog CLI OAuth relay server.
 * This package provides platform-agnostic handlers and utilities
 * that can be used with Cloudflare Workers, AWS Lambda, or Docker.
 */

import { Hono } from "hono";
import type { RelayConfig, AuditLogger, TenantConfig } from "./config/types.js";
import { ConsoleAuditLogger } from "./middleware/audit.js";
import { createAuthHandlers } from "./handlers/auth.js";
import { createTokenHandlers } from "./handlers/token.js";
import { createWellKnownHandlers } from "./handlers/wellknown.js";
import { createPortalHandlers, type PortalAssets } from "./handlers/portal.js";
import { createCertsHandlers } from "./handlers/certs.js";
import { createInfoHandlers } from "./handlers/info.js";
import { createBundleHandlers } from "./handlers/bundle.js";

// Re-export types
export type {
  RelayConfig,
  BacklogAppConfig,
  TenantConfig,
  ServerConfig,
  AccessControlConfig,
  RateLimitConfig,
  CacheConfig,
  ConfigProvider,
  CacheProvider,
  AuditEvent,
  AuditLogger,
} from "./config/types.js";

// Re-export config validation
export {
  parseConfig,
  safeParseConfig,
  RelayConfigSchema,
  BacklogAppConfigSchema,
  TenantConfigSchema,
  ServerConfigSchema,
} from "./config/schema.js";

// Re-export utilities
export { encodeState, decodeState, extractSessionId } from "./utils/state.js";
export type { EncodedStateClaims } from "./utils/state.js";
export { extractRequestContext } from "./utils/request.js";
export type { RequestContext } from "./utils/request.js";
export { createBundle } from "./utils/bundle.js";
export { verifyPassphrase } from "./utils/passphrase.js";

// Re-export middleware
export { AccessControl } from "./middleware/access-control.js";
export { RateLimiter } from "./middleware/rate-limit.js";
export {
  ConsoleAuditLogger,
  NoopAuditLogger,
  AuditActions,
  createAuditEvent,
} from "./middleware/audit.js";
export { createBundleAuthMiddleware } from "./middleware/bundle-auth.js";
export type { BundleAuthOptions, BundleAuthTenantConfig } from "./middleware/bundle-auth.js";

// Re-export handlers
export { createAuthHandlers } from "./handlers/auth.js";
export { createTokenHandlers } from "./handlers/token.js";
export { createWellKnownHandlers } from "./handlers/wellknown.js";
export { createPortalHandlers } from "./handlers/portal.js";
export type { PortalAssets } from "./handlers/portal.js";
export { createCertsHandlers } from "./handlers/certs.js";
export { createInfoHandlers } from "./handlers/info.js";
export { createBundleHandlers } from "./handlers/bundle.js";

/**
 * Options for creating the relay app.
 */
export interface CreateRelayAppOptions {
  /** Relay server configuration */
  config: RelayConfig;
  /** Audit logger (defaults to console logger) */
  auditLogger?: AuditLogger;
  /** Passphrase verification function for portal (required for portal features) */
  verifyPassphrase?: (hash: string, passphrase: string) => Promise<boolean>;
  /** Bundle creation function (required for bundle and portal features) */
  createBundle?: (
    tenant: TenantConfig,
    domain: string,
    relayUrl: string
  ) => Promise<Uint8Array>;
  /** Portal SPA assets (required for portal SPA serving) */
  portalAssets?: PortalAssets;
}

/**
 * Create the relay Hono application.
 *
 * This creates a Hono app with all the relay endpoints:
 * - GET /health - Health check
 * - GET /.well-known/backlog-oauth-relay - Server discovery
 * - GET /auth/start - Start OAuth flow
 * - GET /auth/callback - OAuth callback
 * - POST /auth/token - Token exchange/refresh
 * - GET /v1/relay/tenants/:domain/certs - Get public JWKS
 * - GET /v1/relay/tenants/:domain/info - Get signed relay info
 * - GET /v1/relay/tenants/:domain/bundle - Download config bundle (no auth)
 * - POST /portal/verify - Verify portal passphrase (optional)
 * - POST /portal/bundle/:domain - Download config bundle with auth (optional)
 */
export function createRelayApp(options: CreateRelayAppOptions): Hono {
  const { config, auditLogger = new ConsoleAuditLogger() } = options;

  const app = new Hono();

  // Mount well-known handlers
  app.route("/", createWellKnownHandlers(config));

  // Mount auth handlers
  app.route("/", createAuthHandlers(config, auditLogger));

  // Mount token handlers
  app.route("/", createTokenHandlers(config, auditLogger));

  // Mount certs handlers (for JWKS distribution)
  app.route("/", createCertsHandlers(config));

  // Mount info handlers (for signed relay info)
  app.route("/", createInfoHandlers(config));

  // Mount bundle handlers if createBundle function is provided
  if (options.createBundle) {
    app.route(
      "/",
      createBundleHandlers(config, auditLogger, options.createBundle)
    );
  }

  // Mount portal handlers if both functions are provided
  if (options.verifyPassphrase && options.createBundle) {
    app.route(
      "/",
      createPortalHandlers(
        config,
        auditLogger,
        options.verifyPassphrase,
        options.createBundle,
        options.portalAssets
      )
    );
  }

  return app;
}
