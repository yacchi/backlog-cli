import { Hono } from "hono";
import type { McpServerConfig } from "../config/schema.js";

export function createWellKnownHandlers(config: McpServerConfig): Hono {
    const app = new Hono();

    const protectedResourceHandler = (c: import("hono").Context) => {
        c.header("Cache-Control", "public, max-age=3600");
        return c.json({
            resource: `${config.base_url}/mcp`,
            authorization_servers: [config.base_url],
            scopes_supported: ["backlog"],
        });
    };
    app.get("/.well-known/oauth-protected-resource", protectedResourceHandler);
    app.get("/.well-known/oauth-protected-resource/mcp", protectedResourceHandler);

    app.get("/.well-known/oauth-authorization-server", (c) => {
        c.header("Cache-Control", "public, max-age=3600");
        return c.json({
            issuer: config.base_url,
            authorization_endpoint: `${config.base_url}/mcp/authorize`,
            token_endpoint: `${config.base_url}/mcp/token`,
            registration_endpoint: `${config.base_url}/mcp/register`,
            scopes_supported: ["backlog"],
            response_types_supported: ["code"],
            grant_types_supported: ["authorization_code", "refresh_token"],
            token_endpoint_auth_methods_supported: ["none"],
            code_challenge_methods_supported: ["S256"],
        });
    });

    return app;
}
