import { serve } from "@hono/node-server";
import { createMcpApp, createSandboxClient } from "./index.js";
import { parseConfig } from "./config/schema.js";

const configJson = process.env.MCP_CONFIG;
if (!configJson) {
    console.error("MCP_CONFIG environment variable is required");
    process.exit(1);
}

const config = parseConfig(configJson);

let runScript: Parameters<typeof createMcpApp>[0]["runScript"];

const hasSandboxEnabled = config.spaces.length > 0;

if (hasSandboxEnabled) {
    const sandbox = await createSandboxClient({
        workerPath: process.env.SANDBOX_WORKER_PATH,
        binPath: process.env.BACKLOG_BIN_PATH,
    });

    runScript = (script, token, scriptConfig, opts) => sandbox.execute(script, token, scriptConfig, opts?.readOnly, opts?.files);

    process.on("SIGTERM", () => sandbox.shutdown());
    process.on("SIGINT", () => sandbox.shutdown());
}

const app = await createMcpApp({
    config,
    binPath: process.env.BACKLOG_BIN_PATH,
    runScript,
});

const port = parseInt(process.env.PORT || "8080", 10);

serve({ fetch: app.fetch, port, hostname: "0.0.0.0" }, (info) => {
    console.log(`MCP server listening on http://0.0.0.0:${info.port}`);
});
