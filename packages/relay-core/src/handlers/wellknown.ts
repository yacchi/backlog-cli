/**
 * Well-known endpoint handler for relay server discovery.
 */

import { Hono } from "hono";
import type { RelayConfig } from "../config/types.js";

/**
 * Well-known response type.
 */
interface WellKnownResponse {
  /** Version of the well-known response format */
  version: string;
  /** Relay server capabilities */
  capabilities: string[];
  /** Supported Backlog domains */
  supported_domains: string[];
}

/**
 * Create well-known handlers with the given configuration.
 */
export function createWellKnownHandlers(config: RelayConfig): Hono {
  const app = new Hono();

  /**
   * GET /.well-known/backlog-oauth-relay - Server discovery endpoint.
   */
  app.get("/.well-known/backlog-oauth-relay", (c) => {
    const response: WellKnownResponse = {
      version: "1.0",
      capabilities: ["oauth2", "token-exchange", "token-refresh"],
      supported_domains: config.backlogApps.map((app) => app.domain),
    };

    // Set cache headers for discovery endpoint
    c.header("Cache-Control", "public, max-age=3600");

    return c.json(response);
  });

  /**
   * GET /health - Health check endpoint.
   */
  app.get("/health", (c) => {
    return c.json({ status: "ok" });
  });

  return app;
}
