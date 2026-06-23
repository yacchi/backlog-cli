/**
 * Portal admin handlers.
 *
 * Provides admin-only endpoints for audit log viewing and passphrase management.
 * Access requires OAuth session with Backlog admin role (roleType === 1) and
 * recent active authentication (auth_time within threshold).
 */

import { Hono } from "hono";
import { getCookie } from "hono/cookie";
import type { RelayConfig, AuditLogger } from "../config/types.js";
import { AuditActions, createAuditEvent } from "../middleware/audit.js";
import { extractRequestContext } from "../utils/request.js";
import { verifyPortalSessionToken, type PortalSessionClaims } from "../utils/portal-session.js";
import type { AuditLogReader, PassphraseManager } from "../admin/types.js";

const SESSION_COOKIE = "portal_session";
const AUTH_TIME_THRESHOLD_SECONDS = 1800; // 30 minutes
const BACKLOG_ROLE_ADMIN = 1;

interface AdminContext {
    claims: PortalSessionClaims;
    tenantName: string;
}

async function verifyAdminSession(
    c: { req: { param: (name: string) => string; header: (name: string) => string | undefined }; json: (data: unknown, status?: number) => Response },
    jwksJson: string,
    sessionCookieValue: string | undefined,
): Promise<AdminContext | Response> {
    if (!sessionCookieValue) {
        return c.json({ error: "authentication_required" }, 401);
    }

    let claims: PortalSessionClaims;
    try {
        claims = await verifyPortalSessionToken(sessionCookieValue, jwksJson);
    } catch {
        return c.json({ error: "authentication_required" }, 401);
    }

    const tenantName = c.req.param("name");
    if (claims.tenant !== tenantName) {
        return c.json({ error: "forbidden" }, 403);
    }

    if (claims.role !== BACKLOG_ROLE_ADMIN) {
        return c.json({ error: "admin_required" }, 403);
    }

    const now = Math.floor(Date.now() / 1000);
    if (now - claims.auth_time > AUTH_TIME_THRESHOLD_SECONDS) {
        return c.json({ error: "reauth_required" }, 401);
    }

    return { claims, tenantName };
}

