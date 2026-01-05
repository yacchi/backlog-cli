/**
 * Configuration types for the OAuth relay server.
 *
 * Configuration types are inferred from Zod schemas in schema.ts.
 * This file re-exports them and defines platform abstraction interfaces.
 */

// Re-export configuration types from schema (inferred from Zod)
export type {
  RelayConfig,
  RelayConfigInput,
  ServerConfig,
  ServerConfigInput,
  BacklogAppConfig,
  TenantConfig,
  AccessControlConfig,
  RateLimitConfig,
  CacheConfig,
} from "./schema.js";

import type { RelayConfig } from "./schema.js";

/**
 * Platform abstraction for configuration providers.
 */
export interface ConfigProvider {
  /** Get a configuration value by key */
  get(key: string): Promise<string | undefined>;
  /** Get all configuration */
  getConfig(): Promise<RelayConfig>;
}

/**
 * Platform abstraction for cache providers.
 */
export interface CacheProvider {
  /** Get a cached value */
  get(key: string): Promise<string | undefined>;
  /** Set a cached value with optional TTL in seconds */
  set(key: string, value: string, ttl?: number): Promise<void>;
  /** Delete a cached value */
  delete(key: string): Promise<void>;
}

/**
 * Audit event for logging.
 */
export interface AuditEvent {
  /** Unique session ID for correlation */
  sessionId?: string;
  /** Action being performed */
  action: string;
  /** Backlog space name */
  space?: string;
  /** Backlog domain */
  domain?: string;
  /** Project key */
  project?: string;
  /** User ID (after successful token exchange) */
  userId?: string;
  /** User name */
  userName?: string;
  /** User email */
  userEmail?: string;
  /** Client IP address */
  clientIp?: string;
  /** User agent string */
  userAgent?: string;
  /** Result of the action */
  result: "success" | "error";
  /** Error message if result is "error" */
  error?: string;
  /** Timestamp */
  timestamp: Date;
}

/**
 * Platform abstraction for audit logging.
 */
export interface AuditLogger {
  /** Log an audit event */
  log(event: AuditEvent): void;
}
