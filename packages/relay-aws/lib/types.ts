/**
 * AWS CDK deployment configuration types.
 *
 * Runtime configuration types are imported from @backlog-cli/relay-core.
 */

import type { RelayConfigInput as CoreRelayConfigInput } from "@backlog-cli/relay-core";

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
 * Relay bundle signing configuration for a tenant.
 */
export interface RelayTenantInput {
  jwks?: JWKS;
  active_keys?: string;
  info_ttl?: number;
  /** Plaintext passphrase — auto-hashed with bcrypt at CDK synth time */
  passphrase?: string;
  /** Pre-computed bcrypt hash (mutually exclusive with passphrase) */
  passphrase_hash?: string;
  /** Auto-generated passphrase length in characters (default: 32) */
  passphrase_length?: number;
}

/**
 * MCP access control configuration for a tenant.
 */
export interface McpTenantConfig {
  cli_access: {
    allow: string[];
    deny?: string[];
  };
  script?: {
    enabled?: boolean;
    max_cli_calls?: number;
    timeout_ms?: number;
  };
  skill_projects?: string[];
}

/**
 * Unified tenant configuration keyed by "space.domain" (e.g. "your-space.backlog.jp").
 * Combines relay bundle signing and MCP access control in one place.
 */
export interface UnifiedTenantInput {
  /** Relay bundle signing configuration */
  relay?: RelayTenantInput;
  /** MCP access control configuration */
  mcp?: McpTenantConfig;
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
 * Backlog OAuth app configuration.
 * client_secret is separated into Secrets Manager at deploy time.
 */
export interface BacklogAppInput {
  domain: string;
  client_id: string;
  client_secret: string;
}

/**
 * Parameter Store value format.
 * Secrets (client_secret, JWKS private keys, passphrase_hash) are extracted
 * and stored in Secrets Manager by the CDK stack. SSM only holds non-secret config.
 */
export interface ParameterStoreValue {
  server: {
    port?: number;
    base_url?: string;
    allowed_host_patterns?: string;
  };
  backlog_apps: BacklogAppInput[];
  /** Unified tenant config keyed by "space.domain" (e.g. "your-space.backlog.jp") */
  tenants?: Record<string, UnifiedTenantInput>;
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
 * MCP server configuration.
 * When present, the relay Lambda also serves MCP endpoints (/mcp/*).
 * MCP tenant config is specified in the unified tenants dict (each tenant's `mcp` field).
 */
export interface McpConfig {
  /** Secrets Manager secret name for MCP JWE token key (default: "/backlog-mcp/token-key") */
  tokenKeySecretName?: string;
  /** Token key rotation interval in days (default: 30, 0 to disable) */
  tokenKeyRotationDays?: number;
}

/**
 * Relay server CDK configuration.
 */
export interface RelayConfig extends ParameterStoreConfig {
  /** CloudFront configuration (optional) */
  cloudFront?: CloudFrontConfig;
  /** MCP server configuration (optional) */
  mcp?: McpConfig;
}

/**
 * Build the SSM parameter value from CDK config.
 * Strips secrets (client_secret, JWKS, passphrase) and converts the unified
 * tenant dict into relay-core's array format + separate mcp_tenants dict.
 */
export function buildSsmParameterValue(
  value: ParameterStoreValue,
): { config: CoreRelayConfigInput; mcpTenants: Record<string, unknown> } {
  const relayTenants: Array<{
    allowed_domain: string;
    active_keys?: string;
    info_ttl?: number;
  }> = [];
  const mcpTenants: Record<string, unknown> = {};

  for (const [spaceDomain, tenant] of Object.entries(value.tenants ?? {})) {
    if (tenant.relay) {
      relayTenants.push({
        allowed_domain: spaceDomain,
        active_keys: tenant.relay.active_keys ?? "auto-1",
        info_ttl: tenant.relay.info_ttl,
      });
    }
    if (tenant.mcp) {
      mcpTenants[spaceDomain] = {
        cli_access: tenant.mcp.cli_access,
        script: tenant.mcp.script,
        skill_projects: tenant.mcp.skill_projects,
      };
    }
  }

  const config: CoreRelayConfigInput = {
    server: value.server,
    backlog_apps: value.backlog_apps.map((app) => ({
      domain: app.domain,
      client_id: app.client_id,
      client_secret: "__from_secrets_manager__",
    })),
    tenants: relayTenants.length > 0 ? relayTenants : undefined,
    access_control: value.access_control,
    rate_limit: value.rate_limit,
    cache: value.cache,
  };

  return { config, mcpTenants };
}
