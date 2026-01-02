/**
 * Request context utilities.
 */

import type { Context } from "hono";

/**
 * Extracted request context for audit logging and URL construction.
 */
export interface RequestContext {
  /** Client IP address */
  clientIp: string;
  /** User agent string */
  userAgent: string;
  /** Request host (from Host header or X-Forwarded-Host) */
  host: string;
  /** Request protocol (http or https) */
  protocol: string;
  /** Constructed base URL */
  baseUrl: string;
}

/**
 * Extract request context from a Hono context.
 */
export function extractRequestContext(c: Context): RequestContext {
  const req = c.req;

  // Get client IP from various headers (in priority order)
  const clientIp =
    req.header("cf-connecting-ip") ||
    req.header("x-real-ip") ||
    req.header("x-forwarded-for")?.split(",")[0]?.trim() ||
    "unknown";

  // Get user agent
  const userAgent = req.header("user-agent") || "unknown";

  // Get host from headers
  // x-original-host is used by CloudFront (x-forwarded-host is reserved)
  const host =
    req.header("x-original-host") ||
    req.header("x-forwarded-host") ||
    req.header("host") ||
    new URL(req.url).host;

  // Determine protocol
  const protocol =
    req.header("x-forwarded-proto") ||
    (req.url.startsWith("https") ? "https" : "http");

  // Construct base URL
  const baseUrl = `${protocol}://${host}`;

  return {
    clientIp,
    userAgent,
    host,
    protocol,
    baseUrl,
  };
}
