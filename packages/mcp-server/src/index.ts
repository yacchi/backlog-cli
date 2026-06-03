import { Hono } from "hono";
import { createWellKnownHandlers } from "./oauth/wellknown.js";
import { createOAuthHandlers } from "./oauth/handlers.js";
import type { McpServerConfig } from "./config/schema.js";

export { encrypt, decrypt, encryptToken, decryptToken, generateKey, importKey, exportKey } from "./crypto/jwe.js";
export type { TokenPayload } from "./crypto/jwe.js";
export { McpServerConfigSchema, parseConfig } from "./config/schema.js";
export type { McpServerConfig, McpTenant, CliAccess } from "./config/schema.js";

export interface CreateMcpAppOptions {
    config: McpServerConfig;
}

export function createMcpApp(options: CreateMcpAppOptions): Hono {
    const { config } = options;
    const app = new Hono();

    // Well-known discovery endpoints
    app.route("/", createWellKnownHandlers(config));

    // MCP OAuth AS endpoints
    app.route("/", createOAuthHandlers(config));

    // Health check
    app.get("/health", (c) => c.json({ status: "ok" }));

    return app;
}
