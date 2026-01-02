/**
 * Configuration types for the OAuth relay server.
 */

/**
 * Backlog application configuration for a specific domain.
 */
export interface BacklogAppConfig {
  /** The Backlog domain (e.g., "backlog.jp", "backlog.com") */
  domain: string;
  /** OAuth client ID */
  clientId: string;
  /** OAuth client secret */
  clientSecret: string;
}

/**
 * Tenant configuration for multi-tenant support.
 */
export interface TenantConfig {
  /** Allowed domain pattern (e.g., "myspace.backlog.jp") */
  allowedDomain: string;
  /** Optional passphrase hash for portal access (bcrypt) */
  passphraseHash?: string;
}

/**
 * Access control configuration.
 */
export interface AccessControlConfig {
  /** Allowed space patterns (e.g., "myspace;otherspace;*-dev") */
  allowedSpacePatterns?: string;
  /** Allowed project patterns */
  allowedProjectPatterns?: string;
}

/**
 * Rate limiting configuration.
 */
export interface RateLimitConfig {
  /** Requests per minute per IP */
  requestsPerMinute: number;
  /** Burst size */
  burstSize: number;
}

/**
 * Server configuration.
 */
export interface ServerConfig {
  /** Base URL of the relay server (e.g., "https://relay.example.com") */
  baseUrl?: string;
  /** Allowed host patterns for dynamic base URL construction */
  allowedHostPatterns?: string;
  /** Port for local development */
  port: number;
}

/**
 * Cache control configuration.
 */
export interface CacheConfig {
  /** Certificate cache TTL in seconds */
  certsCacheTtl: number;
  /** Info endpoint cache TTL in seconds */
  infoCacheTtl: number;
}

/**
 * Full relay server configuration.
 */
export interface RelayConfig {
  /** Server configuration */
  server: ServerConfig;
  /** Backlog app configurations by domain */
  backlogApps: BacklogAppConfig[];
  /** Tenant configurations for multi-tenant support */
  tenants?: TenantConfig[];
  /** Access control settings */
  accessControl?: AccessControlConfig;
  /** Rate limiting settings */
  rateLimit?: RateLimitConfig;
  /** Cache control settings */
  cache?: CacheConfig;
}

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
