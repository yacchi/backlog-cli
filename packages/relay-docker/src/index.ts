/**
 * Docker/Node.js adapter for Backlog OAuth Relay Server.
 *
 * 設定は RELAY_CONFIG 環境変数（JSON文字列）から読み込みます。
 * これはAWS Lambda実装と同じパターンで、Docker環境での利用に適しています。
 */

import { serve } from "@hono/node-server";
import { dirname, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import {
  createRelayApp,
  createBundle,
  verifyPassphrase,
  parseConfig,
  type RelayConfig,
  type AuditLogger,
  type AuditEvent,
} from "@backlog-cli/relay-core";
import { loadPortalAssets } from "./portal-assets.js";

const __dirname = dirname(fileURLToPath(import.meta.url));

/**
 * Environment variable names.
 */
export const ENV_VARS = {
  RELAY_CONFIG: "RELAY_CONFIG",
  HOST: "HOST",
  WEB_DIST_PATH: "WEB_DIST_PATH",
} as const;

/**
 * Parse relay configuration from RELAY_CONFIG environment variable.
 * Uses Zod schema validation for runtime type safety.
 */
function getConfig(): RelayConfig {
  const envConfig = process.env[ENV_VARS.RELAY_CONFIG];
  if (!envConfig) {
    throw new Error(
      `${ENV_VARS.RELAY_CONFIG} environment variable is required (JSON string)`
    );
  }
  return parseConfig(envConfig);
}

/**
 * Create an AuditLogger that logs to stdout in JSON format.
 */
function createDockerAuditLogger(): AuditLogger {
  return {
    log(event: AuditEvent): void {
      console.log(JSON.stringify(event));
    },
  };
}

/**
 * Get web dist path from environment or default.
 */
function getWebDistPath(): string {
  const envPath = process.env[ENV_VARS.WEB_DIST_PATH];
  if (envPath) {
    return resolve(envPath);
  }
  // Default: look for web/dist relative to project root
  return resolve(__dirname, "../../../web/dist");
}

/**
 * Start the server.
 */
export async function startServer(): Promise<void> {
  const config = getConfig();
  const auditLogger = createDockerAuditLogger();

  // Load portal assets
  const webDistPath = getWebDistPath();
  const portalAssets = await loadPortalAssets(webDistPath);

  const app = createRelayApp({
    config,
    auditLogger,
    verifyPassphrase,
    createBundle,
    portalAssets,
  });

  const port = config.server.port
  const host = process.env[ENV_VARS.HOST] || "0.0.0.0";

  console.log(`Starting Backlog OAuth Relay Server on ${host}:${port}`);
  if (portalAssets) {
    console.log(`Portal assets loaded from: ${webDistPath}`);
  } else {
    console.log("Portal assets not available (build web package first)");
  }

  serve({
    fetch: app.fetch,
    port,
    hostname: host,
  });
}

// Auto-start when run directly
startServer().catch((err) => {
  console.error("Failed to start server:", err);
  process.exit(1);
});

// Export utilities for customization
export { getConfig, createDockerAuditLogger, loadPortalAssets };
