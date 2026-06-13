import type { Context } from "hono";

/**
 * Resolve the externally-visible base URL for a request.
 *
 * Prefers an explicitly configured base_url (a deterministic OAuth issuer, e.g.
 * a fixed custom domain). Otherwise derives it from proxy/host headers — this is
 * required when the public URL (Lambda Function URL / CloudFront default domain)
 * is not known at deploy time and cannot be configured without a circular
 * dependency.
 *
 * Header precedence: `x-original-host` (set by our CloudFront function) >
 * `x-forwarded-host` (generic proxy) > `host` (direct). Protocol comes from
 * `x-forwarded-proto`, defaulting to https.
 *
 * Behind CloudFront this is safe: CloudFront only forwards a Host that matches a
 * distribution alias, so the derived issuer is constrained to known domains.
 */
export function resolveBaseUrl(c: Context, configured?: string): string {
    if (configured) {
        return configured;
    }
    const host =
        c.req.header("x-original-host") ??
        c.req.header("x-forwarded-host") ??
        c.req.header("host");
    if (host) {
        const proto = c.req.header("x-forwarded-proto") ?? "https";
        return `${proto}://${host}`;
    }
    // Last resort: derive from the request URL itself.
    const url = new URL(c.req.url);
    return `${url.protocol}//${url.host}`;
}
