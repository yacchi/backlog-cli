import { Hono } from "hono";
import { cors } from "hono/cors";
import { createWellKnownHandlers } from "./oauth/wellknown.js";
import { createOAuthHandlers } from "./oauth/handlers.js";
import { createTransportHandlers } from "./transport/handlers.js";
import type { McpServerConfig, McpTenant } from "./config/schema.js";
import type { TokenPayload } from "./crypto/jwe.js";

export { encrypt, decrypt, encryptToken, decryptToken, generateKey, importKey, exportKey } from "./crypto/jwe.js";
export type { TokenPayload } from "./crypto/jwe.js";
export { McpServerConfigSchema, parseConfig } from "./config/schema.js";
export type { McpServerConfig, McpTenant, CliAccess } from "./config/schema.js";
export { checkCliAccess, isReadOnlyCommand } from "./middleware/cli-access.js";
export { executeBacklogCommand } from "./tools/backlog.js";
export { createSandboxClient } from "./sandbox/sandbox-client.js";
export type { SandboxClient, SandboxOptions } from "./sandbox/sandbox-client.js";
export type { TokenExchange } from "./oauth/handlers.js";
export { logToolCall, logSandbox } from "./logging/logger.js";
import type { TokenExchange } from "./oauth/handlers.js";

export interface CreateMcpAppOptions {
    config: McpServerConfig;
    binPath?: string;
    runScript?: (script: string, token: TokenPayload, tenant: McpTenant | undefined, options?: { readOnly?: boolean }) => Promise<{ result: string; error?: string }>;
    tokenExchange?: TokenExchange;
    callbackPath?: string;
}

export function createMcpApp(options: CreateMcpAppOptions): Hono {
    const { config } = options;
    const app = new Hono();

    app.use("*", cors({
        origin: "*",
        allowMethods: ["GET", "POST", "DELETE", "OPTIONS"],
        allowHeaders: ["Content-Type", "Authorization", "Accept", "MCP-Protocol-Version"],
        exposeHeaders: ["WWW-Authenticate"],
    }));

    app.route("/", createWellKnownHandlers(config));
    app.route("/", createOAuthHandlers(config, {
        tokenExchange: options.tokenExchange,
        callbackPath: options.callbackPath,
    }));
    app.route("/", createTransportHandlers(config, {
        binPath: options.binPath,
        runScript: options.runScript,
    }));

    app.get("/health", (c) => c.json({ status: "ok" }));

    return app;
}
