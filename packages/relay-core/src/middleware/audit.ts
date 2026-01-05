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
  RELAY_BUNDLE: "relay_bundle",
  BUNDLE_AUTH: "bundle_auth",
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
