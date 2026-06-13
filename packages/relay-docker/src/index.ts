/**
 * Unified container runtime for the Backlog OAuth Relay + MCP server.
 *
 * Serves both the OAuth relay (relay-core) and, when MCP spaces are configured,
 * the MCP server (mcp-server) from a single HTTP process. The same image runs:
 *
 * - locally / Docker: config from `RELAY_CONFIG` (JSON, secrets inline)
 * - AWS Lambda container: config from SSM + Secrets Manager
 *   (`CONFIG_PARAMETER_NAME` / `RELAY_SECRETS_NAME`), via the Lambda Web Adapter
 *
 * The config source is selected automatically — see {@link ./config-source}.
 */

import { serve } from "@hono/node-server";
import { dirname, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import type { AuditLogger, AuditEvent } from "@backlog-cli/relay-core";
import {
  createSandboxClient,
  type McpServerConfig,
  type CreateMcpAppOptions,
} from "@backlog-cli/mcp-server";
import { loadPortalAssets } from "./portal-assets.js";
import { selectConfigSource } from "./config-source.js";
import { createUnifiedApp } from "./app.js";

const __dirname = dirname(fileURLToPath(import.meta.url));

/**
 * Environment variable names (config-source vars live in {@link ./config-source}).
 */
export const ENV_VARS = {
  HOST: "HOST",
  PORT: "PORT",
  WEB_DIST_PATH: "WEB_DIST_PATH",
  BACKLOG_BIN_PATH: "BACKLOG_BIN_PATH",
  SANDBOX_WORKER_PATH: "SANDBOX_WORKER_PATH",
} as const;

/**
 * Create an AuditLogger that logs to stdout in JSON format.
 */
export function createDockerAuditLogger(): AuditLogger {
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
  // Default: look for web/dist relative to project root.
  return resolve(__dirname, "../../../web/dist");
}

// The sandbox (Deno + Pyodide) is expensive to start, so reuse a single client
// across requests and shut it down on process termination.
let cachedSandbox: Awaited<ReturnType<typeof createSandboxClient>> | null = null;

/**
 * Lazily create the MCP `runScript` implementation backed by the Deno sandbox.
 *
 * Returns undefined (run_script tool disabled) when no spaces are configured,
 * or when the sandbox cannot be started (e.g. Deno not available locally). The
 * latter degrades gracefully so the relay + MCP `backlog` tool stay usable for
 * local testing without the Python sandbox toolchain; a warning is logged.
 */
async function createRunScript(
  mcpConfig: McpServerConfig,
): Promise<CreateMcpAppOptions["runScript"]> {
  if (mcpConfig.spaces.length === 0) {
    return undefined;
  }

  if (!cachedSandbox) {
    try {
      cachedSandbox = await createSandboxClient({
        workerPath: process.env[ENV_VARS.SANDBOX_WORKER_PATH],
        binPath: process.env[ENV_VARS.BACKLOG_BIN_PATH],
      });
      process.on("SIGTERM", () => cachedSandbox?.shutdown());
      process.on("SIGINT", () => cachedSandbox?.shutdown());
    } catch (err) {
      console.warn(
        `MCP run_script disabled: sandbox failed to start (${err instanceof Error ? err.message : String(err)}). ` +
          "The relay and MCP backlog tool remain available.",
      );
      return undefined;
    }
  }

  return (script, token, scriptConfig, opts) =>
    cachedSandbox!.execute(
      script,
      token,
      scriptConfig,
      opts?.readOnly,
      opts?.files,
    );
}

/**
 * Start the unified HTTP server.
 */
export async function startServer(): Promise<void> {
  const configSource = selectConfigSource();
  const rawConfig = await configSource.loadRawConfig();
  const auditLogger = createDockerAuditLogger();

  const webDistPath = getWebDistPath();
  const portalAssets = await loadPortalAssets(webDistPath);

  const app = await createUnifiedApp({
    rawConfig,
    auditLogger,
    portalAssets,
    binPath: process.env[ENV_VARS.BACKLOG_BIN_PATH],
    createRunScript,
  });

  // Port precedence: PORT env (Lambda Web Adapter sets this) > config > 8080.
  const serverConfig = (rawConfig.server ?? {}) as { port?: number };
  const port =
    Number(process.env[ENV_VARS.PORT]) || serverConfig.port || 8080;
  const host = process.env[ENV_VARS.HOST] || "0.0.0.0";

  const mcpEnabled = Array.isArray(rawConfig.mcp_spaces)
    ? (rawConfig.mcp_spaces as unknown[]).length > 0
    : false;

  console.log(
    `Starting Backlog Relay${mcpEnabled ? " + MCP" : ""} server on ${host}:${port}`,
  );
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

// Auto-start when run directly (i.e. as the container entrypoint).
startServer().catch((err) => {
  console.error("Failed to start server:", err);
  process.exit(1);
});

// Export utilities for customization / testing.
export { loadPortalAssets } from "./portal-assets.js";
export { createUnifiedApp } from "./app.js";
export {
  selectConfigSource,
  EnvConfigSource,
  AwsConfigSource,
} from "./config-source.js";
