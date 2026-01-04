/**
 * Zod schemas for configuration validation.
 *
 * These schemas validate the runtime structure of configuration objects.
 * The TypeScript types in types.ts are derived from these schemas using z.infer.
 */

import { z } from "zod";

/**
 * Backlog application configuration schema.
 */
export const BacklogAppConfigSchema = z.object({
  domain: z.string().min(1, "domain is required"),
  client_id: z.string().min(1, "client_id is required"),
  client_secret: z.string().min(1, "client_secret is required"),
});

/**
 * Tenant configuration schema.
 */
export const TenantConfigSchema = z.object({
  allowed_domain: z.string().min(1, "allowed_domain is required"),
  passphrase_hash: z.string().optional(),
  jwks: z.string().optional(),
  active_keys: z.string().optional(),
  info_ttl: z.number().positive().optional(),
});

/**
 * Access control configuration schema.
 */
export const AccessControlConfigSchema = z.object({
  allowed_space_patterns: z.string().optional(),
  allowed_project_patterns: z.string().optional(),
});

/**
 * Rate limiting configuration schema.
 */
export const RateLimitConfigSchema = z.object({
  requests_per_minute: z.number().positive(),
  burst_size: z.number().positive(),
});

/**
 * Default port for local development fallback.
 */
export const DEFAULT_SERVER_PORT = 8080;

/**
 * Server configuration schema.
 */
export const ServerConfigSchema = z.object({
  base_url: z.string().url().optional(),
  allowed_host_patterns: z.string().optional(),
  port: z.number().int().min(1).max(65535).default(DEFAULT_SERVER_PORT),
});

/**
 * Cache control configuration schema.
 */
export const CacheConfigSchema = z.object({
  certs_cache_ttl: z.number().positive(),
  info_cache_ttl: z.number().positive(),
});

/**
 * Full relay server configuration schema.
 */
export const RelayConfigSchema = z.object({
  server: ServerConfigSchema,
  backlog_apps: z.array(BacklogAppConfigSchema).min(1, "At least one backlog_apps entry is required"),
  tenants: z.array(TenantConfigSchema).optional(),
  access_control: AccessControlConfigSchema.optional(),
  rate_limit: RateLimitConfigSchema.optional(),
  cache: CacheConfigSchema.optional(),
});

/**
 * Parse and validate a JSON string as RelayConfig.
 *
 * @param json - JSON string to parse
 * @returns Validated RelayConfig object
 * @throws ZodError if validation fails
 */
export function parseConfig(json: string): z.infer<typeof RelayConfigSchema> {
  const data = JSON.parse(json);
  return RelayConfigSchema.parse(data);
}

/**
 * Result of safeParseConfig.
 */
export type SafeParseConfigResult =
  | { success: true; data: z.infer<typeof RelayConfigSchema> }
  | { success: false; error: z.ZodError };

/**
 * Safely parse and validate a JSON string as RelayConfig.
 *
 * @param json - JSON string to parse
 * @returns Object with success/error information
 */
export function safeParseConfig(json: string): SafeParseConfigResult {
  try {
    const data = JSON.parse(json);
    const result = RelayConfigSchema.safeParse(data);
    if (result.success) {
      return { success: true, data: result.data };
    }
    return { success: false, error: result.error };
  } catch (e) {
    return {
      success: false,
      error: new z.ZodError([
        {
          code: "custom",
          path: [],
          message: e instanceof Error ? e.message : "Invalid JSON",
        },
      ]),
    };
  }
}

// ============================================================
// Inferred types from Zod schemas
// ============================================================

/**
 * Input type for RelayConfig (before Zod parsing).
 * Optional fields with defaults are optional here.
 */
export type RelayConfigInput = z.input<typeof RelayConfigSchema>;

/**
 * Output type for RelayConfig (after Zod parsing).
 * Fields with defaults are required here.
 */
export type RelayConfig = z.output<typeof RelayConfigSchema>;

/**
 * Server configuration (output type, after parsing).
 */
export type ServerConfig = z.output<typeof ServerConfigSchema>;

/**
 * Server configuration input (before parsing).
 */
export type ServerConfigInput = z.input<typeof ServerConfigSchema>;

/**
 * Backlog app configuration.
 */
export type BacklogAppConfig = z.infer<typeof BacklogAppConfigSchema>;

/**
 * Tenant configuration.
 */
export type TenantConfig = z.infer<typeof TenantConfigSchema>;

/**
 * Access control configuration.
 */
export type AccessControlConfig = z.infer<typeof AccessControlConfigSchema>;

/**
 * Rate limit configuration.
 */
export type RateLimitConfig = z.infer<typeof RateLimitConfigSchema>;

/**
 * Cache configuration.
 */
export type CacheConfig = z.infer<typeof CacheConfigSchema>;

// Legacy alias for backwards compatibility
export type RelayConfigParsed = RelayConfig;
