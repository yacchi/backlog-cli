import { Hono } from "hono";
import { cors } from "hono/cors";
import { createWellKnownHandlers } from "./oauth/wellknown.js";
import { createOAuthHandlers } from "./oauth/handlers.js";
import { createTransportHandlers } from "./transport/handlers.js";
import type { McpServerConfig, ScriptConfig } from "./config/schema.js";
import type { TokenPayload } from "./crypto/jwt.js";
import { loadSigningKeys } from "./crypto/jwt.js";

const MCP_CORS_HEADERS = {
    origin: "*",
    allowMethods: ["GET", "POST", "DELETE", "OPTIONS"],
    allowHeaders: ["Content-Type", "Authorization", "Accept", "MCP-Protocol-Version"],
    exposeHeaders: ["WWW-Authenticate"],
};

export { sign, verify, signToken, verifyToken, loadSigningKeys } from "./crypto/jwt.js";
export type { TokenPayload, SigningKeys } from "./crypto/jwt.js";
export { McpServerConfigSchema, parseConfig, matchSpacePattern } from "./config/schema.js";
export type { McpServerConfig, ScriptConfig, SpacePattern, SpaceAccess } from "./config/schema.js";
export { executeBacklogCommand } from "./tools/backlog.js";
export { materializeFiles, substituteFileRefs } from "./tools/files.js";
export { createSandboxClient } from "./sandbox/sandbox-client.js";
export type { SandboxClient, SandboxOptions } from "./sandbox/sandbox-client.js";
export type { TokenExchange } from "./oauth/handlers.js";
export type { ScriptFile } from "./transport/handlers.js";
export { logToolCall, logSandbox } from "./logging/logger.js";
import type { TokenExchange } from "./oauth/handlers.js";
import type { ScriptFile } from "./transport/handlers.js";

export interface CreateMcpAppOptions {
    config: McpServerConfig;
    binPath?: string;
    runScript?: (script: string, token: TokenPayload, scriptConfig: ScriptConfig | undefined, options?: { readOnly?: boolean; files?: ScriptFile[] }) => Promise<{ result: string; error?: string }>;
    tokenExchange?: TokenExchange;
    callbackPath?: string;
}

export async function createMcpApp(options: CreateMcpAppOptions): Promise<Hono> {
    const { config } = options;
    const app = new Hono();

    const keys = await loadSigningKeys(config.jwks);

    // MCP transport, token, register, well-known: cross-origin access required by MCP spec (Bearer auth, no cookies)
    app.use("/mcp", cors(MCP_CORS_HEADERS));
    app.use("/mcp/register", cors(MCP_CORS_HEADERS));
    app.use("/mcp/token", cors(MCP_CORS_HEADERS));
    app.use("/.well-known/*", cors(MCP_CORS_HEADERS));
    // OAuth authorize routes (/mcp/authorize*) intentionally have NO CORS —
    // they are same-origin (HTML served by this server) and use cookies.

    app.route("/", createWellKnownHandlers(config));
    app.route("/", createOAuthHandlers(config, keys, {
        tokenExchange: options.tokenExchange,
        callbackPath: options.callbackPath,
    }));
    app.route("/", createTransportHandlers(config, keys, {
        binPath: options.binPath,
        runScript: options.runScript,
    }));

    app.get("/health", (c) => c.json({ status: "ok" }));

    return app;
}
