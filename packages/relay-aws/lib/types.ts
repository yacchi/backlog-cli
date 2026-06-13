/**
 * AWS CDK deployment configuration types.
 *
 * Runtime configuration types are imported from @yacchi/backlog-relay-core.
 */

import type { RelayConfigInput as CoreRelayConfigInput } from "@yacchi/backlog-relay-core";

// Re-export core types for convenience
export type { BacklogAppConfig, TenantConfig } from "@yacchi/backlog-relay-core";

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
 * Relay tenant configuration.
 * With server-level JWKS, tenants only need passphrase and optional TTL.
 */
export interface RelayTenantInput {
  info_ttl?: number;
  /** Default space host for this tenant (e.g. "example.backlog.jp"). Used by CLI setup when --space is omitted. */
  default_space?: string;
  /** Plaintext passphrase — auto-hashed with bcrypt at CDK synth time */
  passphrase?: string;
  /** Pre-computed bcrypt hash (mutually exclusive with passphrase) */
  passphrase_hash?: string;
  /** Auto-generated passphrase length in characters (default: 32) */
  passphrase_length?: number;
}

/**
 * MCP space access control pattern.
 */
export interface SpacePatternInput {
  pattern: string;
  writable: boolean;
}

/**
 * CloudFront cache configuration.
 */
export interface CloudFrontCacheConfig {
  assetsMaxAge?: number;
  apiDefaultTtl?: number;
  apiMaxTtl?: number;
  apiMinTtl?: number;
}

/**
 * Custom domain settings for CloudFront.
 *
 * domainName と certificateArn は常にセットで指定する必要がある
 * （CloudFront のエイリアスにはドメイン名と ACM 証明書の両方が必須）。
 * hostedZoneId を指定すると Route53 に A/AAAA レコードも作成する。
 */
export interface CloudFrontCustomDomain {
  /** カスタムドメイン名（例: backlog-relay.example.com） */
  domainName: string;
  /** ACM 証明書 ARN（CloudFront 用なので us-east-1 のもの） */
  certificateArn: string;
  /** Route53 ホストゾーン ID（指定時のみ DNS レコードを自動作成） */
  hostedZoneId?: string;
}

/**
 * CloudFront configuration.
 *
 * customDomain は「全て指定」か「未指定」のどちらかのみ許可する。
 * domainName だけ・certificateArn だけといった部分指定は型レベルで防ぐ
 * （部分指定だとカスタムドメインが黙って無効化されデプロイ事故につながるため）。
 */
export interface CloudFrontConfig {
  enabled: boolean;
  /** カスタムドメイン設定（ドメイン名・証明書・任意でホストゾーン） */
  customDomain?: CloudFrontCustomDomain;
  cache?: CloudFrontCacheConfig;
}

/**
 * Parameter Store configuration for CDK deployment.
 */
export interface ParameterStoreConfig {
  parameterName: string;
  parameterValue?: ParameterStoreValue;
}

/**
 * Backlog OAuth app configuration.
 */
export interface BacklogAppInput {
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
  backlog_app: BacklogAppInput;

  /** Server-level signing keys (shared by relay + MCP) */
  jwks?: JWKS;

  /** Tenant (= バンドル配布単位) config keyed by name. スペースとは無関係。 */
  tenants?: Record<string, RelayTenantInput>;

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
 * MCP server configuration (top-level, separate from tenants).
 */
export interface McpConfig {
  spaces: SpacePatternInput[];
  script?: {
    max_cli_calls?: number;
    timeout_ms?: number;
  };
  default_spaces?: string[];
}

/**
 * Container image configuration.
 *
 * The unified runtime ships as a single published image (e.g. GHCR). Lambda can
 * only pull from a same-region private ECR, so the CDK construct copies the
 * published image into a dedicated ECR repo (via cdk-ecr-deployment, registry to
 * registry, no Docker at deploy) and points the DockerImageFunction at it.
 */
export interface ContainerImageConfig {
  /** Source image repository (e.g. the published GHCR repo). */
  source?: string;
  /**
   * Explicit image tag to deploy. When omitted, the latest semver tag is
   * resolved from the registry (see resolveLatestImageTag).
   */
  tag?: string;
  /**
   * Tag-resolution mode when `tag` is omitted:
   * - false (default): pick the highest *stable* release (no prerelease suffix).
   *   This prevents an in-development build from being deployed by accident.
   * - true: pick the highest *prerelease* tag (to intentionally target a dev build).
   */
  prerelease?: boolean;
}

/**
 * Relay server CDK configuration.
 */
export interface RelayConfig extends ParameterStoreConfig {
  cloudFront?: CloudFrontConfig;
  mcp?: McpConfig;
  /** Container image source/tag (defaults to the published GHCR image). */
  image?: ContainerImageConfig;
}

/**
 * Build the SSM parameter value from CDK config.
 * Strips secrets and converts tenants to relay-core format.
 * MCP config (spaces, script, default_spaces) is stored as separate keys.
 */
export function buildSsmParameterValue(
  value: ParameterStoreValue,
  mcp?: McpConfig,
): { config: CoreRelayConfigInput; mcpSpaces?: SpacePatternInput[]; mcpScript?: McpConfig["script"]; mcpDefaultSpaces?: string[] } {
  const relayTenants: Array<{
    name: string;
    default_space?: string;
    info_ttl?: number;
  }> = [];

  for (const [name, tenant] of Object.entries(value.tenants ?? {})) {
    relayTenants.push({
      name,
      default_space: tenant.default_space,
      info_ttl: tenant.info_ttl,
    });
  }

  const config: CoreRelayConfigInput = {
    server: value.server,
    backlog_app: {
      client_id: value.backlog_app.client_id,
      client_secret: "__from_secrets_manager__",
    },
    tenants: relayTenants.length > 0 ? relayTenants : undefined,
    access_control: value.access_control,
    rate_limit: value.rate_limit,
    cache: value.cache,
  };

  return {
    config,
    mcpSpaces: mcp?.spaces,
    mcpScript: mcp?.script,
    mcpDefaultSpaces: mcp?.default_spaces,
  };
}
