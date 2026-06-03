import { Hono } from "hono";
import { createWellKnownHandlers } from "./oauth/wellknown.js";
import { createOAuthHandlers } from "./oauth/handlers.js";
import { createTransportHandlers } from "./transport/handlers.js";
import type { McpServerConfig, McpTenant } from "./config/schema.js";
import type { TokenPayload } from "./crypto/jwe.js";

export { encrypt, decrypt, encryptToken, decryptToken, generateKey, importKey, exportKey } from "./crypto/jwe.js";
export type { TokenPayload } from "./crypto/jwe.js";
export { McpServerConfigSchema, parseConfig } from "./config/schema.js";
export type { McpServerConfig, McpTenant, CliAccess } from "./config/schema.js";
export { checkCliAccess } from "./middleware/cli-access.js";
export { executeBacklogCommand } from "./tools/backlog.js";
export { createSandboxClient } from "./sandbox/sandbox-client.js";
export type { SandboxClient, SandboxOptions } from "./sandbox/sandbox-client.js";

export interface CreateMcpAppOptions {
    config: McpServerConfig;
    binPath?: string;
    runScript?: (script: string, token: TokenPayload, tenant: McpTenant | undefined) => Promise<{ result: string; error?: string }>;
}

export function createMcpApp(options: CreateMcpAppOptions): Hono {
    const { config } = options;
    const app = new Hono();

    app.route("/", createWellKnownHandlers(config));
    app.route("/", createOAuthHandlers(config));
    app.route("/", createTransportHandlers(config, {
        binPath: options.binPath,
        runScript: options.runScript,
    }));

    app.get("/health", (c) => c.json({ status: "ok" }));

    return app;
}