export function createPortalAdminHandlers(
    config: RelayConfig,
    auditLogger: AuditLogger,
    auditLogReader?: AuditLogReader,
    passphraseManager?: PassphraseManager,
): Hono {
    const app = new Hono();
    const jwksJson = config.jwks;

    if (!jwksJson) {
        return app;
    }

    app.use("/api/v1/portal/:name/admin/*", async (c, next) => {
        c.header("Cache-Control", "no-store");
        await next();
    });

    if (auditLogReader) {
        app.get("/api/v1/portal/:name/admin/audit", async (c) => {
            const reqCtx = extractRequestContext(c);
            const sessionCookie = getCookie(c, SESSION_COOKIE);
            const result = await verifyAdminSession(c, jwksJson, sessionCookie);
            if (result instanceof Response) return result;

            const { claims, tenantName } = result;

            const now = new Date();
            const startTimeParam = c.req.query("start_time");
            const endTimeParam = c.req.query("end_time");
            const startTime = startTimeParam ? new Date(startTimeParam) : new Date(now.getTime() - 24 * 3600 * 1000);
            const endTime = endTimeParam ? new Date(endTimeParam) : now;

            if (isNaN(startTime.getTime()) || isNaN(endTime.getTime())) {
                return c.json({ error: "invalid_time_range" }, 400);
            }

            const action = c.req.query("action") || undefined;
            const userEmail = c.req.query("user_email") || undefined;
            const resultFilter = c.req.query("result") as "success" | "error" | undefined;
            const limitParam = c.req.query("limit");
            const limit = limitParam ? Math.min(parseInt(limitParam, 10) || 100, 1000) : 100;

            try {
                const entries = await auditLogReader.query({
                    startTime,
                    endTime,
                    action,
                    userEmail,
                    result: resultFilter,
                    limit,
                });

                auditLogger.log(
                    createAuditEvent({
                        action: AuditActions.ADMIN_AUDIT_QUERY,
                        domain: tenantName,
                        userId: claims.sub,
                        userName: claims.name,
                        userEmail: claims.email,
                        clientIp: reqCtx.clientIp,
                        userAgent: reqCtx.userAgent,
                        result: "success",
                    }),
                );

                return c.json({ entries, count: entries.length });
            } catch (err) {
                auditLogger.log(
                    createAuditEvent({
                        action: AuditActions.ADMIN_AUDIT_QUERY,
                        domain: tenantName,
                        userId: claims.sub,
                        userEmail: claims.email,
                        clientIp: reqCtx.clientIp,
                        userAgent: reqCtx.userAgent,
                        result: "error",
                        error: (err as Error).message,
                    }),
                );
                return c.json({ error: "query_failed" }, 500);
            }
        });
    }

    if (passphraseManager) {
        app.get("/api/v1/portal/:name/admin/passphrase", async (c) => {
            const reqCtx = extractRequestContext(c);
            const sessionCookie = getCookie(c, SESSION_COOKIE);
            const result = await verifyAdminSession(c, jwksJson, sessionCookie);
            if (result instanceof Response) return result;

            const { claims, tenantName } = result;

            try {
                const info = await passphraseManager.getPassphrase(tenantName);

                auditLogger.log(
                    createAuditEvent({
                        action: AuditActions.ADMIN_PASSPHRASE_VIEW,
                        domain: tenantName,
                        userId: claims.sub,
                        userName: claims.name,
                        userEmail: claims.email,
                        clientIp: reqCtx.clientIp,
                        userAgent: reqCtx.userAgent,
                        result: "success",
                    }),
                );

                return c.json(info);
            } catch (err) {
                auditLogger.log(
                    createAuditEvent({
                        action: AuditActions.ADMIN_PASSPHRASE_VIEW,
                        domain: tenantName,
                        userId: claims.sub,
                        userEmail: claims.email,
                        clientIp: reqCtx.clientIp,
                        userAgent: reqCtx.userAgent,
                        result: "error",
                        error: (err as Error).message,
                    }),
                );
                return c.json({ error: "failed" }, 500);
            }
        });

        app.put("/api/v1/portal/:name/admin/passphrase", async (c) => {
            const reqCtx = extractRequestContext(c);
            const sessionCookie = getCookie(c, SESSION_COOKIE);
            const result = await verifyAdminSession(c, jwksJson, sessionCookie);
            if (result instanceof Response) return result;

            const { claims, tenantName } = result;

            let body: { passphrase?: string };
            try {
                body = await c.req.json();
            } catch {
                return c.json({ error: "invalid_request" }, 400);
            }

            if (!body.passphrase || typeof body.passphrase !== "string" || body.passphrase.length < 8) {
                return c.json({ error: "passphrase_too_short" }, 400);
            }

            try {
                await passphraseManager.setPassphrase(tenantName, body.passphrase);

                auditLogger.log(
                    createAuditEvent({
                        action: AuditActions.ADMIN_PASSPHRASE_SET,
                        domain: tenantName,
                        userId: claims.sub,
                        userName: claims.name,
                        userEmail: claims.email,
                        clientIp: reqCtx.clientIp,
                        userAgent: reqCtx.userAgent,
                        result: "success",
                    }),
                );

                return c.json({ success: true });
            } catch (err) {
                auditLogger.log(
                    createAuditEvent({
                        action: AuditActions.ADMIN_PASSPHRASE_SET,
                        domain: tenantName,
                        userId: claims.sub,
                        userEmail: claims.email,
                        clientIp: reqCtx.clientIp,
                        userAgent: reqCtx.userAgent,
                        result: "error",
                        error: (err as Error).message,
                    }),
                );
                return c.json({ error: "failed" }, 500);
            }
        });

        app.post("/api/v1/portal/:name/admin/passphrase/generate", async (c) => {
            const reqCtx = extractRequestContext(c);
            const sessionCookie = getCookie(c, SESSION_COOKIE);
            const result = await verifyAdminSession(c, jwksJson, sessionCookie);
            if (result instanceof Response) return result;

            const { claims, tenantName } = result;

            try {
                const generated = await passphraseManager.generatePassphrase(tenantName);

                auditLogger.log(
                    createAuditEvent({
                        action: AuditActions.ADMIN_PASSPHRASE_GENERATE,
                        domain: tenantName,
                        userId: claims.sub,
                        userName: claims.name,
                        userEmail: claims.email,
                        clientIp: reqCtx.clientIp,
                        userAgent: reqCtx.userAgent,
                        result: "success",
                    }),
                );

                return c.json({ success: true, passphrase: generated.passphrase });
            } catch (err) {
                auditLogger.log(
                    createAuditEvent({
                        action: AuditActions.ADMIN_PASSPHRASE_GENERATE,
                        domain: tenantName,
                        userId: claims.sub,
                        userEmail: claims.email,
                        clientIp: reqCtx.clientIp,
                        userAgent: reqCtx.userAgent,
                        result: "error",
                        error: (err as Error).message,
                    }),
                );
                return c.json({ error: "failed" }, 500);
            }
        });

        app.delete("/api/v1/portal/:name/admin/passphrase", async (c) => {
            const reqCtx = extractRequestContext(c);
            const sessionCookie = getCookie(c, SESSION_COOKIE);
            const result = await verifyAdminSession(c, jwksJson, sessionCookie);
            if (result instanceof Response) return result;

            const { claims, tenantName } = result;

            try {
                await passphraseManager.clearPassphrase(tenantName);

                auditLogger.log(
                    createAuditEvent({
                        action: AuditActions.ADMIN_PASSPHRASE_CLEAR,
                        domain: tenantName,
                        userId: claims.sub,
                        userName: claims.name,
                        userEmail: claims.email,
                        clientIp: reqCtx.clientIp,
                        userAgent: reqCtx.userAgent,
                        result: "success",
                    }),
                );

                return c.json({ success: true });
            } catch (err) {
                auditLogger.log(
                    createAuditEvent({
                        action: AuditActions.ADMIN_PASSPHRASE_CLEAR,
                        domain: tenantName,
                        userId: claims.sub,
                        userEmail: claims.email,
                        clientIp: reqCtx.clientIp,
                        userAgent: reqCtx.userAgent,
                        result: "error",
                        error: (err as Error).message,
                    }),
                );
                return c.json({ error: "failed" }, 500);
            }
        });
    }

    return app;
}
