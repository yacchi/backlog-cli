/**
 * Docker/Node.js adapter for Backlog OAuth Relay Server.
 */

import { serve } from "@hono/node-server";
import { readFile } from "node:fs/promises";
import {
  createRelayApp,
  type RelayConfig,
  type AuditLogger,
  type AuditEvent,
} from "@backlog-cli/relay-core";

/**
 * Environment variable names.
 */
export const ENV_VARS = {
  RELAY_CONFIG: "RELAY_CONFIG",
  CONFIG_FILE: "CONFIG_FILE",
  PORT: "PORT",
  HOST: "HOST",
} as const;

/**
 * Parse relay configuration from environment or file.
 */
async function getConfig(): Promise<RelayConfig> {
  const configFile = process.env[ENV_VARS.CONFIG_FILE];

  if (configFile) {
    const content = await readFile(configFile, "utf-8");
    return JSON.parse(content) as RelayConfig;
  }

  const envConfig = process.env[ENV_VARS.RELAY_CONFIG];
  if (!envConfig) {
    throw new Error(
      `Either ${ENV_VARS.RELAY_CONFIG} or ${ENV_VARS.CONFIG_FILE} environment variable is required`
    );
  }
  return JSON.parse(envConfig) as RelayConfig;
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
 * Start the server.
 */
export async function startServer(): Promise<void> {
  const config = await getConfig();
  const auditLogger = createDockerAuditLogger();

  const app = createRelayApp({
    config,
    auditLogger,
  });

  const port = parseInt(process.env[ENV_VARS.PORT] || "3000", 10);
  const host = process.env[ENV_VARS.HOST] || "0.0.0.0";

  console.log(`Starting Backlog OAuth Relay Server on ${host}:${port}`);

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
export { getConfig, createDockerAuditLogger };
