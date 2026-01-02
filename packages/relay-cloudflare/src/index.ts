/**
 * Cloudflare Workers adapter for Backlog OAuth Relay Server.
 */

import {
  createRelayApp,
  type RelayConfig,
  type AuditLogger,
  type AuditEvent,
} from "@backlog-cli/relay-core";

/**
 * Cloudflare Workers environment bindings.
 */
export interface Env {
  // Required: Relay configuration as JSON string
  RELAY_CONFIG: string;

  // Optional: KV namespace for caching
  CACHE?: KVNamespace;

  // Optional: Environment name
  ENVIRONMENT?: string;
}

/**
 * Parse relay configuration from environment.
 */
function parseConfig(env: Env): RelayConfig {
  if (!env.RELAY_CONFIG) {
    throw new Error("RELAY_CONFIG environment variable is required");
  }
  return JSON.parse(env.RELAY_CONFIG) as RelayConfig;
}

/**
 * Create an AuditLogger that logs to console.
 * In production, you might want to use Cloudflare Logpush or similar.
 */
function createCloudflareAuditLogger(): AuditLogger {
  return {
    log(event: AuditEvent): void {
      console.log(JSON.stringify(event));
    },
  };
}

/**
 * Default export for Cloudflare Workers.
 */
export default {
  async fetch(
    request: Request,
    env: Env,
    _ctx: ExecutionContext
  ): Promise<Response> {
    const config = parseConfig(env);
    const auditLogger = createCloudflareAuditLogger();

    const app = createRelayApp({
      config,
      auditLogger,
    });

    return app.fetch(request, env, _ctx);
  },
};

// Export utilities for customization
export { parseConfig, createCloudflareAuditLogger };
