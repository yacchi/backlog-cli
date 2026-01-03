/**
 * AWS CDK deployment configuration types.
 *
 * Runtime configuration types are imported from @backlog-cli/relay-core.
 */

import type { RelayConfig as CoreRelayConfig } from "@backlog-cli/relay-core";

// Re-export core types for convenience
export type { BacklogAppConfig, TenantConfig } from "@backlog-cli/relay-core";

/**
 * JWK (JSON Web Key) for CDK configuration.
 * JWKS objects in config are serialized to JSON strings before storing in Parameter Store.
 */
export interface JWK {
  kty: string;
  kid?: string;
  use?: string;
  alg?: string;
  crv?: string;
  x?: string;
  d?: string;
  n?: string;
  e?: string;
  y?: string;
  [key: string]: unknown;
}

/**
 * JWKS (JSON Web Key Set)
 */
export interface JWKS {
  keys: JWK[];
}

/**
 * Tenant configuration for CDK (allows JWKS as object).
 * The JWKS object will be serialized to JSON string before storing in Parameter Store.
 */
export interface TenantConfigWithJwksObject {
  allowed_domain: string;
  passphrase_hash?: string;
  /** JWKS as object (will be serialized to JSON string) */
  jwks?: JWKS;
  active_keys?: string;
  info_ttl?: number;
}

/**
 * CloudFront cache configuration.
 */
export interface CloudFrontCacheConfig {
  /**
   * Static assets (/assets/*) cache TTL in seconds
   * @default 31536000 (365 days)
   */
  assetsMaxAge?: number;

  /**
   * API (certs/info) default TTL in seconds
   * Used when origin doesn't return Cache-Control
   * @default 3600 (1 hour)
   */
  apiDefaultTtl?: number;

  /**
   * API (certs/info) maximum TTL in seconds
   * Caps the origin's max-age
   * @default 86400 (24 hours)
   */
  apiMaxTtl?: number;

  /**
   * API (certs/info) minimum TTL in seconds
   * Minimum cache time to reduce Lambda load
   * @default 300 (5 minutes)
   */
  apiMinTtl?: number;
}

/**
 * CloudFront configuration.
 */
export interface CloudFrontConfig {
  /**
   * Enable CloudFront
   * If true, access via CloudFront (caching enabled)
   */
  enabled: boolean;

  /**
   * Custom domain name (e.g., relay.example.com)
   * If not specified, CloudFront default domain (*.cloudfront.net) is used
   */
  domainName?: string;

  /**
   * ACM certificate ARN (us-east-1 region)
   * Required when domainName is specified
   */
  certificateArn?: string;

  /**
   * Route 53 hosted zone ID (optional)
   * If specified, DNS records are automatically created
   */
  hostedZoneId?: string;

  /**
   * Cache configuration
   * Uses recommended defaults if not specified
   */
  cache?: CloudFrontCacheConfig;
}

/**
 * Parameter Store configuration for CDK deployment.
 */
export interface ParameterStoreConfig {
  /** SSM Parameter Store parameter name */
  parameterName: string;
  /** Configuration value to store (will be JSON serialized) */
  parameterValue?: ParameterStoreValue;
}

/**
 * Parameter Store value format.
 * This extends CoreRelayConfig but allows JWKS as objects (will be serialized).
 */
export interface ParameterStoreValue {
  server: {
    port?: number;
    base_url?: string;
    allowed_host_patterns?: string;
  };
  backlog_apps: Array<{
    domain: string;
    client_id: string;
    client_secret: string;
  }>;
  tenants?: TenantConfigWithJwksObject[];
  access_control?: {
    allowed_space_patterns?: string;
    allowed_project_patterns?: string;
  };
  rate_limit?: {
    requests_per_minute: number;
    burst_size: number;
  };
  cache?: {
    certs_cache_ttl: number;
    info_cache_ttl: number;
  };
}

/**
 * Relay server CDK configuration.
 */
export interface RelayConfig extends ParameterStoreConfig {
  /** CloudFront configuration (optional) */
  cloudFront?: CloudFrontConfig;
}

/**
 * Serialize JWKS objects to JSON strings in tenant configurations.
 */
export function serializeParameterValue(value: ParameterStoreValue): CoreRelayConfig {
  const tenants = value.tenants?.map((tenant) => ({
    ...tenant,
    jwks: tenant.jwks ? JSON.stringify(tenant.jwks) : undefined,
  }));

  return {
    server: value.server,
    backlog_apps: value.backlog_apps,
    tenants,
    access_control: value.access_control,
    rate_limit: value.rate_limit,
    cache: value.cache,
  } as CoreRelayConfig;
}
