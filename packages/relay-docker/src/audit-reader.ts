/**
 * CloudWatch Logs Insights implementation of AuditLogReader.
 *
 * Queries structured audit JSON logs emitted by the relay's AuditLogger.
 * Audit events are identified by `component = "audit"` in the log line.
 */

import {
    CloudWatchLogsClient,
    StartQueryCommand,
    GetQueryResultsCommand,
    type ResultField,
} from "@aws-sdk/client-cloudwatch-logs";
import type { AuditLogReader, AuditLogQuery, AuditLogEntry } from "@yacchi/backlog-relay-core";

const POLL_INTERVAL_MS = 500;
const MAX_POLL_ATTEMPTS = 60; // 30 seconds max

export class CloudWatchLogsAuditReader implements AuditLogReader {
    private readonly client: CloudWatchLogsClient;
    private readonly logGroupName: string;

    constructor(logGroupName: string) {
        this.client = new CloudWatchLogsClient({});
        this.logGroupName = logGroupName;
    }

    async query(params: AuditLogQuery): Promise<AuditLogEntry[]> {
        const filters = ['filter component = "audit"'];
        if (params.action) {
            filters.push(`and action = "${params.action}"`);
        }
        if (params.userEmail) {
            filters.push(`and userEmail = "${params.userEmail}"`);
        }
        if (params.result) {
            filters.push(`and result = "${params.result}"`);
        }

        const limit = params.limit ?? 100;
        const queryString = [
            "fields @timestamp, action, result, userId, userName, userEmail, clientIp, userAgent, space, error, requestId, durationMs",
            filters.join(" "),
            "sort @timestamp desc",
            `limit ${limit}`,
        ].join(" | ");

        const startQuery = await this.client.send(
            new StartQueryCommand({
                logGroupName: this.logGroupName,
                startTime: Math.floor(params.startTime.getTime() / 1000),
                endTime: Math.floor(params.endTime.getTime() / 1000),
                queryString,
                limit,
            }),
        );

        if (!startQuery.queryId) {
            throw new Error("Failed to start CloudWatch Logs Insights query");
        }

        for (let i = 0; i < MAX_POLL_ATTEMPTS; i++) {
            await new Promise((resolve) => setTimeout(resolve, POLL_INTERVAL_MS));

            const results = await this.client.send(
                new GetQueryResultsCommand({ queryId: startQuery.queryId }),
            );

            if (results.status === "Complete") {
                return (results.results ?? []).map(parseResultRow);
            }
            if (results.status === "Failed" || results.status === "Cancelled") {
                throw new Error(`Query ${results.status}`);
            }
        }

        throw new Error("Query timed out");
    }
}

function parseResultRow(fields: ResultField[]): AuditLogEntry {
    const row: Record<string, string> = {};
    for (const field of fields) {
        if (field.field && field.value !== undefined) {
            row[field.field] = field.value;
        }
    }
    return {
        timestamp: row["@timestamp"] ?? "",
        action: row["action"] ?? "",
        result: row["result"] || (row["error"] ? "error" : "success"),
        userId: row["userId"] || undefined,
        userName: row["userName"] || undefined,
        userEmail: row["userEmail"] || undefined,
        clientIp: row["clientIp"] || undefined,
        userAgent: row["userAgent"] || undefined,
        space: row["space"] || undefined,
        error: row["error"] || undefined,
        requestId: row["requestId"] || undefined,
        durationMs: row["durationMs"] ? parseFloat(row["durationMs"]) : undefined,
    };
}
