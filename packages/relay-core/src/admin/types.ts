/**
 * Admin feature interfaces.
 *
 * Pluggable abstractions for audit log reading and passphrase management.
 * Platform-specific implementations (CloudWatch Logs, Secrets Manager, etc.)
 * are provided by relay-docker or relay-aws.
 */

export interface AuditLogQuery {
    startTime: Date;
    endTime: Date;
    action?: string;
    userEmail?: string;
    result?: "success" | "error";
    limit?: number;
}

export interface AuditLogEntry {
    timestamp: string;
    action: string;
    result: string;
    userId?: string;
    userName?: string;
    userEmail?: string;
    clientIp?: string;
    userAgent?: string;
    space?: string;
    error?: string;
    requestId?: string;
    durationMs?: number;
}

export interface AuditLogReader {
    query(params: AuditLogQuery): Promise<AuditLogEntry[]>;
}

export interface PassphraseInfo {
    hasPassphrase: boolean;
    passphrase?: string;
}

export interface PassphraseManager {
    getPassphrase(tenantName: string): Promise<PassphraseInfo>;
    setPassphrase(tenantName: string, passphrase: string): Promise<void>;
    generatePassphrase(tenantName: string): Promise<{ passphrase: string }>;
    clearPassphrase(tenantName: string): Promise<void>;
}
