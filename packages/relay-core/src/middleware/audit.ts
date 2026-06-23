/**
 * Audit logging middleware.
 */

import type { AuditEvent, AuditLogger } from "../config/types.js";

/** Audit action constants */
export const AuditActions = {
  AUTH_START: "auth_start",
  AUTH_CALLBACK: "auth_callback",
  TOKEN_EXCHANGE: "token_exchange",
  TOKEN_REFRESH: "token_refresh",
  ACCESS_DENIED: "access_denied",
  PORTAL_VERIFY: "portal_verify",
  PORTAL_DOWNLOAD: "portal_download",
  PORTAL_PROVISION: "portal_provision",
  PORTAL_OAUTH_START: "portal_oauth_start",
  PORTAL_OAUTH_LOGIN: "portal_oauth_login",
  PORTAL_LOGOUT: "portal_logout",
  RELAY_BUNDLE: "relay_bundle",
  BUNDLE_AUTH: "bundle_auth",
  ADMIN_AUDIT_QUERY: "admin_audit_query",
  ADMIN_PASSPHRASE_VIEW: "admin_passphrase_view",
  ADMIN_PASSPHRASE_SET: "admin_passphrase_set",
  ADMIN_PASSPHRASE_GENERATE: "admin_passphrase_generate",
  ADMIN_PASSPHRASE_CLEAR: "admin_passphrase_clear",
} as const;

/**
 * Default console-based audit logger.
 */
export class ConsoleAuditLogger implements AuditLogger {
  log(event: AuditEvent): void {
    const logData = {
      ...event,
      timestamp: event.timestamp.toISOString(),
    };
    console.log(JSON.stringify(logData));
  }
}

/**
 * No-op audit logger for testing.
 */
export class NoopAuditLogger implements AuditLogger {
  log(_event: AuditEvent): void {
    // Do nothing
  }
}

/**
 * Create an audit event with defaults.
 */
export function createAuditEvent(
  partial: Omit<AuditEvent, "timestamp">
): AuditEvent {
  return {
    ...partial,
    timestamp: new Date(),
  };
}
