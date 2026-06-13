import { Hono } from "hono";
import type { McpServerConfig } from "../config/schema.js";
import { resolveBaseUrl } from "../base-url.js";

export function createWellKnownHandlers(config: McpServerConfig): Hono {
    const app = new Hono();

    const protectedResourceHandler = (c: import("hono").Context) => {
        const baseUrl = resolveBaseUrl(c, config.base_url);
        c.header("Cache-Control", "public, max-age=3600");
        return c.json({
            resource: `${baseUrl}/mcp`,
            authorization_servers: [baseUrl],
            scopes_supported: ["backlog"],
        });
    };
    app.get("/.well-known/oauth-protected-resource", protectedResourceHandler);
    app.get("/.well-known/oauth-protected-resource/mcp", protectedResourceHandler);

    app.get("/.well-known/oauth-authorization-server", (c) => {
        const baseUrl = resolveBaseUrl(c, config.base_url);
        c.header("Cache-Control", "public, max-age=3600");
        return c.json({
            issuer: baseUrl,
            authorization_endpoint: `${baseUrl}/mcp/authorize`,
            token_endpoint: `${baseUrl}/mcp/token`,
            registration_endpoint: `${baseUrl}/mcp/register`,
            scopes_supported: ["backlog"],
            response_types_supported: ["code"],
            grant_types_supported: ["authorization_code", "refresh_token"],
            token_endpoint_auth_methods_supported: ["none"],
            code_challenge_methods_supported: ["S256"],
        });
    });

    return app;
}
